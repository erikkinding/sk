package main

import (
	"path/filepath"
	"testing"

	prompt "github.com/c-bata/go-prompt"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/tools/clientcmd/api"
)

func TestCreateSkDir_IdempotentOnRepeatCalls(t *testing.T) {
	tmpDir := t.TempDir()
	origSkDir := skDir
	skDir = filepath.Join(tmpDir, ".sk")
	t.Cleanup(func() { skDir = origSkDir })

	// First call creates the dir; second call must not return an error.
	assert.NoError(t, createSkDir())
	assert.NoError(t, createSkDir())
}

func TestGetContextNames_CurrentFirstThenAlphabetical(t *testing.T) {
	alphabetical := []string{
		"cluster-alpha",
		"cluster-beta/east",
		"cluster-beta/west",
		"cluster-gamma",
	}
	tests := []struct {
		currentContext string
		want           []string
	}{
		{"", alphabetical},
		{"cluster-alpha", alphabetical},
		{"cluster-beta/west", []string{
			"cluster-beta/west", "cluster-alpha", "cluster-beta/east", "cluster-gamma",
		}},
		{"cluster-gamma", []string{
			"cluster-gamma", "cluster-alpha", "cluster-beta/east", "cluster-beta/west",
		}},
		{"missing-context", alphabetical},
	}
	for _, tt := range tests {
		t.Run("current="+tt.currentContext, func(t *testing.T) {
			cfg := api.Config{
				CurrentContext: tt.currentContext,
				Contexts: map[string]*api.Context{
					"cluster-gamma":     {},
					"cluster-beta/west": {},
					"cluster-alpha":     {},
					"cluster-beta/east": {},
				},
			}

			assert.Equal(t, tt.want, getContextNames(cfg))
		})
	}
}

func TestGetContextNames_EmptyConfig(t *testing.T) {
	assert.Empty(t, getContextNames(api.Config{}))
	assert.Empty(t, getContextNames(api.Config{
		Contexts: map[string]*api.Context{},
	}))
}

func TestCompleter_MarksCurrentContextWithoutChangingItsName(t *testing.T) {
	suggestions := []string{"cluster-beta", "cluster-alpha"}

	got := completer(suggestions, "cluster-beta")(prompt.Document{})

	assert.Equal(t, []prompt.Suggest{
		{Text: "cluster-beta", Description: "*"},
		{Text: "cluster-alpha"},
	}, got)
}

func TestCompleter_FiltersByContextName(t *testing.T) {
	suggestions := []string{"cluster-beta", "cluster-alpha"}
	tests := []struct {
		query string
		want  []prompt.Suggest
	}{
		{"cb", []prompt.Suggest{{Text: "cluster-beta", Description: "*"}}},
		{"cal", []prompt.Suggest{{Text: "cluster-alpha"}}},
		{"*", []prompt.Suggest{}},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			buf := prompt.NewBuffer()
			buf.InsertText(tt.query, false, true)

			got := completer(suggestions, "cluster-beta")(*buf.Document())

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCompleter_NoMarkerWithoutCurrentContext(t *testing.T) {
	suggestions := []string{"cluster-alpha", "cluster-beta"}
	for _, currentContext := range []string{"", "missing-context"} {
		t.Run("current="+currentContext, func(t *testing.T) {
			got := completer(suggestions, currentContext)(prompt.Document{})

			assert.Equal(t, []prompt.Suggest{
				{Text: "cluster-alpha"},
				{Text: "cluster-beta"},
			}, got)
		})
	}
}
