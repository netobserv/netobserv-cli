//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	sigsyaml "sigs.k8s.io/yaml"

	agentconfig "github.com/netobserv/netobserv-ebpf-agent/pkg/config"
)

var (
	ylog = logrus.WithField("component", "yaml_test")
)

// splitYAMLDocs splits a `---`-separated multi-document YAML file into its documents, the same
// way the old bash-era tests did, since internal/k8s/client.YAMLWriter still separates
// documents with a bare "---" line.
func splitYAMLDocs(t *testing.T, yamlPath string) []string {
	t.Helper()
	data, err := os.ReadFile(yamlPath)
	require.NoError(t, err)
	return strings.Split(string(data), "---")
}

// findGeneratedYAML returns the single *_capture_*.yml file matching prefix under dirPath,
// replacing the filepath.Walk-based lookups every test below used to duplicate.
func findGeneratedYAML(t *testing.T, dirPath, prefix string) string {
	t.Helper()
	var matches []string
	err := filepath.Walk(dirPath, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.Contains(p, prefix) && filepath.Ext(p) == ".yml" {
			matches = append(matches, p)
		}
		return nil
	})
	require.NoError(t, err)
	require.Len(t, matches, 1, "expected exactly one %s*.yml file under %s", prefix, dirPath)
	return matches[0]
}

// findDocByKind returns the first document whose `kind:` matches, decoded as a generic map for
// field-order-independent inspection.
func findDocByKind(t *testing.T, docs []string, kind string) map[string]any {
	t.Helper()
	for _, doc := range docs {
		if strings.TrimSpace(doc) == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, sigsyaml.Unmarshal([]byte(doc), &m))
		// Decode-then-compare, rather than a raw "kind: X" substring match: "kind: Service"
		// would also match a "kind: ServiceAccount" document, since the latter's text contains
		// the former as a substring.
		if m["kind"] == kind {
			return m
		}
	}
	t.Fatalf("no document with kind %q found", kind)
	return nil
}

// decodeDaemonSet finds and fully decodes the DaemonSet document into a typed object, so this
// package can inspect its env vars/volumes/securityContext exactly like
// internal/plugin/manifests/agent's own unit tests do, rather than pattern-matching raw YAML text whose
// field order depends on internal/k8s/client.ToUnstructured's map-based round trip.
func decodeDaemonSet(t *testing.T, docs []string) *appsv1.DaemonSet {
	t.Helper()
	m := findDocByKind(t, docs, "DaemonSet")
	data, err := sigsyaml.Marshal(m)
	require.NoError(t, err)
	var ds appsv1.DaemonSet
	require.NoError(t, sigsyaml.Unmarshal(data, &ds))
	return &ds
}

func envVar(t *testing.T, ds *appsv1.DaemonSet, name string) string {
	t.Helper()
	require.NotEmpty(t, ds.Spec.Template.Spec.Containers)
	for _, e := range ds.Spec.Template.Spec.Containers[0].Env {
		if e.Name == name {
			return e.Value
		}
	}
	t.Fatalf("env var %q not found", name)
	return ""
}

func optionalEnvVar(ds *appsv1.DaemonSet, name string) string {
	if len(ds.Spec.Template.Spec.Containers) == 0 {
		return ""
	}
	for _, env := range ds.Spec.Template.Spec.Containers[0].Env {
		if env.Name == name {
			return env.Value
		}
	}
	return ""
}

func flowFilters(t *testing.T, ds *appsv1.DaemonSet) []agentconfig.FlowFilter {
	t.Helper()
	var filters []agentconfig.FlowFilter
	require.NoError(t, json.Unmarshal([]byte(envVar(t, ds, "FLOW_FILTER_RULES")), &filters))
	return filters
}

func assertCommonSetupMessages(t *testing.T, output string, agentsLabel string) {
	t.Helper()
	assert.Contains(t, output, "creating netobserv-cli namespace")
	assert.Contains(t, output, "creating service account")
	assert.Contains(t, output, fmt.Sprintf("creating %s agents", agentsLabel))
	assert.Contains(t, output, "Check the generated YAML file in output folder")
}

