//go:build integration

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/k3s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	ctxAlpha = "k3s-alpha"
	ctxBeta  = "k3s-beta"
	ctxGamma = "k3s-gamma"

	testNamespace1 = "test-ns-alpha"
	testNamespace2 = "test-ns-beta"
)

// baseKubeConfig is a multi-context kubeconfig built from the k3s container in TestMain.
// All contexts point to the same cluster, enabling context-switching tests without needing
// multiple real clusters while still exercising real kubeconfig file manipulation.
var baseKubeConfig []byte

// TestMain starts a k3s container once for the entire integration test suite, builds a
// multi-context kubeconfig for it, pre-creates test namespaces, then runs all tests.
func TestMain(m *testing.M) {
	ctx := context.Background()

	k3sContainer, err := k3s.Run(ctx, "rancher/k3s:latest")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start k3s container: %v\n", err)
		os.Exit(1)
	}

	rawKubeConfig, err := k3sContainer.GetKubeConfig(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get kubeconfig from k3s container: %v\n", err)
		k3sContainer.Terminate(ctx) //nolint:errcheck
		os.Exit(1)
	}

	cfg, err := clientcmd.Load(rawKubeConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse kubeconfig: %v\n", err)
		k3sContainer.Terminate(ctx) //nolint:errcheck
		os.Exit(1)
	}

	// The k3s container ships a single context named "default".  Clone its cluster
	// and auth-info into three distinctly-named contexts so that tests can exercise
	// the context-switching logic against a real kubeconfig file.
	defaultCtx := cfg.Contexts["default"]
	for _, name := range []string{ctxAlpha, ctxBeta, ctxGamma} {
		cfg.Contexts[name] = &api.Context{
			Cluster:   defaultCtx.Cluster,
			AuthInfo:  defaultCtx.AuthInfo,
			Namespace: "default",
		}
	}
	delete(cfg.Contexts, "default")
	cfg.CurrentContext = ctxAlpha

	baseKubeConfig, err = clientcmd.Write(*cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to serialise multi-context kubeconfig: %v\n", err)
		k3sContainer.Terminate(ctx) //nolint:errcheck
		os.Exit(1)
	}

	// Create a k8s client to pre-populate test namespaces.
	restCfg, err := clientcmd.RESTConfigFromKubeConfig(rawKubeConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build REST config: %v\n", err)
		k3sContainer.Terminate(ctx) //nolint:errcheck
		os.Exit(1)
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build k8s client: %v\n", err)
		k3sContainer.Terminate(ctx) //nolint:errcheck
		os.Exit(1)
	}

	for _, ns := range []string{testNamespace1, testNamespace2} {
		_, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		}, metav1.CreateOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create namespace %q: %v\n", ns, err)
			k3sContainer.Terminate(ctx) //nolint:errcheck
			os.Exit(1)
		}
	}

	exitCode := m.Run()

	k3sContainer.Terminate(ctx) //nolint:errcheck
	os.Exit(exitCode)
}

// setupTest prepares an isolated test environment:
//   - copies baseKubeConfig into a fresh temp file so each test starts from a known state,
//   - overrides the package-level kubeConfigPath and skDir globals,
//   - sets the KUBECONFIG env var so that setConfig() (which uses NewDefaultPathOptions)
//     writes to the same temp file.
//
// All changes are automatically reverted via t.Cleanup.
func setupTest(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	kubeconfigFile := filepath.Join(tmpDir, "kubeconfig")
	require.NoError(t, os.WriteFile(kubeconfigFile, baseKubeConfig, 0o600))

	tempSkDir := filepath.Join(tmpDir, ".sk")
	require.NoError(t, os.MkdirAll(tempSkDir, 0o700))

	origKubeConfigPath := kubeConfigPath
	origSkDir := skDir
	origEnv := os.Getenv("KUBECONFIG")

	kubeConfigPath = kubeconfigFile
	skDir = tempSkDir
	t.Setenv("KUBECONFIG", kubeconfigFile)

	t.Cleanup(func() {
		kubeConfigPath = origKubeConfigPath
		skDir = origSkDir
		os.Setenv("KUBECONFIG", origEnv) //nolint:errcheck
	})

	return kubeconfigFile
}

// captureStdout captures whatever fn writes to os.Stdout and returns it as a string.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	origStdout := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = origStdout
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	return buf.String()
}

// ── Context tests ─────────────────────────────────────────────────────────────

func TestGetContextNames_ListsAllContexts(t *testing.T) {
	setupTest(t)

	cfg, err := loadConfig().RawConfig()
	require.NoError(t, err)

	names := getContextNames(cfg)

	assert.ElementsMatch(t, []string{ctxAlpha, ctxBeta, ctxGamma}, names)
}

