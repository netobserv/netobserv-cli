//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/gopacket/gopacket/pcapgo"
	"github.com/netobserv/network-observability-cli/e2e/cluster"
	"github.com/stretchr/testify/assert"
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
		func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			timer := time.AfterFunc(ExportLogsTimeout, func() {
				agentLogs := testCluster.GetAgentLogs()
				err := os.WriteFile(path.Join(testCluster.GetLogsDir(), StartupDate+"-flowAgentLogs"), []byte(agentLogs), 0666)
				assert.Nil(t, err)
			})
			defer timer.Stop()

			output, err := RunCommand(clog, "commands/oc-netobserv", "flows", "--log-level=trace", "--sampling=1", "--headless", "--max-time=20s", "--copy=true")
			// Wait for the collection deadline and output flush before copying.
			if !assert.NoError(t, err, output) {
				t.FailNow()
			}

			err = os.WriteFile(path.Join("output", StartupDate+"-flowOutput"), []byte(output), 0666)
			assert.Nil(t, err)

			assert.NotEmpty(t, output)
			// ensure script setup is fine
			assert.Contains(t, output, "namespace/netobserv-cli created")
			assert.Contains(t, output, "serviceaccount/netobserv-cli created")
			assert.Contains(t, output, "service/collector created")
			assert.Contains(t, output, "daemonset.apps/netobserv-cli created")
			assert.Contains(t, output, "pod/collector created")
			assert.Contains(t, output, "pod/collector condition met")
			// check that CLI is running
			assert.Contains(t, output, "Starting Flow Capture...")
			assert.Contains(t, output, "Started collector")
			// Successful completion must include reaching the collection limit.
			assert.Contains(t, output, "exiting collection")
			return ctx
		},
	).Assess("check downloaded output flow files",
		func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			var jsons []string
			var dbs []string

			dirPath := path.Join("output", "flow")
			assert.True(t, dirExists(dirPath), "directory %s not found", dirPath)
			err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					fmt.Println(err)
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
			if !assert.Len(t, jsons, 1) {
				t.FailNow()
			}
			jsonBytes, err := os.ReadFile(jsons[0])
			assert.Nil(t, err)
			assert.Contains(t, string(jsonBytes), "AgentIP")

			// check db file
			if !assert.Len(t, dbs, 1) {
				t.FailNow()
			}
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
		func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			timer := time.AfterFunc(ExportLogsTimeout, func() {
				agentLogs := testCluster.GetAgentLogs()
				err := os.WriteFile(path.Join(testCluster.GetLogsDir(), StartupDate+"-packetAgentLogs"), []byte(agentLogs), 0666)
				assert.Nil(t, err)
			})
			defer timer.Stop()

			output, err := RunCommand(clog, "commands/oc-netobserv", "packets", "--log-level=trace", "--protocol=TCP", "--port=6443", "--headless", "--max-time=20s", "--copy=true")
			// Wait for the collection deadline and output flush before copying.
			if !assert.NoError(t, err, output) {
				t.FailNow()
			}

			err = os.WriteFile(path.Join("output", StartupDate+"-packetOutput"), []byte(output), 0666)
			assert.Nil(t, err)

			assert.NotEmpty(t, output)
			// ensure script setup is fine
			assert.Contains(t, output, "namespace/netobserv-cli created")
			assert.Contains(t, output, "serviceaccount/netobserv-cli created")
			assert.Contains(t, output, "service/collector created")
			assert.Contains(t, output, "daemonset.apps/netobserv-cli created")
			assert.Contains(t, output, "pod/collector created")
			assert.Contains(t, output, "pod/collector condition met")
			// check that CLI is running
			assert.Contains(t, output, "Starting Packet Capture...")
			assert.Contains(t, output, "Started collector")
			// Successful completion must include reaching the collection limit.
			assert.Contains(t, output, "exiting collection")
			return ctx
		},
	).Assess("check downloaded output pcap files",
		func(ctx context.Context, t *testing.T, _ *envconf.Config) context.Context {
			var pcaps []string

			dirPath := path.Join("output", "pcap")
			assert.True(t, dirExists(dirPath), "directory %s not found", dirPath)
			err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					fmt.Println(err)
				}

				if !info.IsDir() {
					if filepath.Ext(path) == ".pcapng" {
						pcaps = append(pcaps, path)
					}
				}

				return nil
			})
			assert.Nil(t, err)

			// Decode the complete file: a magic number alone misses truncation.
			if !assert.Len(t, pcaps, 1) {
				t.FailNow()
			}
			f, err := os.Open(pcaps[0])
			if !assert.NoError(t, err) {
				t.FailNow()
			}
			defer f.Close()
			reader, err := pcapgo.NewNgReader(f, pcapgo.DefaultNgReaderOptions)
			if !assert.NoError(t, err) {
				t.FailNow()
			}
			packets := 0
			for {
				_, _, err = reader.ReadPacketData()
				if errors.Is(err, io.EOF) {
					break
				}
				if !assert.NoError(t, err) {
					t.FailNow()
				}
				packets++
			}
			if !assert.Positive(t, packets, "expected API traffic in capture") {
				t.FailNow()
			}

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
