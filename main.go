package main

import (
	"fmt"
	"os"
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

	if err != nil {
		panic(err)
	}

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

func switchContext(c api.Config, ctx string) error {
	c.CurrentContext = ctx
	err := clientcmd.ModifyConfig(clientcmd.NewDefaultPathOptions(), c, true)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return nil
}
