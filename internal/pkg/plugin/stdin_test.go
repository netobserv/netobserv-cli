package plugin

import (
	"bufio"
	"io"
	"os"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"golang.org/x/term"
)

func TestExecInputReleasesTerminalForCopyPrompt(t *testing.T) {
	master, terminal, err := pty.Open()
	mustNoError(t, err)
	defer master.Close()
	defer terminal.Close()
	state, err := term.MakeRaw(int(terminal.Fd()))
	mustNoError(t, err)
	defer func() { mustNoError(t, term.Restore(int(terminal.Fd()), state)) }()

	input := newExecInput(terminal)
	defer input.stop()
	result := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, input)
		result <- err
	}()
	// Exercise cancellation while the reader is waiting for terminal input.
	time.Sleep(100 * time.Millisecond)
	input.stop()
	select {
	case err := <-result:
		mustNoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("remote stdin reader did not stop")
	}
	_, err = master.Write([]byte("no\n"))
	mustNoError(t, err)
	answer, err := bufio.NewReader(terminal).ReadString('\n')
	mustNoError(t, err)
	assert.Equal(t, "no\n", answer)
}

func TestExecInputForwardsDataAndEOF(t *testing.T) {
	read, write, err := os.Pipe()
	mustNoError(t, err)
	defer read.Close()
	defer write.Close()
	input := newExecInput(read)
	defer input.stop()
	_, err = write.Write([]byte("capture input"))
	mustNoError(t, err)
	mustNoError(t, write.Close())
	data, err := io.ReadAll(input)
	mustNoError(t, err)
	assert.Equal(t, "capture input", string(data))
}
