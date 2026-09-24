//go:build e2e

package e2e

import (
	"os"
	"path"
	"regexp"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

var (
	slog = logrus.WithField("component", "command_test")
)

func TestHelpCommand(t *testing.T) {
	t.Run("help command", func(t *testing.T) {
		output, err := RunCommand(slog, "commands/oc-netobserv", "help")
		assert.Nil(t, err)

		err = os.WriteFile(path.Join("output", StartupDate+"-helpOutput"), []byte(output), 0666)
		assert.Nil(t, err)

		assert.NotEmpty(t, output)
		// ensure help display overall description
		assert.Contains(t, output, "NetObserv allows you to capture flows, packets and metrics from your cluster.")
		assert.Contains(t, output, "Find more information at: https://github.com/netobserv/network-observability-cli/")
		// ensure help to display main commands
		assert.Contains(t, output, "Main commands:")
		assert.Contains(t, output, "Syntax: netobserv [flows|packets|metrics|follow|stop|copy|cleanup|version] [options]")
		assert.Contains(t, output, "flows      Capture flows information in JSON format using collector pod.")
		assert.Contains(t, output, "metrics    Capture metrics information in Prometheus using a ServiceMonitor (OCP cluster only).")
		assert.Contains(t, output, "packets    Capture packets information in pcap format using collector pod.")
		// ensure help to display extra commands
		assert.Contains(t, output, "Extra commands:")
		assert.Contains(t, output, "cleanup    Remove netobserv components and configurations.")
		assert.Contains(t, output, "copy       Copy collector generated files locally.")
		assert.Contains(t, output, "follow     Follow collector logs when running in background.")
		assert.Contains(t, output, "stop       Stop collection by removing agent daemonset.")
		assert.Contains(t, output, "version    Print software version.")
		// ensure help to display --help hint
		assert.Contains(t, output, "Use --help with any command for more details")
		// ensure help to display examples
		assert.Contains(t, output, "Flow capture examples:")
		assert.Contains(t, output, "netobserv flows --drops")
		assert.Contains(t, output, "netobserv packets --port=8080")
		assert.Contains(t, output, "netobserv metrics --enable_all")
		assert.Contains(t, output, "Packet capture examples:")
		assert.Contains(t, output, "Capture flows in the background")
		assert.Contains(t, output, "Capture metrics in the background")
	})

	t.Run("no argument shows help", func(t *testing.T) {
		output, err := RunCommand(slog, "commands/oc-netobserv")
		assert.Nil(t, err)
		assert.NotEmpty(t, output)
		assert.Contains(t, output, "Main commands:")
	})
}

func TestSubcommandHelp(t *testing.T) {
	t.Run("flows --help", func(t *testing.T) {
		output, err := RunCommand(slog, "commands/oc-netobserv", "flows", "--help")
		assert.Nil(t, err)
		assert.NotEmpty(t, output)
		plain := ansiRegex.ReplaceAllString(output, "")
		assert.Contains(t, plain, "Syntax: netobserv flows [options]")
		assert.Contains(t, plain, "Features:")
		assert.Contains(t, plain, "Filters:")
		assert.Contains(t, plain, "Options:")
		assert.Contains(t, plain, "Examples:")
	})

	t.Run("flows --help at any position", func(t *testing.T) {
		output, err := RunCommand(slog, "commands/oc-netobserv", "flows", "--port=8080", "--help")
		assert.Nil(t, err)
		assert.NotEmpty(t, output)
		plain := ansiRegex.ReplaceAllString(output, "")
		assert.Contains(t, plain, "Syntax: netobserv flows [options]")
		assert.Contains(t, plain, "Filters:")
	})

	t.Run("packets --help", func(t *testing.T) {
		output, err := RunCommand(slog, "commands/oc-netobserv", "packets", "--help")
		assert.Nil(t, err)
		assert.NotEmpty(t, output)
		plain := ansiRegex.ReplaceAllString(output, "")
		assert.Contains(t, plain, "Syntax: netobserv packets [options]")
	})

	t.Run("metrics --help", func(t *testing.T) {
		output, err := RunCommand(slog, "commands/oc-netobserv", "metrics", "--help")
		assert.Nil(t, err)
		assert.NotEmpty(t, output)
		plain := ansiRegex.ReplaceAllString(output, "")
		assert.Contains(t, plain, "Syntax: netobserv metrics [options]")
	})
}

func TestVersionCommand(t *testing.T) {
	t.Run("version command", func(t *testing.T) {
		output, err := RunCommand(slog, "commands/oc-netobserv", "version")
		assert.Nil(t, err)

		err = os.WriteFile(path.Join("output", StartupDate+"-versionOutput"), []byte(output), 0666)
		assert.Nil(t, err)

		assert.NotEmpty(t, output)
		// ensure version display test
		assert.Contains(t, output, "NetObserv CLI version test")
	})
}
