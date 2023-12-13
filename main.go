package main

import (
	"context"
	"log"
	"path/filepath"

	"github.com/paulrademacher/climenu"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"
)

func main() {

	// Load kube config
	kubeConfig := filepath.Join(homedir.HomeDir(), ".kube", "config")

	client := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfig},
		&clientcmd.ConfigOverrides{
			CurrentContext: "",
		})

	rawConfig, err := client.RawConfig()

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
	if !escaped {
		switchContext(rawConfig, contextSelection)
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	checkErr(err)

	clientset, err := kubernetes.NewForConfig(config)
	checkErr(err)

	namespaces, err := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	checkErr(err)

	// Namespace selection
	i = 0
	menu = climenu.NewButtonMenu("", "select a namespace")
	for _, ns := range namespaces.Items {
		menu.AddMenuItem(ns.Name, ns.Name)
		// if contextName == rawConfig.CurrentContext {
		// 	menu.CursorPos = i
		// }
		// i++
	}

	nsSelection, escaped := menu.Run()
	if !escaped {
		switchNs(rawConfig, contextSelection, nsSelection)
	}
}

func switchContext(c api.Config, ctx string) {
	c.CurrentContext = ctx
	err := clientcmd.ModifyConfig(clientcmd.NewDefaultPathOptions(), c, true)
	checkErr(err)
}

func switchNs(c api.Config, ctx, ns string) {
	c.Contexts[ctx].Namespace = ns
	err := clientcmd.ModifyConfig(clientcmd.NewDefaultPathOptions(), c, true)
	checkErr(err)
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