func TestFlowFiltersYAML(t *testing.T) {
	f1 := features.New("flow yaml").Setup(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			output, err := RunCommand(ylog, "commands/oc-netobserv", "flows",
				"--protocol=TCP",
				"--port=8080",
				"or",
				"--protocol=UDP",
				"--yaml")
			assert.Nil(t, err)

			err = os.WriteFile(path.Join("output", StartupDate+"-flowYAMLOutput"), []byte(output), 0666)
			assert.Nil(t, err)

			assert.NotEmpty(t, output)
			assertCommonSetupMessages(t, output, "flow-capture")
			assert.Contains(t, output, "creating collector service")

			return ctx
		},
	).Assess("check generated yaml output",
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			assert.True(t, dirExists("output"))
			yamlPath := findGeneratedYAML(t, "output", "flows_capture")
			docs := splitYAMLDocs(t, yamlPath)
			require.Len(t, docs, 10)

			ns := findDocByKind(t, docs, "Namespace")
			assert.Equal(t, "netobserv-cli", metadataField(t, ns, "name"))
			assert.Equal(t, "privileged", nestedField(t, ns, "metadata", "labels", "pod-security.kubernetes.io/enforce"))

			svc := findDocByKind(t, docs, "Service")
			assert.Equal(t, "collector", metadataField(t, svc, "name"))

			ds := decodeDaemonSet(t, docs)
			assert.Equal(t, "netobserv-cli", ds.Namespace)

			filters := flowFilters(t, ds)
			require.Len(t, filters, 2)
			assert.Equal(t, "TCP", filters[0].Protocol)
			assert.Equal(t, int32(8080), filters[0].Port)
			assert.Equal(t, "UDP", filters[1].Protocol)

			require.NotEmpty(t, ds.Spec.Template.Spec.Containers)
			assert.Contains(t, ds.Spec.Template.Spec.Containers[0].Image, "netobserv-ebpf-agent")

			return ctx
		},
	).Feature()
	testCluster.TestEnv().Test(t, f1)
}

func TestPacketFiltersYAML(t *testing.T) {
	f1 := features.New("packet yaml").Setup(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			output, err := RunCommand(ylog, "commands/oc-netobserv", "packets",
				"--node-selector=netobserv:true",
				"--port=80",
				"--yaml")
			assert.Nil(t, err)

			err = os.WriteFile(path.Join("output", StartupDate+"-packetYAMLOutput"), []byte(output), 0666)
			assert.Nil(t, err)

			assert.NotEmpty(t, output)
			assertCommonSetupMessages(t, output, "packet-capture")
			assert.Contains(t, output, "creating collector service")

			return ctx
		},
	).Assess("check generated yaml output",
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			assert.True(t, dirExists("output"))
			yamlPath := findGeneratedYAML(t, "output", "packets_capture")
			docs := splitYAMLDocs(t, yamlPath)
			require.Len(t, docs, 10)

			ds := decodeDaemonSet(t, docs)
			filters := flowFilters(t, ds)
			require.Len(t, filters, 1)
			assert.Equal(t, int32(80), filters[0].Port)
			assert.Equal(t, "true", ds.Spec.Template.Spec.NodeSelector["netobserv"])

			return ctx
		},
	).Feature()
	testCluster.TestEnv().Test(t, f1)
}

