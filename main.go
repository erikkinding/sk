package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/term"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"

	prompt "github.com/c-bata/go-prompt"
)

var (
	kubeConfigPath = resolveKubeConfigPath()
	termState      *term.State
)

const (
	previousContextKey         = "previous_context"
	previousNamespaceKey       = "previous_namespace"
	favoriteContextKeyPrefix   = "favorite_context_"
	favoriteNamespaceKeyPrefix = "favorite_namespace_"
)

func main() {

	// To deal with this issue, temporary workaround:
	saveTermState()
	defer restoreTermState()

	// Create and check config dir
	checkErr(createSkDir())

	// Flags
	var switchPrevious bool
	var nameSpaceMode bool
	var nameSpaceOnlyMode bool
	var printCurrent bool
	var listFavorites bool
	var favorite string

	flag.BoolVar(&switchPrevious, "p", false, "Use to switch to the previously used context and namespace. Has no effect if state can't be retrieved.")
	flag.BoolVar(&nameSpaceMode, "n", false, "Select namespace from the ones available for the selected context")
	flag.BoolVar(&nameSpaceOnlyMode, "N", false, "Only select namespace from the ones available for the selected context")
	flag.BoolVar(&printCurrent, "c", false, "Print the currently selected context and namespace")
	flag.BoolVar(&listFavorites, "l", false, "List all stored favorites")
	flag.StringVar(&favorite, "f", "", "Select a favorite context")
	flag.StringVar(&favorite, "F", "", "Store current context and namespace as favorite")
	flag.Parse()

	loadFavorite := flagPassed("f")
	storeFavorite := flagPassed("F")
	if loadFavorite && storeFavorite {
		fail("Can't use -f and -F at the same time")
	}

	// Load kube config
	clientConfig := loadConfig()
	rawConfig, err := clientConfig.RawConfig()
	checkErr(err)

	// Print current context and namespace
	if printCurrent {
		printCurrentContextAndNamespace(rawConfig)
		return
	}

	// Previous, to store if something is changed
	currentContext := rawConfig.CurrentContext
	var currentNamespace string
	var hasPrevious bool
	if currentContext != "" {
		currentNamespace = rawConfig.Contexts[currentContext].Namespace
		hasPrevious = true
	}

	if loadFavorite {
		favoriteContext := readValue(fmt.Sprintf("%s%s", favoriteContextKeyPrefix, favorite))
		favoriteNamespace := readValue(fmt.Sprintf("%s%s", favoriteNamespaceKeyPrefix, favorite))
		if favoriteContext != "" && favoriteNamespace != "" {
			rawConfig.CurrentContext = favoriteContext
			rawConfig.Contexts[currentContext].Namespace = favoriteNamespace
			setConfig(rawConfig)
		}
	} else if storeFavorite {
		checkErr(storeValue(fmt.Sprintf("%s%s", favoriteContextKeyPrefix, favorite), currentContext))
		checkErr(storeValue(fmt.Sprintf("%s%s", favoriteNamespaceKeyPrefix, favorite), currentNamespace))
	} else if switchPrevious {
		previousContext := readValue(previousContextKey)
		previousNamespace := readValue(previousNamespaceKey)
		if previousContext != "" && previousNamespace != "" {
			rawConfig.CurrentContext = previousContext
			rawConfig.Contexts[currentContext].Namespace = previousNamespace
			setConfig(rawConfig)
		}
	} else if listFavorites {
		printFavorites()
	} else {
		// Context
		if !nameSpaceOnlyMode {
			rawConfig = selectContext(rawConfig)
		}

		// Namespace
		if nameSpaceMode || nameSpaceOnlyMode {
			selectNamespace(rawConfig)
		}
	}

	// Store previous.
	if hasPrevious {
		checkErr(storeValue(previousContextKey, currentContext))
		checkErr(storeValue(previousNamespaceKey, currentNamespace))
	}
}

func printFavorites() {
	userHome, err := os.UserHomeDir()
	if err != nil {
		fail("Couldn't resolve user home dir")
	}

	files, err := os.ReadDir(path.Join(userHome, ".sk"))
	if err != nil {
		fail("Couldn't read sk dir")
	}

	type favorite struct {
		context   string
		namespace string
	}

	favorites := map[string]favorite{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fileName := file.Name()
		if strings.HasPrefix(fileName, favoriteContextKeyPrefix) {
			favoriteName := strings.TrimPrefix(fileName, favoriteContextKeyPrefix)
			c := readValue(fileName)
			f, ok := favorites[favoriteName]
			if !ok {
				favorites[favoriteName] = favorite{context: c, namespace: f.namespace}
			} else {
				favorites[favoriteName] = favorite{context: "", namespace: ""}
			}
		}

		if strings.HasPrefix(fileName, favoriteNamespaceKeyPrefix) {
			favoriteName := strings.TrimPrefix(fileName, favoriteNamespaceKeyPrefix)
			n := readValue(fileName)
			f, ok := favorites[favoriteName]
			if ok {
				favorites[favoriteName] = favorite{context: f.context, namespace: n}
			} else {
				favorites[favoriteName] = favorite{context: "", namespace: ""}
			}
		}
	}

	for k, v := range favorites {
		fmt.Printf("%s: %s/%s\n", k, v.context, v.namespace)
	}
}

func flagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func saveTermState() {
	oldState, err := term.GetState(int(os.Stdin.Fd()))
	if err != nil {
		return
	}
	termState = oldState
}

func restoreTermState() {
	if termState != nil {
		_ = term.Restore(int(os.Stdin.Fd()), termState)
	}
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

	_, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		fmt.Printf("Couldn't get terminal size: %s\n", err.Error())
		os.Exit(1)
	}

	p := prompt.New(
		executor,
		completer(suggestions),
		prompt.OptionPreviewSuggestionTextColor(prompt.Blue),
		prompt.OptionSelectedSuggestionBGColor(prompt.LightGray),
		prompt.OptionSuggestionBGColor(prompt.DarkGray),
		prompt.OptionMaxSuggestion(uint16(height-2)),
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
	restoreTermState()
	log.Fatal(msg)
}

func readValue(key string) string {
	userHome, err := os.UserHomeDir()
	if err != nil {
		fail("Couldn't resolve user home dir")
	}

	p := path.Join(userHome, ".sk", key)
	fileBytes, err := os.ReadFile(p)

	// Fine, simply no previous value stored
	if os.IsNotExist(err) {
		return ""
	}

	checkErr(err)

	return string(fileBytes)
}

func storeValue(key, value string) error {
	userHome, err := os.UserHomeDir()
	if err != nil {
		fail("Couldn't resolve user home dir")
	}

	p := path.Join(userHome, ".sk", key)

	// Create or truncate
	f, err := os.Create(p)
	if err != nil {
		return err
	}

	_, err = f.WriteString(value)
	return err
}

func createSkDir() error {
	userHome, err := os.UserHomeDir()
	if err != nil {
		fail("Couldn't resolve user home dir")
	}

	err = os.Mkdir(path.Join(userHome, ".sk"), os.ModePerm)
	if err == nil || strings.Contains(err.Error(), "file exists") {
		return nil
	}

	return err
}

func printCurrentContextAndNamespace(rawConfig api.Config) {
	currentContext := rawConfig.CurrentContext
	currentNamespace := rawConfig.Contexts[currentContext].Namespace
	fmt.Printf("Current context: %s\n", currentContext)
	fmt.Printf("Current namespace: %s\n", currentNamespace)
}
