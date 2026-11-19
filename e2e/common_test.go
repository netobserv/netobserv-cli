package e2e

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestRunCommandWithPTYInputSendsKeyboardData(t *testing.T) {
	output, err := RunCommandWithPTYInput(
		logrus.New().WithField("test", t.Name()),
		"/bin/sh",
		[]string{"-c", "printf ready; read value; printf received:$value"},
		PTYInput{Delay: 50 * time.Millisecond, Data: []byte("tui\n")},
	)

	require.NoError(t, err)
	require.Contains(t, output, "ready")
	require.Contains(t, output, "received:tui")
}
