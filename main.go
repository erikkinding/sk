package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/paulrademacher/climenu"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"
)

const (
	namespaceFilterConfig = "KS_NAMESPACE_FILTER"
)

var (
	kubeConfigPath  = resolveKubeConfigPath()
	namespaceFilter = loadNamespaceFilter()
)

func main() {
	// Load kube config
	clientConfig := loadConfig()
	rawConfig, err := clientConfig.RawConfig()
	checkErr(err)

	// Context
	selectedContext := selectContext(rawConfig)

	// Namespace
	selectNamespace(rawConfig, selectedContext)
}

func selectContext(rawConfig api.Config) string {
	i := 0
	menu := climenu.NewButtonMenu("", "select a context")
	for contextName := range rawConfig.Contexts {
		menu.AddMenuItem(contextName, contextName)
		if contextName == rawConfig.CurrentContext {
			menu.CursorPos = i
		}
		i++
	}

	contextSelection, escaped := menu.Run()
	if escaped {
		os.Exit(0)
	}
	rawConfig.CurrentContext = contextSelection
	setConfig(rawConfig)

	return contextSelection
}

func selectNamespace(rawConfig api.Config, selectedContext string) {
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	checkErr(err)

	clientset, err := kubernetes.NewForConfig(restConfig)
	checkErr(err)

	namespaces, err := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	checkErr(err)

	// Namespace selection
	i := 0
	currentNamespace := rawConfig.Contexts[selectedContext].Namespace
	menu := climenu.NewButtonMenu("", "select a namespace")
	filterForContext := namespaceFilter[selectedContext]
	for _, ns := range namespaces.Items {
		keep := true
		if filterForContext != nil {
			keep = false
			for _, criteria := range filterForContext {
				if strings.Contains(ns.Name, criteria) {
					keep = true
					break
				}
			}
		}

		if keep {
			menu.AddMenuItem(ns.Name, ns.Name)
			if ns.Name == currentNamespace {
				menu.CursorPos = i
			}
			i++
		}
	}

	nsSelection, escaped := menu.Run()
	if escaped {
		os.Exit(0)
	}
	rawConfig.Contexts[selectedContext].Namespace = nsSelection

	setConfig(rawConfig)
}

func loadNamespaceFilter() map[string][]string {
	filterConfig := os.Getenv(namespaceFilterConfig)
	if filterConfig == "" {
		return nil
	}

	// Config is expected on the format "context_name:match_criteria1,context_name:match_criteria2"
	settings := strings.Split(filterConfig, ",")
	filter := map[string][]string{}
	for _, setting := range settings {
		contextAndCriteria := strings.Split(setting, ":")
		if len(contextAndCriteria) != 2 {
			log.Fatal(fmt.Sprintf("%s is expected on the format 'context_name:match_criteria1,context_name:match_criteria2'", namespaceFilterConfig))
		}
		filter[contextAndCriteria[0]] = append(filter[contextAndCriteria[0]], contextAndCriteria[1])
	}

	return filter
}

func loadConfig() clientcmd.ClientConfig {
	client := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfigPath},
		&clientcmd.ConfigOverrides{
			CurrentContext: "",
		})

	return client
}

func setConfig(c api.Config) {
	err := clientcmd.ModifyConfig(clientcmd.NewDefaultPathOptions(), c, true)
	checkErr(err)
}

func resolveKubeConfigPath() string {
	pathFromEnv := os.Getenv("KUBECONFIG")
	if pathFromEnv != "" {
		return pathFromEnv
	}

	return filepath.Join(homedir.HomeDir(), ".kube", "config")
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
