package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"slices"
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
	skDir          = resolveSkDir()
	termState      *term.State
)

// Version is set at build time using -ldflags "-X main.version=1.0.0"
var version = "dev"

const (
	// previousStateFile holds "context\nnamespace" and is written atomically
	// (temp-file + rename) so a concurrent sk -p never sees a torn state.
	previousStateFile          = "previous_state"
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
	var printVersion bool
	var switchPrevious bool
	var nameSpaceMode bool
	var nameSpaceOnlyMode bool
	var printCurrent bool
	var listFavorites bool
	var favorite string

	flag.BoolVar(&printVersion, "v", false, "Print the current version")
	flag.BoolVar(&switchPrevious, "p", false, "Use to switch to the previously used context and namespace. Has no effect if state can't be retrieved.")
	flag.BoolVar(&nameSpaceMode, "n", false, "Select namespace from the ones available for the selected context")
	flag.BoolVar(&nameSpaceOnlyMode, "N", false, "Only select namespace from the ones available for the selected context")
	flag.BoolVar(&printCurrent, "c", false, "Print the currently selected context and namespace")
	flag.BoolVar(&listFavorites, "l", false, "List all stored favorites")
	flag.StringVar(&favorite, "f", "", "Select a favorite context")
	flag.StringVar(&favorite, "F", "", "Store current context and namespace as favorite")
	flag.Parse()

	// Allow bare "-" as a shorthand for -p (switch to previous context/namespace).
	if slices.Contains(flag.Args(), "-") {
		switchPrevious = true
	}

	if printVersion {
		fmt.Println(version)
		return
	}

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
		fmt.Println(favoriteContext)
		fmt.Println(favoriteNamespace)
		if favoriteContext != "" && favoriteNamespace != "" {
			rawConfig.CurrentContext = favoriteContext
			rawConfig.Contexts[favoriteContext].Namespace = favoriteNamespace
			setConfig(rawConfig)
		}
	} else if storeFavorite {
		checkErr(storeValue(fmt.Sprintf("%s%s", favoriteContextKeyPrefix, favorite), currentContext))
		checkErr(storeValue(fmt.Sprintf("%s%s", favoriteNamespaceKeyPrefix, favorite), currentNamespace))
	} else if switchPrevious {
		previousContext, previousNamespace := readPreviousState()
		if previousContext != "" {
			rawConfig.CurrentContext = previousContext
			rawConfig.Contexts[previousContext].Namespace = previousNamespace
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

	// Store previous only when context or namespace actually changed.
	// This prevents toggling to the same destination from clobbering the
	// stored previous state, and skips no-op invocations entirely.
	if hasPrevious {
		newConfig, err := loadConfig().RawConfig()
		checkErr(err)
		newContext := newConfig.CurrentContext
		var newNamespace string
		if newContext != "" {
			newNamespace = newConfig.Contexts[newContext].Namespace
		}
		if newContext != currentContext || newNamespace != currentNamespace {
			checkErr(storePreviousState(currentContext, currentNamespace))
		}
	}
}

func printFavorites() {
	files, err := os.ReadDir(skDir)
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

func getContextNames(rawConfig api.Config) []string {
	contexts := []string{}
	for context := range rawConfig.Contexts {
		// Current value on top
		if context == rawConfig.CurrentContext {
			contexts = append([]string{context}, contexts...)
		} else {
			contexts = append(contexts, context)
		}
	}
	return contexts
}

func applyContextChange(rawConfig api.Config, contextName string) error {
	if rawConfig.Contexts[contextName] == nil {
		return fmt.Errorf("context %q not found in kubeconfig", contextName)
	}
	rawConfig.CurrentContext = contextName
	setConfig(rawConfig)
	return nil
}

func selectContext(rawConfig api.Config) api.Config {
	contexts := getContextNames(rawConfig)

	selectedContext := showPrompt(contexts)

	if !validateSelection(contexts, selectedContext) {
		fail(fmt.Sprintf("'%s' is not a valid context selection", selectedContext))
	}

	checkErr(applyContextChange(rawConfig, selectedContext))
	rawConfig.CurrentContext = selectedContext

	return rawConfig
}

func listNamespaces(cfgPath string) ([]string, error) {
	restConfig, err := clientcmd.BuildConfigFromFlags("", cfgPath)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	nss, err := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(nss.Items))
	for _, ns := range nss.Items {
		names = append(names, ns.Name)
	}
	return names, nil
}

func applyNamespaceChange(rawConfig api.Config, contextName, namespaceName string) error {
	ctx, ok := rawConfig.Contexts[contextName]
	if !ok || ctx == nil {
		return fmt.Errorf("context %q not found in kubeconfig", contextName)
	}
	rawConfig.Contexts[contextName].Namespace = namespaceName
	setConfig(rawConfig)
	return nil
}

func selectNamespace(rawConfig api.Config) {
	selectedContext := rawConfig.CurrentContext

	allNs, err := listNamespaces(kubeConfigPath)
	checkErr(err)

	// Current value on top
	currentNamespace := rawConfig.Contexts[selectedContext].Namespace
	nsNames := []string{}
	for _, name := range allNs {
		if name == currentNamespace {
			nsNames = append([]string{name}, nsNames...)
		} else {
			nsNames = append(nsNames, name)
		}
	}

	nsSelection := showPrompt(nsNames)

	if !validateSelection(nsNames, nsSelection) {
		fail(fmt.Sprintf("'%s' is not a valid namespace selection", selectedContext))
	}

	checkErr(applyNamespaceChange(rawConfig, selectedContext, nsSelection))
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

func resolveSkDir() string {
	userHome, err := os.UserHomeDir()
	if err != nil {
		// best effort
		return ".sk"
	}
	return path.Join(userHome, ".sk")
}

func readValue(key string) string {
	p := path.Join(skDir, key)
	fileBytes, err := os.ReadFile(p)

	// Fine, simply no previous value stored
	if os.IsNotExist(err) {
		return ""
	}

	checkErr(err)

	return string(fileBytes)
}

func storeValue(key, value string) error {
	p := path.Join(skDir, key)

	// Create or truncate
	f, err := os.Create(p)
	if err != nil {
		return err
	}

	_, err = f.WriteString(value)
	if err != nil {
		f.Close()
		return err
	}

	return f.Close()
}

// storePreviousState writes ctx and ns as a single atomic operation so that a
// concurrent sk -p can never observe a torn state (new context + old namespace).
func storePreviousState(ctx, ns string) error {
	dest := path.Join(skDir, previousStateFile)

	// Write to a sibling temp file, then rename into place. On POSIX systems
	// rename(2) is atomic within the same filesystem, guaranteeing readers
	// always see either the old complete state or the new complete state.
	tmp, err := os.CreateTemp(skDir, previousStateFile+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	_, err = fmt.Fprintf(tmp, "%s\n%s", ctx, ns)
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err = tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}

	return os.Rename(tmpName, dest)
}

// readPreviousState returns the context and namespace stored by storePreviousState.
// Returns empty strings when no state has been stored yet.
// Falls back to the legacy two-file format (previous_context / previous_namespace)
// and migrates it to the new format on first read.
func readPreviousState() (ctx, ns string) {
	p := path.Join(skDir, previousStateFile)
	data, err := os.ReadFile(p)
	if err == nil {
		parts := strings.SplitN(string(data), "\n", 2)
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
	}
	if !os.IsNotExist(err) {
		checkErr(err)
	}

	// Legacy migration: read the old two-file format and promote to the new
	// atomic single-file format so the next read is already migrated.
	legacyCtx := readValue("previous_context")
	if legacyCtx == "" {
		return "", ""
	}
	legacyNs := readValue("previous_namespace")
	// Best-effort migration — ignore errors; the files will be rewritten on
	// the next successful switch anyway.
	_ = storePreviousState(legacyCtx, legacyNs)
	return legacyCtx, legacyNs
}

func createSkDir() error {
	err := os.Mkdir(skDir, os.ModePerm)
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
