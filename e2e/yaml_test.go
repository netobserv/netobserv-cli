//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

var (
	ylog = logrus.WithField("component", "yaml_test")
)

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
			// ensure script setup is fine
			assert.Contains(t, output, "creating netobserv-cli namespace")
			assert.Contains(t, output, "creating service account")
			assert.Contains(t, output, "creating collector service")
			assert.Contains(t, output, "creating flow-capture agents")
			// check CLI done successfully
			assert.Contains(t, output, "Check the generated YAML file in output folder")

			return ctx
		},
	).Assess("check generated yaml output",
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			var yamls []string

			dirPath := "output"
			assert.True(t, dirExists(dirPath), "directory %s not found", dirPath)
			err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					fmt.Println(err)
				}

				if !info.IsDir() {
					if strings.Contains(path, "flows_capture") && filepath.Ext(path) == ".yml" {
						yamls = append(yamls, path)
					}
				}

				return nil
			})
			assert.Nil(t, err)

			// check yaml file
			assert.Equal(t, 1, len(yamls))
			yamlBytes, err := os.ReadFile(yamls[0])
			assert.Nil(t, err)

			// check yamls parts
			yamlStr := string(yamlBytes[:])
			yamls = strings.Split(yamlStr, "---")
			assert.Equal(t, 10, len(yamls))

			// check yaml contents
			assert.Contains(t, yamls[0], "kind: Namespace")
			assert.Contains(t, yamls[0], "name: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[0]), Normalize("labels:app:netobserv-clipod-security.kubernetes.io/enforce:privilegedpod-security.kubernetes.io/audit:privilegedopenshift.io/cluster-monitoring:\"true\""))

			assert.Contains(t, yamls[1], "kind: ServiceAccount")
			assert.Contains(t, yamls[1], "name: netobserv-cli")
			assert.Contains(t, yamls[1], "namespace: \"netobserv-cli\"")

			assert.Contains(t, yamls[2], "kind: ClusterRole")
			assert.Contains(t, yamls[2], "name: netobserv-cli")
			assert.Contains(t, yamls[2], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[2]), Normalize("- apiGroups: - security.openshift.io resourceNames: - privileged resources: - securitycontextconstraints verbs: - use"))
			assert.Contains(t, Normalize(yamls[2]), Normalize("- apiGroups: - apps resources: - daemonsets verbs: - list - get - watch - delete"))
			assert.Contains(t, Normalize(yamls[2]), Normalize("- apiGroups: - resources: - pods - services - nodes verbs: - list - get - watch"))
			assert.Contains(t, Normalize(yamls[2]), Normalize("- apiGroups: - apps resources: - replicasets verbs: - list - get - watch"))
			assert.Contains(t, Normalize(yamls[2]), Normalize("- apiGroups: - config.openshift.io resources: - apiservers resourceNames: - cluster verbs: - get"))

			assert.Contains(t, yamls[3], "kind: ClusterRoleBinding")
			assert.Contains(t, yamls[3], "name: netobserv-cli")
			assert.Contains(t, yamls[3], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[3]), Normalize("subjects: - kind: ServiceAccount name: netobserv-cli namespace: \"netobserv-cli\""))
			assert.Contains(t, Normalize(yamls[3]), Normalize("roleRef: apiGroup: rbac.authorization.k8s.io kind: ClusterRole name: netobserv-cli"))

			assert.Contains(t, yamls[4], "kind: ClusterRoleBinding")
			assert.Contains(t, yamls[4], "name: netobserv-cli-monitoring")
			assert.Contains(t, yamls[4], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[4]), Normalize("subjects: - kind: ServiceAccount name: netobserv-cli namespace: \"netobserv-cli\""))
			assert.Contains(t, Normalize(yamls[4]), Normalize("roleRef: apiGroup: rbac.authorization.k8s.io kind: ClusterRole name: cluster-monitoring-view"))

			assert.Contains(t, yamls[5], "kind: SecurityContextConstraints")
			assert.Contains(t, yamls[5], "name: netobserv-cli")
			assert.Contains(t, yamls[5], "namespace: \"netobserv-cli\"")

			assert.Contains(t, yamls[6], "kind: Role")
			assert.Contains(t, yamls[6], "name: netobserv-cli")
			assert.Contains(t, yamls[6], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[6]), Normalize("- apiGroups: - resources: - configmaps verbs: - get - create - update"))

			assert.Contains(t, yamls[7], "kind: RoleBinding")
			assert.Contains(t, yamls[7], "name: netobserv-cli")
			assert.Contains(t, yamls[7], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[7]), Normalize("subjects: - kind: ServiceAccount name: netobserv-cli namespace: \"netobserv-cli\""))
			assert.Contains(t, Normalize(yamls[7]), Normalize("roleRef: apiGroup: rbac.authorization.k8s.io kind: Role name: netobserv-cli"))

			assert.Contains(t, yamls[8], "kind: Service")
			assert.Contains(t, yamls[8], "name: collector")
			assert.Contains(t, yamls[8], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[8]), Normalize("ports: - name: collector protocol: TCP port: 9999 targetPort: 9999"))

			assert.Contains(t, yamls[9], "kind: DaemonSet")
			assert.Contains(t, yamls[9], "name: netobserv-cli")
			assert.Contains(t, yamls[9], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[9]), Normalize("[{\"direction\": \"\", \"ip_cidr\": \"0.0.0.0/0\", \"protocol\": \"TCP\", \"source_port\": 0, \"destination_port\": 0, \"port\": 8080, \"source_port_range\": \"\", \"source_ports\": \"\", \"destination_port_range\": \"\", \"destination_ports\": \"\", \"port_range\": \"\", \"ports\": \"\", \"icmp_type\": 0, \"icmp_code\": 0, \"peer_ip\": \"\", \"peer_cidr\": \"\", \"action\": \"Accept\", \"tcp_flags\": \"\", \"drops\": false}, {\"direction\": \"\", \"ip_cidr\": \"0.0.0.0/0\", \"protocol\": \"UDP\", \"source_port\": 0, \"destination_port\": 0, \"port\": 0, \"source_port_range\": \"\", \"source_ports\": \"\", \"destination_port_range\": \"\", \"destination_ports\": \"\", \"port_range\": \"\", \"ports\": \"\", \"icmp_type\": 0, \"icmp_code\": 0, \"peer_ip\": \"\", \"peer_cidr\": \"\", \"action\": \"Accept\", \"tcp_flags\": \"\", \"drops\": false}]"))
			assert.Contains(t, Normalize(yamls[9]), Normalize("\"grpc\": { \"targetHost\": \"collector.netobserv-cli.svc.cluster.local\", \"targetPort\": 9999 }"))

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
			// ensure script setup is fine
			assert.Contains(t, output, "creating netobserv-cli namespace")
			assert.Contains(t, output, "creating service account")
			assert.Contains(t, output, "creating collector service")
			assert.Contains(t, output, "creating packet-capture agents")
			// check CLI done successfully
			assert.Contains(t, output, "Check the generated YAML file in output folder")

			return ctx
		},
	).Assess("check generated yaml output",
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			var yamls []string

			dirPath := "output"
			assert.True(t, dirExists(dirPath), "directory %s not found", dirPath)
			err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					fmt.Println(err)
				}

				if !info.IsDir() {
					if strings.Contains(path, "packets_capture") && filepath.Ext(path) == ".yml" {
						yamls = append(yamls, path)
					}
				}

				return nil
			})
			assert.Nil(t, err)

			// check yaml file
			assert.Equal(t, 1, len(yamls))
			yamlBytes, err := os.ReadFile(yamls[0])
			assert.Nil(t, err)

			// check yamls parts
			yamlStr := string(yamlBytes[:])
			yamls = strings.Split(yamlStr, "---")
			assert.Equal(t, 10, len(yamls))

			// check yaml contents
			assert.Contains(t, yamls[0], "kind: Namespace")
			assert.Contains(t, yamls[0], "name: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[0]), Normalize("labels:app:netobserv-clipod-security.kubernetes.io/enforce:privilegedpod-security.kubernetes.io/audit:privilegedopenshift.io/cluster-monitoring:\"true\""))

			assert.Contains(t, yamls[1], "kind: ServiceAccount")
			assert.Contains(t, yamls[1], "name: netobserv-cli")
			assert.Contains(t, yamls[1], "namespace: \"netobserv-cli\"")

			assert.Contains(t, yamls[2], "kind: ClusterRole")
			assert.Contains(t, yamls[2], "name: netobserv-cli")
			assert.Contains(t, yamls[2], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[2]), Normalize("- apiGroups: - security.openshift.io resourceNames: - privileged resources: - securitycontextconstraints verbs: - use"))
			assert.Contains(t, Normalize(yamls[2]), Normalize("- apiGroups: - apps resources: - daemonsets verbs: - list - get - watch - delete"))
			assert.Contains(t, Normalize(yamls[2]), Normalize("- apiGroups: - resources: - pods - services - nodes verbs: - list - get - watch"))
			assert.Contains(t, Normalize(yamls[2]), Normalize("- apiGroups: - apps resources: - replicasets verbs: - list - get - watch"))
			assert.Contains(t, Normalize(yamls[2]), Normalize("- apiGroups: - config.openshift.io resources: - apiservers resourceNames: - cluster verbs: - get"))

			assert.Contains(t, yamls[3], "kind: ClusterRoleBinding")
			assert.Contains(t, yamls[3], "name: netobserv-cli")
			assert.Contains(t, yamls[3], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[3]), Normalize("subjects: - kind: ServiceAccount name: netobserv-cli namespace: \"netobserv-cli\""))
			assert.Contains(t, Normalize(yamls[3]), Normalize("roleRef: apiGroup: rbac.authorization.k8s.io kind: ClusterRole name: netobserv-cli"))

			assert.Contains(t, yamls[4], "kind: ClusterRoleBinding")
			assert.Contains(t, yamls[4], "name: netobserv-cli-monitoring")
			assert.Contains(t, yamls[4], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[4]), Normalize("subjects: - kind: ServiceAccount name: netobserv-cli namespace: \"netobserv-cli\""))
			assert.Contains(t, Normalize(yamls[4]), Normalize("roleRef: apiGroup: rbac.authorization.k8s.io kind: ClusterRole name: cluster-monitoring-view"))

			assert.Contains(t, yamls[5], "kind: SecurityContextConstraints")
			assert.Contains(t, yamls[5], "name: netobserv-cli")
			assert.Contains(t, yamls[5], "namespace: \"netobserv-cli\"")

			assert.Contains(t, yamls[6], "kind: Role")
			assert.Contains(t, yamls[6], "name: netobserv-cli")
			assert.Contains(t, yamls[6], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[6]), Normalize("- apiGroups: - resources: - configmaps verbs: - get - create - update"))

			assert.Contains(t, yamls[7], "kind: RoleBinding")
			assert.Contains(t, yamls[7], "name: netobserv-cli")
			assert.Contains(t, yamls[7], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[7]), Normalize("subjects: - kind: ServiceAccount name: netobserv-cli namespace: \"netobserv-cli\""))
			assert.Contains(t, Normalize(yamls[7]), Normalize("roleRef: apiGroup: rbac.authorization.k8s.io kind: Role name: netobserv-cli"))

			assert.Contains(t, yamls[8], "kind: Service")
			assert.Contains(t, yamls[8], "name: collector")
			assert.Contains(t, yamls[8], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[8]), Normalize("ports: - name: collector protocol: TCP port: 9999 targetPort: 9999"))

			assert.Contains(t, yamls[9], "kind: DaemonSet")
			assert.Contains(t, yamls[9], "name: netobserv-cli")
			assert.Contains(t, yamls[9], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[9]), Normalize("[{\"direction\": \"\", \"ip_cidr\": \"0.0.0.0/0\", \"protocol\": \"\", \"source_port\": 0, \"destination_port\": 0, \"port\": 80, \"source_port_range\": \"\", \"source_ports\": \"\", \"destination_port_range\": \"\", \"destination_ports\": \"\", \"port_range\": \"\", \"ports\": \"\", \"icmp_type\": 0, \"icmp_code\": 0, \"peer_ip\": \"\", \"peer_cidr\": \"\", \"action\": \"Accept\", \"tcp_flags\": \"\", \"drops\": false}]"))
			assert.Contains(t, Normalize(yamls[9]), Normalize("nodeSelector: netobserv: \"true\""))

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
			// ensure script setup is fine
			assert.Contains(t, output, "creating netobserv-cli namespace")
			assert.Contains(t, output, "creating service account")
			assert.Contains(t, output, "creating service monitor")
			assert.Contains(t, output, "creating metric-capture agents")
			// check CLI done successfully
			assert.Contains(t, output, "Check the generated YAML file in output folder.")

			return ctx
		},
	).Assess("check generated yaml output",
		func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			var yamls []string

			dirPath := "output"
			assert.True(t, dirExists(dirPath), "directory %s not found", dirPath)
			err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					fmt.Println(err)
				}

				if !info.IsDir() {
					if strings.Contains(path, "metrics_capture") && filepath.Ext(path) == ".yml" {
						yamls = append(yamls, path)
					}
				}

				return nil
			})
			assert.Nil(t, err)

			// check yaml file
			assert.Equal(t, 1, len(yamls))
			yamlBytes, err := os.ReadFile(yamls[0])
			assert.Nil(t, err)

			// check yamls parts
			yamlStr := string(yamlBytes[:])
			yamls = strings.Split(yamlStr, "---")
			assert.Equal(t, 14, len(yamls))

			// check yaml contents
			assert.Contains(t, yamls[0], "kind: Namespace")
			assert.Contains(t, yamls[0], "name: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[0]), Normalize("labels:app:netobserv-clipod-security.kubernetes.io/enforce:privilegedpod-security.kubernetes.io/audit:privilegedopenshift.io/cluster-monitoring:\"true\""))

			assert.Contains(t, yamls[1], "kind: ServiceAccount")
			assert.Contains(t, yamls[1], "name: netobserv-cli")
			assert.Contains(t, yamls[1], "namespace: \"netobserv-cli\"")

			assert.Contains(t, yamls[2], "kind: ClusterRole")
			assert.Contains(t, yamls[2], "name: netobserv-cli")
			assert.Contains(t, yamls[2], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[2]), Normalize("- apiGroups: - security.openshift.io resourceNames: - privileged resources: - securitycontextconstraints verbs: - use"))
			assert.Contains(t, Normalize(yamls[2]), Normalize("- apiGroups: - apps resources: - daemonsets verbs: - list - get - watch - delete"))
			assert.Contains(t, Normalize(yamls[2]), Normalize("- apiGroups: - resources: - pods - services - nodes verbs: - list - get - watch"))
			assert.Contains(t, Normalize(yamls[2]), Normalize("- apiGroups: - apps resources: - replicasets verbs: - list - get - watch"))
			assert.Contains(t, Normalize(yamls[2]), Normalize("- apiGroups: - config.openshift.io resources: - apiservers resourceNames: - cluster verbs: - get"))

			assert.Contains(t, yamls[3], "kind: ClusterRoleBinding")
			assert.Contains(t, yamls[3], "name: netobserv-cli")
			assert.Contains(t, yamls[3], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[3]), Normalize("subjects: - kind: ServiceAccount name: netobserv-cli namespace: \"netobserv-cli\""))
			assert.Contains(t, Normalize(yamls[3]), Normalize("roleRef: apiGroup: rbac.authorization.k8s.io kind: ClusterRole name: netobserv-cli"))

			assert.Contains(t, yamls[4], "kind: ClusterRoleBinding")
			assert.Contains(t, yamls[4], "name: netobserv-cli-monitoring")
			assert.Contains(t, yamls[4], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[4]), Normalize("subjects: - kind: ServiceAccount name: netobserv-cli namespace: \"netobserv-cli\""))
			assert.Contains(t, Normalize(yamls[4]), Normalize("roleRef: apiGroup: rbac.authorization.k8s.io kind: ClusterRole name: cluster-monitoring-view"))

			assert.Contains(t, yamls[5], "kind: SecurityContextConstraints")
			assert.Contains(t, yamls[5], "name: netobserv-cli")
			assert.Contains(t, yamls[5], "namespace: \"netobserv-cli\"")

			assert.Contains(t, yamls[6], "kind: Role")
			assert.Contains(t, yamls[6], "name: netobserv-cli")
			assert.Contains(t, yamls[6], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[6]), Normalize("- apiGroups: - resources: - configmaps verbs: - get - create - update"))

			assert.Contains(t, yamls[7], "kind: RoleBinding")
			assert.Contains(t, yamls[7], "name: netobserv-cli")
			assert.Contains(t, yamls[7], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[7]), Normalize("subjects: - kind: ServiceAccount name: netobserv-cli namespace: \"netobserv-cli\""))
			assert.Contains(t, Normalize(yamls[7]), Normalize("roleRef: apiGroup: rbac.authorization.k8s.io kind: Role name: netobserv-cli"))

			assert.Contains(t, yamls[8], "kind: ClusterRole")
			assert.Contains(t, yamls[8], "name: netobserv-cli-metrics")
			assert.Contains(t, yamls[8], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[8]), Normalize("- apiGroups: - resources: - pods - services - endpoints verbs: - list - get - watch - nonResourceURLs: - /metrics verbs: - get"))

			assert.Contains(t, yamls[9], "kind: ClusterRoleBinding")
			assert.Contains(t, yamls[9], "name: netobserv-cli")
			assert.Contains(t, yamls[9], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[9]), Normalize("subjects: - kind: ServiceAccount name: prometheus-k8s namespace: openshift-monitoring"))
			assert.Contains(t, Normalize(yamls[9]), Normalize("roleRef: apiGroup: rbac.authorization.k8s.io kind: ClusterRole name: netobserv-cli-metrics"))

			assert.Contains(t, yamls[10], "kind: ServiceMonitor")
			assert.Contains(t, yamls[10], "name: netobserv-cli")
			assert.Contains(t, yamls[10], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[10]), Normalize("namespaceSelector: matchNames: - \"netobserv-cli\""))
			assert.Contains(t, Normalize(yamls[10]), Normalize("selector: matchLabels: app: netobserv-cli"))

			assert.Contains(t, yamls[11], "kind: Service")
			assert.Contains(t, yamls[11], "name: netobserv-cli")
			assert.Contains(t, yamls[11], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[11]), Normalize("ports: - name: prometheus protocol: TCP port: 9401 targetPort: 9401"))

			assert.Contains(t, yamls[12], "kind: ConfigMap")
			assert.Contains(t, yamls[12], "name: netobserv-cli")
			assert.Contains(t, yamls[12], "namespace: openshift-config-managed")
			assert.Contains(t, yamls[12], "console.openshift.io/dashboard: \"true\"")

			assert.Contains(t, yamls[13], "kind: DaemonSet")
			assert.Contains(t, yamls[13], "name: netobserv-cli")
			assert.Contains(t, yamls[13], "namespace: \"netobserv-cli\"")
			assert.Contains(t, Normalize(yamls[13]), Normalize("ports: - name: prometheus containerPort: 9401 protocol: TCP"))

			return ctx
		},
	).Feature()
	testCluster.TestEnv().Test(t, f1)
}

func Normalize(str string) string {
	str = strings.ReplaceAll(str, "\n", "")
	str = strings.ReplaceAll(str, " ", "")
	return str
}
