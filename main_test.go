package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
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
