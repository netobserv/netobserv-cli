//go:build e2e

package e2e

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/netobserv/network-observability-cli/e2e/cluster"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	"github.com/sirupsen/logrus"
)

const (
	clusterNamePrefix = "netobserv-cli-e2e-test-cluster"
	namespace         = "default"
	ExportLogsTimeout = 30 * time.Second
)

var (
	testCluster *cluster.Kind
	clog        = logrus.WithField("component", "capture_test")
)

func TestMain(m *testing.M) {
	if os.Getenv("ACTIONS_RUNNER_DEBUG") == "true" {
		logrus.StandardLogger().SetLevel(logrus.DebugLevel)
	}
	testCluster = cluster.NewKind(
		clusterNamePrefix+StartupDate,
		path.Join(".."),
	)
	testCluster.Run(m)
}

func TestFlowCapture(t *testing.T) {
	f1 := features.New("flow capture").Setup(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			timer := time.AfterFunc(ExportLogsTimeout, func() {
				agentLogs := testCluster.GetAgentLogs()
				err := os.WriteFile(path.Join(testCluster.GetLogsDir(), StartupDate+"-flowAgentLogs"), []byte(agentLogs), 0666)
				assert.Nil(t, err)
			})
			defer timer.Stop()

			output, err := RunCommandWithPTYInput(clog, "commands/oc-netobserv", []string{"flows", "--log-level=trace"}, PTYInput{Delay: 20 * time.Second, Data: []byte{0}}, PTYInput{Delay: 2 * time.Second, Data: []byte{9}}, PTYInput{Delay: 1 * time.Second, Data: []byte{27}})
			// TODO: find a way to avoid error here; this is probably related to SIGTERM instead of CTRL + C call
			//assert.Nil(t, err)

			err = os.WriteFile(path.Join("output", StartupDate+"-flowOutput"), []byte(output), 0666)
			assert.Nil(t, err)

			assert.NotEmpty(t, output)
			// ensure setup is fine
			assert.Contains(t, output, "creating netobserv-cli namespace")
			assert.Contains(t, output, "creating service account")
			assert.Contains(t, output, "creating collector service")
			assert.Contains(t, output, "creating flow-capture agents")
			assert.Contains(t, output, "Running collector...")
			assert.Contains(t, output, "Executing collector command...")
			// check that the in-pod CLI is running (unchanged in this rewrite)
			assert.Contains(t, output, "Starting Flow Capture...")
			assert.Contains(t, output, "Flows")
			assert.Contains(t, output, "Table refresh is paused")
			return ctx
		},
	).Assess("check downloaded output flow files",
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			var jsons []string
			var dbs []string

			dirPath := path.Join("output", "flow")
			assert.True(t, dirExists(dirPath), "directory %s not found", dirPath)
			err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if !info.IsDir() {
					if filepath.Ext(path) == ".json" {
						jsons = append(jsons, path)
					} else if filepath.Ext(path) == ".db" {
						dbs = append(dbs, path)
					}
				}

				return nil
			})
			assert.Nil(t, err)

			// check json file
			require.Len(t, jsons, 1)
			jsonBytes, err := os.ReadFile(jsons[0])
			assert.Nil(t, err)
			assert.Contains(t, string(jsonBytes), "AgentIP")

			// check db file
			require.Len(t, dbs, 1)
			dbBytes, err := os.ReadFile(dbs[0])
			assert.Nil(t, err)
			assert.Contains(t, string(dbBytes), "SQLite format")
			return ctx
		},
	).Feature()
	testCluster.TestEnv().Test(t, f1)
}

func TestPacketCapture(t *testing.T) {
	f1 := features.New("packet capture").Setup(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			timer := time.AfterFunc(ExportLogsTimeout, func() {
				agentLogs := testCluster.GetAgentLogs()
				err := os.WriteFile(path.Join(testCluster.GetLogsDir(), StartupDate+"-packetAgentLogs"), []byte(agentLogs), 0666)
				assert.Nil(t, err)
			})
			defer timer.Stop()

			output, err := RunCommandWithPTYInput(clog, "commands/oc-netobserv", []string{"packets", "--log-level=trace", "--protocol=TCP", "--port=6443"}, PTYInput{Delay: 20 * time.Second, Data: []byte{0}}, PTYInput{Delay: 2 * time.Second, Data: []byte{9}}, PTYInput{Delay: 1 * time.Second, Data: []byte{27}})
			// TODO: find a way to avoid error here; this is probably related to SIGTERM instead of CTRL + C call
			//assert.Nil(t, err)

			err = os.WriteFile(path.Join("output", StartupDate+"-packetOutput"), []byte(output), 0666)
			assert.Nil(t, err)

			assert.NotEmpty(t, output)
			// ensure setup is fine
			assert.Contains(t, output, "creating netobserv-cli namespace")
			assert.Contains(t, output, "creating service account")
			assert.Contains(t, output, "creating collector service")
			assert.Contains(t, output, "creating packet-capture agents")
			assert.Contains(t, output, "Running collector...")
			assert.Contains(t, output, "Executing collector command...")
			// check that the in-pod CLI is running (unchanged in this rewrite)
			assert.Contains(t, output, "Starting Packet Capture...")
			assert.Contains(t, output, "Flows")
			assert.Contains(t, output, "Table refresh is paused")
			return ctx
		},
	).Assess("check downloaded output pcap files",
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			var pcaps []string

			dirPath := path.Join("output", "pcap")
			assert.True(t, dirExists(dirPath), "directory %s not found", dirPath)
			err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if !info.IsDir() {
					if filepath.Ext(path) == ".pcapng" {
						pcaps = append(pcaps, path)
					}
				}

				return nil
			})
			assert.Nil(t, err)

			// check pcap file
			require.Len(t, pcaps, 1)
			pcapBytes, err := os.ReadFile(pcaps[0])
			assert.Nil(t, err)

			// check pcap magic number
			assert.Equal(t, []byte{0x4d, 0x3c, 0x2b, 0x1a}, pcapBytes[8:12])

			return ctx
		},
	).Feature()
	testCluster.TestEnv().Test(t, f1)
}

func dirExists(dir string) bool {
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}