func TestPacketOpenSSLYAML(t *testing.T) {
	// Run from a temp dir so output is isolated from other packet YAML tests.
	origDir, err := os.Getwd()
	assert.Nil(t, err)
	absCmd, err := filepath.Abs("commands/oc-netobserv")
	assert.Nil(t, err)
	tmpDir := t.TempDir()
	assert.Nil(t, os.Chdir(tmpDir))
	defer os.Chdir(origDir)

	f1 := features.New("packet openssl yaml").Setup(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			output, err := RunCommand(ylog, absCmd, "packets",
				"--port=8443",
				"--enable_openssl",
				"--tls_process_allowlist=nginx",
				"--yaml")
			assert.Nil(t, err)

			assert.NotEmpty(t, output)
			assert.Contains(t, output, "creating packet-capture agents")
			assert.Contains(t, output, "Check the generated YAML file in output folder")

			return ctx
		},
	).Assess("check generated yaml has openssl config",
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			assert.True(t, dirExists("output"))
			yamlPath := findGeneratedYAML(t, "output", "packets_capture")
			docs := splitYAMLDocs(t, yamlPath)
			ds := decodeDaemonSet(t, docs)

			assert.Equal(t, "true", envVar(t, ds, "ENABLE_OPENSSL_TRACKING"))
			assert.Equal(t, "nginx", envVar(t, ds, "TLS_PLAINTEXT_PROCESS_ALLOWLIST"))

			require.NotEmpty(t, ds.Spec.Template.Spec.Containers)
			c := ds.Spec.Template.Spec.Containers[0]
			require.NotNil(t, c.SecurityContext)
			require.NotNil(t, c.SecurityContext.Privileged)
			assert.True(t, *c.SecurityContext.Privileged)
			assert.True(t, ds.Spec.Template.Spec.HostPID)

			var hasHostUsr bool
			for _, v := range ds.Spec.Template.Spec.Volumes {
				if v.Name == "host-usr" {
					hasHostUsr = true
				}
			}
			assert.True(t, hasHostUsr, "expected a host-usr volume for libssl discovery")

			var hasSysPtrace bool
			if c.SecurityContext.Capabilities != nil {
				for _, cap := range c.SecurityContext.Capabilities.Add {
					if cap == "SYS_PTRACE" {
						hasSysPtrace = true
					}
				}
			}
			assert.True(t, hasSysPtrace)

			filters := flowFilters(t, ds)
			require.Len(t, filters, 1)
			assert.Equal(t, int32(8443), filters[0].Port)

			return ctx
		},
	).Feature()
	testCluster.TestEnv().Test(t, f1)
}

// test metrics only as YAML output as kind can't manage ServiceMonitor CR
func TestMetricYAML(t *testing.T) {
	f1 := features.New("metric yaml").Setup(
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			output, err := RunCommand(ylog, "commands/oc-netobserv", "metrics", "--yaml")
			assert.Nil(t, err)

			err = os.WriteFile(path.Join("output", StartupDate+"-metricYAMLOutput"), []byte(output), 0666)
			assert.Nil(t, err)

			assert.NotEmpty(t, output)
			assertCommonSetupMessages(t, output, "metric-capture")
			assert.Contains(t, output, "creating service monitor")

			return ctx
		},
	).Assess("check generated yaml output",
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			assert.True(t, dirExists("output"))
			yamlPath := findGeneratedYAML(t, "output", "metrics_capture")
			docs := splitYAMLDocs(t, yamlPath)
			require.Len(t, docs, 14)

			sm := findDocByKind(t, docs, "ServiceMonitor")
			assert.Equal(t, "netobserv-cli", metadataField(t, sm, "name"))

			dashboardCM := findDocByKind(t, docs, "ConfigMap")
			assert.Equal(t, "openshift-config-managed", metadataField(t, dashboardCM, "namespace"))

			ds := decodeDaemonSet(t, docs)
			assert.NotEqual(t, "true", optionalEnvVar(ds, "ENABLE_PKT_DROPS"))

			return ctx
		},
	).Feature()
	testCluster.TestEnv().Test(t, f1)
}

func metadataField(t *testing.T, doc map[string]any, field string) string {
	t.Helper()
	return nestedField(t, doc, "metadata", field)
}

// nestedField walks a chain of map keys in a generically-decoded YAML document, replacing the
// old Normalize()-and-substring-match approach: field order in internal/k8s/client's
// unstructured round trip is not meaningful, so assertions here check values, not raw text.
func nestedField(t *testing.T, doc map[string]any, path ...string) string {
	t.Helper()
	var cur any = doc
	for _, key := range path {
		m, ok := cur.(map[string]any)
		require.True(t, ok, "path %v: %q is not a map", path, key)
		cur, ok = m[key]
		require.True(t, ok, "path %v: key %q not found", path, key)
	}
	s, ok := cur.(string)
	require.True(t, ok, "path %v: value is not a string (%T)", path, cur)
	return s
}
