package main

import (
	"context"
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

var (
	kubeConfig      = filepath.Join(homedir.HomeDir(), ".kube", "config")
	namespaceFilter = loadNamespaceFilter()
)

func main() {

	// Load kube config
	clientConfig := loadConfig()

	rawConfig, err := clientConfig.RawConfig()
	checkErr(err)

	// Context options and selection
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
		return
	}
	rawConfig.CurrentContext = contextSelection
	setConfig(rawConfig)

	// List namesapces for selected context
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	checkErr(err)

	clientset, err := kubernetes.NewForConfig(restConfig)
	checkErr(err)

	namespaces, err := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	checkErr(err)

	// Namespace selection
	i = 0
	currentNamespace := rawConfig.Contexts[contextSelection].Namespace
	menu = climenu.NewButtonMenu("", "select a namespace")
	filterForContext := namespaceFilter[contextSelection]
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
		return
	}
	rawConfig.Contexts[contextSelection].Namespace = nsSelection

	setConfig(rawConfig)
}

func loadNamespaceFilter() map[string][]string {
	filterConfig := os.Getenv("KS_NAMESPACE_FILTER")
	if filterConfig == "" {
		return nil
	}

	// Config is expected on the format "context_name:match_criteria1,context_name:match_criteria2"
	settings := strings.Split(filterConfig, ",")
	filter := map[string][]string{}
	for _, setting := range settings {
		contextAndCriteria := strings.Split(setting, ":")
		if len(contextAndCriteria) != 2 {
			log.Fatal("KS_NAMESPACE_FILTER is expected on the format 'context_name:match_criteria1,context_name:match_criteria2'")
		}
		filter[contextAndCriteria[0]] = append(filter[contextAndCriteria[0]], contextAndCriteria[1])
	}

	return filter
}

func loadConfig() clientcmd.ClientConfig {
	client := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfig},
		&clientcmd.ConfigOverrides{
			CurrentContext: "",
		})

	return client
}

func setConfig(c api.Config) {
	err := clientcmd.ModifyConfig(clientcmd.NewDefaultPathOptions(), c, true)
	checkErr(err)
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
