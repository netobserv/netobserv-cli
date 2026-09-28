package plugin

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/yaml"
)

// These fixed fixtures preserve output from the retired Bash implementation.
// Keep them independent of the Go generator to detect compatibility regressions.
func TestBashManifestParity(t *testing.T) {
	t.Setenv("NETOBSERV_NAMESPACE", "")
	t.Setenv("NETOBSERV_AGENT_IMAGE", "")
	cases := []struct {
		name, mode string
		args       []string
	}{
		{"flows-default", "flows", nil},
		{"packets", "packets", []string{"--protocol=TCP", "--port=8080", "or", "--protocol=UDP", "--dport=53"}},
		{"filters", "flows", []string{"--direction=Ingress", "--cidr=10.0.0.0/8", "--protocol=TCP", "--dport=443", "--port=80", "--sport_range=1000-2000", "--dport_range=80-90", "--port_range=100-200", "--sports=80,443", "--dports=53,54", "--ports=22,23", "--icmp_type=8", "--icmp_code=0", "--peer_ip=10.1.1.1", "--peer_cidr=10.2.0.0/16", "--action=Reject", "--tcp_flags=SYN", "--drops"}},
		{"features", "flows", []string{"--enable_all", "--enable_ipsec=false", "--sampling=50", "--interfaces=eth0,eth1", "--exclude_interfaces=lo", "--node-selector=kubernetes.io/hostname:node1", "--log-level=debug"}},
		{"repeated", "flows", []string{"--enable_dns", "--enable_dns=false", "--enable_all=false", "--privileged", "--privileged=false", "--drops", "--drops=false"}},
		{"queries", "flows", []string{`--query=SrcK8S_Namespace=~"app-.*"`, `--query=DstPort=443`}},
		{"metrics-default", "metrics", nil},
		{"metrics-all", "metrics", []string{"--enable_all"}},
		{"metrics-selected", "metrics", []string{"--enable_dns", "--include_list=node", "--enable_rtt", "--enable_pkt_drop", "--enable_network_events"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := filepath.Join("..", "..", "..", "e2e", "testdata", tc.name+".yaml")

			b, err := os.ReadFile(fixture)
			mustNoError(t, err)
			var expected appsv1.DaemonSet
			mustNoError(t, yaml.Unmarshal(b, &expected))
			o, err := parseOptions(tc.mode, tc.args)
			mustNoError(t, err)
			m, err := buildManifests(o, false, nil)
			mustNoError(t, err)
			// Embedded JSON whitespace is immaterial; normalize before comparing specs.
			for _, ds := range []*appsv1.DaemonSet{&expected, m.agent} {
				for i := range ds.Spec.Template.Spec.Containers[0].Env {
					env := &ds.Spec.Template.Spec.Containers[0].Env[i]
					if env.Name == "FLP_CONFIG" || env.Name == "FLOW_FILTER_RULES" {
						var v any
						mustNoError(t, json.Unmarshal([]byte(env.Value), &v))
						b, err := json.Marshal(v)
						mustNoError(t, err)
						env.Value = string(b)
					}
				}
			}
			assert.Equal(t, expected.Spec, m.agent.Spec)
		})
	}
}

