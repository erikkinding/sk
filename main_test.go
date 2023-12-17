package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStorePrevious(t *testing.T) {
	err := storePrevious("testing", "value1")
	assert.NoError(t, err)
}

func TestReadPrevious(t *testing.T) {
	v := "value1"
	err := storePrevious("testing", v)
	assert.NoError(t, err)

	previous := readPrevious("testing")
	assert.Equal(t, v, previous)
}
