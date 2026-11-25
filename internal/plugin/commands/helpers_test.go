package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPromptYesNoAcceptsTerminalEscapesBeforeAnswer(t *testing.T) {
	previous := stdin
	stdin = bytes.NewBufferString("\x1b[?25l\x1b[?25hyes\n")
	t.Cleanup(func() { stdin = previous })

	assert.True(t, promptYesNo(&bytes.Buffer{}, ""))
}

func TestPromptYesNoAcceptsControlCharactersAroundAnswer(t *testing.T) {
	previous := stdin
	stdin = bytes.NewBufferString("\x00yes\x00\n")
	t.Cleanup(func() { stdin = previous })

	assert.True(t, promptYesNo(&bytes.Buffer{}, ""))
}

func TestNewestJSONFile(t *testing.T) {
	dir := t.TempDir()
	flowDir := filepath.Join(dir, "flow")
	assert.NoError(t, os.MkdirAll(flowDir, 0o755))
	jsonPath := filepath.Join(flowDir, "2026-01-02.json")
	assert.NoError(t, os.WriteFile(jsonPath, []byte("[]"), 0o644))

	got, err := newestJSONFile(dir)
	assert.NoError(t, err)
	assert.Equal(t, jsonPath, got)
}
