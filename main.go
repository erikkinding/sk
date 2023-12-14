package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"

	prompt "github.com/c-bata/go-prompt"
)

var (
	kubeConfigPath = resolveKubeConfigPath()
)

func main() {

	// Load kube config
	clientConfig := loadConfig()
	rawConfig, err := clientConfig.RawConfig()
	checkErr(err)

	// Context
	rawConfig = selectContext(rawConfig)

	// Namespace
	selectNamespace(rawConfig)
}

func selectContext(rawConfig api.Config) api.Config {
	contexts := []string{}
	for context := range rawConfig.Contexts {
		// Current value on top
		if context == rawConfig.CurrentContext {
			contexts = append([]string{context}, contexts...)
		} else {
			contexts = append(contexts, context)
		}
	}

	selectedContext := showPrompt(contexts)

	if !validateSelection(contexts, selectedContext) {
		fail(fmt.Sprintf("'%s' is not a valid context selection", selectedContext))
	}

	rawConfig.CurrentContext = selectedContext
	setConfig(rawConfig)

	return rawConfig
}

func selectNamespace(rawConfig api.Config) {
	selectedContext := rawConfig.CurrentContext

	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	checkErr(err)

	clientset, err := kubernetes.NewForConfig(restConfig)
	checkErr(err)

	nss, err := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	checkErr(err)

	// Namespace selection
	currentNamespace := rawConfig.Contexts[selectedContext].Namespace
	nsNames := []string{}
	for _, ns := range nss.Items {
		// Current value on top
		if ns.Name == currentNamespace {
			nsNames = append([]string{ns.Name}, nsNames...)
		} else {
			nsNames = append(nsNames, ns.Name)
		}
	}

	nsSelection := showPrompt(nsNames)

	if !validateSelection(nsNames, nsSelection) {
		fail(fmt.Sprintf("'%s' is not a valid namespace selection", selectedContext))
	}

	rawConfig.Contexts[selectedContext].Namespace = nsSelection

	setConfig(rawConfig)
}

func completer(suggestions []string) func(in prompt.Document) []prompt.Suggest {
	return func(in prompt.Document) []prompt.Suggest {
		s := []prompt.Suggest{}
		for _, suggestion := range suggestions {
			s = append(s, prompt.Suggest{Text: suggestion})
		}
		return prompt.FilterHasPrefix(s, in.GetWordBeforeCursor(), true)
	}
}

func executor(in string) {

	fmt.Println(in)

	if in[0] == byte(prompt.ControlC) {
		os.Exit(0)
	}
}

func showPrompt(suggestions []string) string {
	p := prompt.New(
		executor,
		completer(suggestions),
		prompt.OptionPreviewSuggestionTextColor(prompt.Blue),
		prompt.OptionSelectedSuggestionBGColor(prompt.LightGray),
		prompt.OptionSuggestionBGColor(prompt.DarkGray),
		prompt.OptionMaxSuggestion(15),
		prompt.OptionCompletionOnDown(),
		prompt.OptionShowCompletionAtStart(),
		prompt.OptionPrefix(" ⎈ "),
	)

	return p.Input()
}

func validateSelection(selections []string, selection string) bool {
	valid := false
	for _, s := range selections {
		if s == selection {
			valid = true
			break
		}
	}

	return valid
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
		fail(err.Error())
	}
}

func fail(msg string) {
	fmt.Println(msg)
	os.Exit(1)
}
