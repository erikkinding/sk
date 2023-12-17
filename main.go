package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

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

	var switchPrevious bool
	flag.BoolVar(&switchPrevious, "p", false, "Use to switch to the previously used context and namespace")
	flag.Parse()

	// Load kube config
	clientConfig := loadConfig()
	rawConfig, err := clientConfig.RawConfig()
	checkErr(err)

	// Previous, to store if something is changed
	currentContext := rawConfig.CurrentContext
	currentNamespace := rawConfig.Contexts[currentContext].Namespace

	if switchPrevious {
		previousContext := readPrevious("context")
		previousNamespace := readPrevious("namespace")
		if previousContext != "" && previousNamespace != "" {
			rawConfig.CurrentContext = previousContext
			rawConfig.Contexts[currentContext].Namespace = previousNamespace
			setConfig(rawConfig)
		}

	} else {
		// Context
		rawConfig = selectContext(rawConfig)

		// Namespace
		selectNamespace(rawConfig)
	}

	// Store previous.
	checkErr(createTempDir())
	checkErr(storePrevious("context", currentContext))
	checkErr(storePrevious("namespace", currentNamespace))
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

		return prompt.FilterFuzzy(s, in.GetWordBeforeCursor(), true)
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

func readPrevious(key string) string {
	p := path.Join(os.TempDir(), "sk", key)
	fileBytes, err := os.ReadFile(p)

	// Fine, simply no previous value stored
	if os.IsNotExist(err) {
		return ""
	}

	checkErr(err)

	return string(fileBytes)
}

func storePrevious(key, value string) error {
	p := path.Join(os.TempDir(), "sk", key)

	// Create or truncate
	f, err := os.Create(p)
	if err != nil {
		return err
	}

	n, err := f.WriteString(value)
	fmt.Printf("Wrote %d bytes to %s", n, p)
	return err
}

func createTempDir() error {
	err := os.Mkdir(path.Join(os.TempDir(), "sk"), os.ModePerm)
	if strings.Contains(err.Error(), "file exists") {
		return nil
	}

	return err
}
