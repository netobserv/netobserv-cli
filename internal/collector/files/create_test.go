package files

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateCaptureCreatesRequestedFile(t *testing.T) {
	file, err := CreateCapture("unit-test", "capture.txt")
	require.NoError(t, err)
	path := file.Name()
	require.NoError(t, file.Close())
	t.Cleanup(func() { _ = os.Remove(path); _ = os.Remove(filepath.Dir(path)) })

	_, err = os.Stat(path)
	assert.NoError(t, err)
}

func TestCreateCaptureRejectsPathTraversal(t *testing.T) {
	file, err := CreateCapture("unit-test", "../outside.txt")
	if file != nil {
		_ = file.Close()
	}
	assert.Error(t, err)
}
