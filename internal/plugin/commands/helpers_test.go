package commands

import (
	"bytes"
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
