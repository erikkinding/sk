package main

import (
	"log"
	"path/filepath"

	"github.com/paulrademacher/climenu"
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

	config, err := client.RawConfig()

	checkErr(err)

	// Display options
	i := 0
	menu := climenu.NewButtonMenu("", "select a context")
	for contextName := range config.Contexts {
		menu.AddMenuItem(contextName, contextName)
		if contextName == config.CurrentContext {
			menu.CursorPos = i
		}
		i++
	}

	// Make selection
	selection, escaped := menu.Run()
	if !escaped {
		switchContext(config, selection)
	}
}

func switchContext(c api.Config, ctx string) {
	c.CurrentContext = ctx
	err := clientcmd.ModifyConfig(clientcmd.NewDefaultPathOptions(), c, true)
	checkErr(err)
}
func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
