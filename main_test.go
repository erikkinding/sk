package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateMultiplSkDirs(t *testing.T) {
	err := createSkDir()
	assert.NoError(t, err)
	err = createSkDir()
	assert.NoError(t, err)
}
