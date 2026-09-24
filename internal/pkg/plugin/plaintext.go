package plugin

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const collectorKeylogPath = "/etc/capture-keylog/keys.log"

func configurePlaintextAgent(spec *corev1.PodSpec, o *options) {
	c := &spec.Containers[0]
	for _, v := range o.values {
		switch v.key {
		case "tls_plaintext_min_bytes", "tls_plaintext_preview_bytes":
			setEnv(c, strings.ToUpper(v.key), v.value)
		case "tls_process_allowlist":
			setEnv(c, "TLS_PLAINTEXT_PROCESS_ALLOWLIST", v.value)
		}
	}
	if !o.enabled("enable_openssl") {
		return
	}
	setEnv(c, "ENABLE_OPENSSL_TRACKING", "true")
	spec.HostPID = true
	writable := false
	c.SecurityContext.ReadOnlyRootFilesystem = &writable
	c.SecurityContext.Capabilities.Add = append(c.SecurityContext.Capabilities.Add, corev1.Capability("SYS_PTRACE"))
	directory := corev1.HostPathDirectory
	for _, name := range []string{"usr", "lib", "lib64"} {
		spec.Volumes = append(spec.Volumes, corev1.Volume{Name: "host-" + name, VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/" + name, Type: &directory}}})
		c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{Name: "host-" + name, MountPath: "/host/" + name, ReadOnly: true})
	}
}

// Snapshot the local key log into a namespaced Secret; the collector never
// assumes that a path on the user's machine exists inside its container.
func (m *manifests) addKeylog(o *options) error {
	path := ""
	for _, v := range o.values {
		if v.key == "tls-keylog" {
			path = v.value
		}
	}
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read TLS key log: %w", err)
	}
	secret := &corev1.Secret{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}, ObjectMeta: metav1.ObjectMeta{Name: "capture-keylog", Namespace: o.namespace}, Data: map[string][]byte{"keys.log": data}}
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(secret)
	if err != nil {
		return err
	}
	m.objects = append(m.objects, &unstructured.Unstructured{Object: obj})
	m.pod.Spec.Volumes = append(m.pod.Spec.Volumes, corev1.Volume{Name: "capture-keylog", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "capture-keylog"}}})
	m.pod.Spec.Containers[0].VolumeMounts = append(m.pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{Name: "capture-keylog", MountPath: "/etc/capture-keylog", ReadOnly: true})
	return nil
}

func confirmPlaintext(o *options, in io.Reader, out, errOut io.Writer) error {
	if o.mode != "packets" || !o.enabled("enable_openssl") {
		return nil
	}
	peer, port := "", ""
	for _, v := range o.values {
		if peer == "" && oneOf(v.key, "peer_ip", "peer_cidr", "tls_process_allowlist") {
			peer = v.key + "=" + v.value
		}
		if port == "" && oneOf(v.key, "port", "dport", "sport", "ports", "dports", "sports", "port_range", "dport_range", "sport_range") {
			port = v.key + "=" + v.value
		}
	}
	var scopes []string
	if peer != "" {
		scopes = append(scopes, peer)
	} else {
		fmt.Fprintln(errOut, "Warning: TLS plaintext capture has no peer or process scope. OpenSSL hooks libssl.so in every container on each node (infra binaries excluded). Recommended: --peer_ip=<pod-ip>.")
	}
	if port != "" {
		scopes = append(scopes, port)
	} else {
		fmt.Fprintln(errOut, "Warning: TLS plaintext capture has no port filter. Recommended: --port=<service-port> to filter exported plaintext and wire capture.")
	}
	if len(scopes) > 0 {
		fmt.Fprintf(out, "Plaintext capture scoped to %s.\n", strings.Join(scopes, ", "))
	}
	fmt.Fprintln(errOut, "WARNING: TLS decryption exposes encrypted traffic in plain text. In some jurisdictions, capturing others' communications may be prohibited without consent. Make sure this is legally permitted before proceeding.")
	if os.Getenv("isE2E") == "true" {
		return nil
	}
	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, "Continue? [yes/no] ")
		if !scanner.Scan() {
			return fmt.Errorf("capture aborted")
		}
		answer := strings.ToLower(scanner.Text())
		if strings.HasPrefix(answer, "y") {
			return nil
		}
		if strings.HasPrefix(answer, "n") {
			return fmt.Errorf("capture aborted")
		}
		fmt.Fprintln(out, "Please answer yes or no.")
	}
}

func validatePlaintextValue(opt option) error {
	switch opt.key {
	case "tls_plaintext_min_bytes", "tls_plaintext_preview_bytes":
		if _, err := strconv.ParseUint(opt.value, 10, 64); err != nil {
			return fmt.Errorf("--%s requires a non-negative integer", opt.key)
		}
	case "tls_process_allowlist", "tls-keylog":
		if strings.TrimSpace(opt.value) == "" {
			return fmt.Errorf("--%s must not be empty", opt.key)
		}
	}
	return nil
}