func TestCommandCompatibility(t *testing.T) {
	for _, mode := range []string{"flows", "packets", "metrics", "follow", "stop", "copy", "cleanup"} {
		for _, help := range []string{"help", "--help"} {
			t.Run(mode+help, func(t *testing.T) {
				var out bytes.Buffer
				c := NewCommand("test")
				c.SetOut(&out)
				c.SetArgs([]string{mode, "--port=8080", help})
				mustNoError(t, c.Execute())
				assert.Contains(t, out.String(), "Syntax: netobserv "+mode)
			})
		}
	}
	for _, arg := range []string{"version", "--version"} {
		var out bytes.Buffer
		c := NewCommand("test")
		c.SetOut(&out)
		c.SetArgs([]string{arg})
		mustNoError(t, c.Execute())
		assert.Contains(t, out.String(), "NetObserv CLI version test")
	}
}
func TestCollectorArguments(t *testing.T) {
	args := []string{"--background", "--max-time=15m", "--protocol=TCP", "--port=8080", "or", "--protocol=UDP", `--query=SrcK8S_Name="a'b; $(touch x)"`}
	o, err := parseOptions("flows", args)
	mustNoError(t, err)
	command := o.collectorCommand()
	assert.Equal(t, "background|max-time=15m|protocol=TCP|port=8080|or|protocol=UDP|query=SrcK8S_Name=\"a'b; $(touch x)\"", command[len(command)-1])
	assert.Contains(t, command, "15m0s")
	m, err := buildManifests(o, false, nil)
	mustNoError(t, err)
	assert.Equal(t, command, m.pod.Spec.Containers[0].Command[4:])
}
func TestAllBashFlagsAccepted(t *testing.T) {
	b, err := os.ReadFile("../../../e2e/testdata/legacy-help-flags.txt")
	mustNoError(t, err)
	o, err := parseOptions("flows", []string{"--sport=1234", "--cidr=::/0", "or", "--cidr=10.0.0.0/8"})
	mustNoError(t, err)
	assert.Len(t, o.filters, 2)
	assert.Equal(t, 1234, o.filters[0]["source_port"])
	// Every flag documented in the Bash help must still appear in embedded help.
	for _, flag := range strings.Fields(string(b)) {
		found := false
		for _, mode := range []string{"flows", "packets", "metrics"} {
			text, err := helpFiles.ReadFile("help/" + mode + ".txt")
			mustNoError(t, err)
			found = found || strings.Contains(string(text), flag)
		}
		assert.True(t, found, flag)
	}
}

func mustNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestEveryBashOptionAccepted(t *testing.T) {
	data, err := os.ReadFile("../../../e2e/testdata/legacy-options.txt")
	mustNoError(t, err)
	cases := strings.Fields(string(data))
	samples := map[string]string{
		"sampling": "50", "interfaces": "eth0", "exclude_interfaces": "lo", "direction": "Ingress", "cidr": "10.0.0.0/8", "protocol": "TCP",
		"sport": "80", "dport": "443", "port": "53", "sport_range": "1000-2000", "dport_range": "1000-2000", "port_range": "1000-2000",
		"sports": "80,443", "dports": "80,443", "ports": "80,443", "tcp_flags": "SYN", "query": `SrcPort=80`, "icmp_type": "8", "icmp_code": "0",
		"peer_ip": "10.1.1.1", "peer_cidr": "10.2.0.0/16", "action": "Accept", "log-level": "debug", "max-time": "15m", "max-bytes": "1000",
		"node-selector": "example.com/node:true", "include_list": "node",
	}
	for _, key := range cases {
		t.Run(key, func(t *testing.T) {
			mode := "flows"
			if key == "include_list" {
				mode = "metrics"
			}
			args := []string{"--" + key}
			if key == "or" {
				args = []string{"--port=80", "or", "--port=443"}
			} else if !booleanOption(key) && key != "copy" {
				value, ok := samples[key]
				if !ok {
					t.Fatalf("missing compatibility sample for %s", key)
				}
				args[0] += "=" + value
			}
			_, err := parseOptions(mode, args)
			mustNoError(t, err)
		})
	}
	assert.Greater(t, len(cases), 40)
}

func TestOfflineYAML(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing"))
	for _, mode := range []string{"flows", "packets", "metrics"} {
		dir := t.TempDir()
		var out bytes.Buffer
		c := NewCommand("test")
		c.SetOut(&out)
		c.SetArgs([]string{mode, "--port=443", "--yaml", "--output-dir=" + dir})
		mustNoError(t, c.Execute())
		files, err := filepath.Glob(filepath.Join(dir, "*.yml"))
		mustNoError(t, err)
		assert.Len(t, files, 1)
		data, err := os.ReadFile(files[0])
		mustNoError(t, err)
		assert.Contains(t, string(data), "kind: DaemonSet")
		assert.Contains(t, string(data), "kind: SecurityContextConstraints")
	}
}
