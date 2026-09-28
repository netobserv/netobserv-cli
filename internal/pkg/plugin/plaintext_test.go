package plugin

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestPlaintextOptionsAndManifest(t *testing.T) {
	o, err := parseOptions("packets", []string{"--port=8443", "--enable_openssl", "--tls_process_allowlist=nginx,envoy", "--tls_plaintext_min_bytes=12", "--tls_plaintext_preview_bytes=0"})
	mustNoError(t, err)
	m, err := buildManifests(o, false, nil)
	mustNoError(t, err)
	spec := m.agent.Spec.Template.Spec
	c := spec.Containers[0]
	env := map[string]string{}
	for _, e := range c.Env {
		env[e.Name] = e.Value
	}
	assert.Equal(t, "true", env["ENABLE_OPENSSL_TRACKING"])
	assert.Equal(t, "12", env["TLS_PLAINTEXT_MIN_BYTES"])
	assert.Equal(t, "0", env["TLS_PLAINTEXT_PREVIEW_BYTES"])
	assert.Equal(t, "nginx,envoy", env["TLS_PLAINTEXT_PROCESS_ALLOWLIST"])
	assert.True(t, spec.HostPID)
	assert.NotNil(t, c.SecurityContext.Privileged)
	assert.True(t, *c.SecurityContext.Privileged)
	assert.True(t, *c.SecurityContext.AllowPrivilegeEscalation)
	assert.False(t, *c.SecurityContext.ReadOnlyRootFilesystem)
	assert.Contains(t, c.SecurityContext.Capabilities.Add, corev1.Capability("SYS_PTRACE"))
	for _, name := range []string{"usr", "lib", "lib64"} {
		assert.Contains(t, c.VolumeMounts, corev1.VolumeMount{Name: "host-" + name, MountPath: "/host/" + name, ReadOnly: true})
	}
	var filters []map[string]any
	mustNoError(t, json.Unmarshal([]byte(env["FLOW_FILTER_RULES"]), &filters))
	assert.Equal(t, float64(8443), filters[0]["port"])
	assert.Contains(t, strings.Join(o.collectorCommand(), " "), "enable_openssl")
	for _, args := range [][]string{{"--enable_openssl=false"}, {"--enable_all"}} {
		o, err = parseOptions("packets", append(args, "--port=443"))
		mustNoError(t, err)
		m, err = buildManifests(o, false, nil)
		mustNoError(t, err)
		assert.False(t, m.agent.Spec.Template.Spec.HostPID)
	}
}

func TestPlaintextValidation(t *testing.T) {
	for _, flag := range []string{"--enable_openssl", "--tls_plaintext_min_bytes=0", "--tls_plaintext_preview_bytes=256", "--tls_process_allowlist=nginx", "--tls-keylog=keys.log"} {
		for _, mode := range []string{"flows", "metrics"} {
			_, err := parseOptions(mode, []string{flag})
			assert.Error(t, err)
		}
	}
	for _, flag := range []string{"--enable_openssl=invalid", "--tls_plaintext_min_bytes=-1", "--tls_plaintext_preview_bytes=abc", "--tls_process_allowlist=", "--tls-keylog="} {
		_, err := parseOptions("packets", []string{"--port=443", flag})
		assert.Error(t, err)
	}
}

func TestPlaintextConfirmation(t *testing.T) {
	t.Setenv("isE2E", "false")
	o, err := parseOptions("packets", []string{"--port=443", "--enable_openssl", "--tls_process_allowlist=nginx"})
	mustNoError(t, err)
	for _, tc := range []struct {
		input  string
		denied bool
	}{{"yes\n", false}, {"no\n", true}, {"", true}, {"invalid\ny\n", false}} {
		var out, warnings bytes.Buffer
		err = confirmPlaintext(o, strings.NewReader(tc.input), &out, &warnings)
		assert.Equal(t, tc.denied, err != nil)
		assert.Contains(t, warnings.String(), "TLS decryption exposes")
		assert.NotContains(t, warnings.String(), "no peer or process scope")
		assert.Contains(t, out.String(), "tls_process_allowlist=nginx, port=443")
	}
	t.Setenv("isE2E", "true")
	mustNoError(t, confirmPlaintext(o, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}))
}

func TestLocalKeylogSecret(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keys.log")
	mustNoError(t, os.WriteFile(file, []byte("test-keylog\n"), 0600))
	o, err := parseOptions("packets", []string{"--port=443", "--tls-keylog=" + file})
	mustNoError(t, err)
	for _, tls := range []bool{false, true} {
		m, err := buildManifests(o, tls, nil)
		mustNoError(t, err)
		var secret corev1.Secret
		mustNoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(m.objects[len(m.objects)-1].Object, &secret))
		assert.Equal(t, []byte("test-keylog\n"), secret.Data["keys.log"])
		assert.Equal(t, o.namespace, secret.Namespace)
		assert.Contains(t, m.pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{Name: "capture-keylog", MountPath: "/etc/capture-keylog", ReadOnly: true})
		assert.Contains(t, o.collectorCommand(), collectorKeylogPath)
	}
	mustNoError(t, os.Remove(file))
	_, err = buildManifests(o, false, nil)
	assert.ErrorContains(t, err, "read TLS key log")
}