func TestGetContextNames_CurrentContextIsFirst(t *testing.T) {
	setupTest(t)

	cfg, err := loadConfig().RawConfig()
	require.NoError(t, err)

	names := getContextNames(cfg)

	// The current context (k3s-alpha) must appear at index 0 so the TUI
	// shows the already-selected value at the top of the list.
	assert.Equal(t, cfg.CurrentContext, names[0])
}

func TestApplyContextChange_SwitchesToTargetContext(t *testing.T) {
	setupTest(t)

	cfg, err := loadConfig().RawConfig()
	require.NoError(t, err)
	require.Equal(t, ctxAlpha, cfg.CurrentContext)

	require.NoError(t, applyContextChange(cfg, ctxBeta))

	updated, err := loadConfig().RawConfig()
	require.NoError(t, err)
	assert.Equal(t, ctxBeta, updated.CurrentContext)
}

func TestApplyContextChange_CyclesThroughAllContexts(t *testing.T) {
	setupTest(t)

	for _, target := range []string{ctxBeta, ctxGamma, ctxAlpha} {
		cfg, err := loadConfig().RawConfig()
		require.NoError(t, err)
		require.NoError(t, applyContextChange(cfg, target))

		updated, err := loadConfig().RawConfig()
		require.NoError(t, err)
		assert.Equal(t, target, updated.CurrentContext)
	}
}

func TestApplyContextChange_ReturnsErrorForUnknownContext(t *testing.T) {
	setupTest(t)

	cfg, err := loadConfig().RawConfig()
	require.NoError(t, err)

	err = applyContextChange(cfg, "does-not-exist")
	assert.Error(t, err)
}

// ── Namespace tests ───────────────────────────────────────────────────────────

func TestListNamespaces_ContainsBuiltinNamespaces(t *testing.T) {
	kubeconfigFile := setupTest(t)

	namespaces, err := listNamespaces(kubeconfigFile)
	require.NoError(t, err)

	for _, expected := range []string{"default", "kube-system", "kube-public"} {
		assert.Contains(t, namespaces, expected, "expected built-in namespace %q to be listed", expected)
	}
}

func TestListNamespaces_ContainsTestNamespaces(t *testing.T) {
	kubeconfigFile := setupTest(t)

	namespaces, err := listNamespaces(kubeconfigFile)
	require.NoError(t, err)

	assert.Contains(t, namespaces, testNamespace1)
	assert.Contains(t, namespaces, testNamespace2)
}

func TestApplyNamespaceChange_UpdatesNamespace(t *testing.T) {
	setupTest(t)

	cfg, err := loadConfig().RawConfig()
	require.NoError(t, err)

	require.NoError(t, applyNamespaceChange(cfg, ctxAlpha, "kube-system"))

	updated, err := loadConfig().RawConfig()
	require.NoError(t, err)
	assert.Equal(t, "kube-system", updated.Contexts[ctxAlpha].Namespace)
}

func TestApplyNamespaceChange_SurvivesContextSwitch(t *testing.T) {
	// Verify that namespace changes on one context don't affect another.
	setupTest(t)

	cfg, err := loadConfig().RawConfig()
	require.NoError(t, err)

	require.NoError(t, applyNamespaceChange(cfg, ctxAlpha, testNamespace1))
	require.NoError(t, applyNamespaceChange(cfg, ctxBeta, testNamespace2))

	updated, err := loadConfig().RawConfig()
	require.NoError(t, err)
	assert.Equal(t, testNamespace1, updated.Contexts[ctxAlpha].Namespace)
	assert.Equal(t, testNamespace2, updated.Contexts[ctxBeta].Namespace)
}

func TestApplyNamespaceChange_ReturnsErrorForUnknownContext(t *testing.T) {
	setupTest(t)

	cfg, err := loadConfig().RawConfig()
	require.NoError(t, err)

	err = applyNamespaceChange(cfg, "no-such-context", "default")
	assert.Error(t, err)
}

// ── Print current ─────────────────────────────────────────────────────────────

func TestPrintCurrentContextAndNamespace_ShowsContextAndNamespace(t *testing.T) {
	setupTest(t)

	cfg, err := loadConfig().RawConfig()
	require.NoError(t, err)

	// Put the cluster into a known state.
	require.NoError(t, applyContextChange(cfg, ctxBeta))
	cfg2, err := loadConfig().RawConfig()
	require.NoError(t, err)
	require.NoError(t, applyNamespaceChange(cfg2, ctxBeta, "kube-system"))

	cfg3, err := loadConfig().RawConfig()
	require.NoError(t, err)

	output := captureStdout(t, func() {
		printCurrentContextAndNamespace(cfg3)
	})

	assert.Contains(t, output, ctxBeta)
	assert.Contains(t, output, "kube-system")
}

