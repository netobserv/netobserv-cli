package commands

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaptureCommandsAcceptPositionalHelp(t *testing.T) {
	for _, command := range []string{"flows", "packets", "metrics"} {
		t.Run(command, func(t *testing.T) {
			root := NewRootCommand("oc-netobserv", "test", []string{command, "help"})
			root.SetArgs([]string{command, "help"})
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetErr(&output)

			require.NoError(t, root.Execute())
			assert.Contains(t, output.String(), "Usage:")
			assert.Contains(t, output.String(), "netobserv "+command)
			assert.NotContains(t, output.String(), "creating netobserv-cli namespace")
		})
	}
}

func TestMetricsDoesNotOfferOutputCopy(t *testing.T) {
	root := NewRootCommand("oc-netobserv", "test")
	metrics, _, err := root.Find([]string{"metrics"})
	require.NoError(t, err)
	require.NotNil(t, metrics)

	assert.Nil(t, metrics.Flags().Lookup("copy"))
}