// ── Previous context tracking ─────────────────────────────────────────────────

func TestPreviousContext_StoreAndRetrieve(t *testing.T) {
	setupTest(t)

	require.NoError(t, storeValue(previousContextKey, ctxBeta))
	require.NoError(t, storeValue(previousNamespaceKey, "kube-system"))

	assert.Equal(t, ctxBeta, readValue(previousContextKey))
	assert.Equal(t, "kube-system", readValue(previousNamespaceKey))
}

func TestPreviousContext_SwitchToPrevious(t *testing.T) {
	// Simulate the -p flag: switch to whatever was stored as previous.
	setupTest(t)

	// Store a "previous" state.
	require.NoError(t, storeValue(previousContextKey, ctxGamma))
	require.NoError(t, storeValue(previousNamespaceKey, testNamespace1))

	// Apply it (mirrors the main() -p code path).
	cfg, err := loadConfig().RawConfig()
	require.NoError(t, err)

	prevCtx := readValue(previousContextKey)
	prevNs := readValue(previousNamespaceKey)
	require.Equal(t, ctxGamma, prevCtx)
	require.Equal(t, testNamespace1, prevNs)

	require.NoError(t, applyContextChange(cfg, prevCtx))
	cfg2, err := loadConfig().RawConfig()
	require.NoError(t, err)
	require.NoError(t, applyNamespaceChange(cfg2, prevCtx, prevNs))

	cfg3, err := loadConfig().RawConfig()
	require.NoError(t, err)
	assert.Equal(t, ctxGamma, cfg3.CurrentContext)
	assert.Equal(t, testNamespace1, cfg3.Contexts[ctxGamma].Namespace)
}

func TestPreviousContext_EmptyWhenNeverStored(t *testing.T) {
	setupTest(t)

	assert.Equal(t, "", readValue(previousContextKey))
	assert.Equal(t, "", readValue(previousNamespaceKey))
}

// ── Favorites ─────────────────────────────────────────────────────────────────

func TestFavorite_StoreAndLoad(t *testing.T) {
	setupTest(t)

	favName := "staging"
	require.NoError(t, storeValue(favoriteContextKeyPrefix+favName, ctxBeta))
	require.NoError(t, storeValue(favoriteNamespaceKeyPrefix+favName, testNamespace2))

	assert.Equal(t, ctxBeta, readValue(favoriteContextKeyPrefix+favName))
	assert.Equal(t, testNamespace2, readValue(favoriteNamespaceKeyPrefix+favName))
}

func TestFavorite_OverwriteExisting(t *testing.T) {
	setupTest(t)

	favName := "env"
	require.NoError(t, storeValue(favoriteContextKeyPrefix+favName, ctxAlpha))
	require.NoError(t, storeValue(favoriteContextKeyPrefix+favName, ctxGamma))

	assert.Equal(t, ctxGamma, readValue(favoriteContextKeyPrefix+favName))
}

func TestFavorite_ApplyFavorite(t *testing.T) {
	// Full simulate of the -f flag: store a favorite then apply it.
	setupTest(t)

	favName := "prod"
	require.NoError(t, storeValue(favoriteContextKeyPrefix+favName, ctxGamma))
	require.NoError(t, storeValue(favoriteNamespaceKeyPrefix+favName, testNamespace2))

	favCtx := readValue(favoriteContextKeyPrefix + favName)
	favNs := readValue(favoriteNamespaceKeyPrefix + favName)
	require.Equal(t, ctxGamma, favCtx)
	require.Equal(t, testNamespace2, favNs)

	cfg, err := loadConfig().RawConfig()
	require.NoError(t, err)
	cfg.CurrentContext = favCtx
	cfg.Contexts[favCtx].Namespace = favNs
	setConfig(cfg)

	updated, err := loadConfig().RawConfig()
	require.NoError(t, err)
	assert.Equal(t, ctxGamma, updated.CurrentContext)
	assert.Equal(t, testNamespace2, updated.Contexts[ctxGamma].Namespace)
}

func TestListFavorites_PrintsAllStoredFavorites(t *testing.T) {
	setupTest(t)

	require.NoError(t, storeValue(favoriteContextKeyPrefix+"dev", ctxAlpha))
	require.NoError(t, storeValue(favoriteNamespaceKeyPrefix+"dev", "default"))
	require.NoError(t, storeValue(favoriteContextKeyPrefix+"prod", ctxBeta))
	require.NoError(t, storeValue(favoriteNamespaceKeyPrefix+"prod", testNamespace1))

	output := captureStdout(t, printFavorites)

	assert.Contains(t, output, "dev")
	assert.Contains(t, output, "prod")
}

func TestListFavorites_EmptyWhenNoneDefined(t *testing.T) {
	setupTest(t)

	output := captureStdout(t, printFavorites)

	assert.Empty(t, output)
}

// ── Selection validation ──────────────────────────────────────────────────────

func TestValidateSelection_AcceptsValidOptions(t *testing.T) {
	options := []string{ctxAlpha, ctxBeta, ctxGamma}
	for _, opt := range options {
		assert.True(t, validateSelection(options, opt), "expected %q to be valid", opt)
	}
}

func TestValidateSelection_RejectsUnknownOption(t *testing.T) {
	options := []string{ctxAlpha, ctxBeta, ctxGamma}
	assert.False(t, validateSelection(options, "not-in-list"))
	assert.False(t, validateSelection(options, ""))
}

// ── Full workflow integration ─────────────────────────────────────────────────

// TestFullWorkflow exercises the sequence a user would follow:
//  1. List contexts, switch to a different one.
//  2. List namespaces, switch to another namespace.
//  3. Print the current state.
//  4. Store the target as a favorite.
//  5. Switch to a third context, then restore via -p semantics.
//  6. Jump back to the stored favorite via -f semantics.
func TestFullWorkflow(t *testing.T) {
	kubeconfigFile := setupTest(t)

	// 1. List contexts and switch.
	cfg, err := loadConfig().RawConfig()
	require.NoError(t, err)
	assert.Equal(t, ctxAlpha, cfg.CurrentContext)

	names := getContextNames(cfg)
	assert.Len(t, names, 3)
	require.NoError(t, applyContextChange(cfg, ctxBeta))

	// 2. List namespaces from the live cluster and switch.
	namespaces, err := listNamespaces(kubeconfigFile)
	require.NoError(t, err)
	assert.Contains(t, namespaces, testNamespace1)

	cfg2, err := loadConfig().RawConfig()
	require.NoError(t, err)
	require.NoError(t, applyNamespaceChange(cfg2, ctxBeta, testNamespace1))

	// 3. Print current state.
	cfg3, err := loadConfig().RawConfig()
	require.NoError(t, err)
	output := captureStdout(t, func() { printCurrentContextAndNamespace(cfg3) })
	assert.Contains(t, output, ctxBeta)
	assert.Contains(t, output, testNamespace1)

	// 4. Store as favorite "myenv".
	require.NoError(t, storeValue(favoriteContextKeyPrefix+"myenv", ctxBeta))
	require.NoError(t, storeValue(favoriteNamespaceKeyPrefix+"myenv", testNamespace1))

	// 5. Switch away to ctxGamma and store ctxBeta as "previous".
	require.NoError(t, storeValue(previousContextKey, ctxBeta))
	require.NoError(t, storeValue(previousNamespaceKey, testNamespace1))

	cfg4, err := loadConfig().RawConfig()
	require.NoError(t, err)
	require.NoError(t, applyContextChange(cfg4, ctxGamma))

	// Restore previous (-p semantics).
	prevCtx := readValue(previousContextKey)
	prevNs := readValue(previousNamespaceKey)
	cfg5, err := loadConfig().RawConfig()
	require.NoError(t, err)
	require.NoError(t, applyContextChange(cfg5, prevCtx))
	cfg6, err := loadConfig().RawConfig()
	require.NoError(t, err)
	require.NoError(t, applyNamespaceChange(cfg6, prevCtx, prevNs))

	chk, err := loadConfig().RawConfig()
	require.NoError(t, err)
	assert.Equal(t, ctxBeta, chk.CurrentContext)
	assert.Equal(t, testNamespace1, chk.Contexts[ctxBeta].Namespace)

	// 6. Apply favorite (-f semantics).
	favCtx := readValue(favoriteContextKeyPrefix + "myenv")
	favNs := readValue(favoriteNamespaceKeyPrefix + "myenv")
	cfg7, err := loadConfig().RawConfig()
	require.NoError(t, err)
	cfg7.CurrentContext = favCtx
	cfg7.Contexts[favCtx].Namespace = favNs
	setConfig(cfg7)

	final, err := loadConfig().RawConfig()
	require.NoError(t, err)
	assert.Equal(t, ctxBeta, final.CurrentContext)
	assert.Equal(t, testNamespace1, final.Contexts[ctxBeta].Namespace)
}
