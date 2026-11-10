package agent

import (
	"strconv"
	"strings"
)

// SetSampling replaces edit_manifest "sampling".
func (b *Builder) SetSampling(n int) {
	b.setEnv(envSampling, strconv.Itoa(n))
}

// SetInterfaces replaces edit_manifest "interfaces". Unlike the bash flag (a single
// already-comma-joined string the user had to format themselves), the CLI flag backing this is
// a repeatable --interfaces=eth0 --interfaces=eth1 string slice; this is where it's joined back
// into the comma-separated form the in-pod agent's INTERFACES env var expects.
func (b *Builder) SetInterfaces(interfaces []string) {
	b.setEnv(envInterfaces, strings.Join(interfaces, ","))
}

// SetExcludeInterfaces replaces edit_manifest "exclude_interfaces".
func (b *Builder) SetExcludeInterfaces(interfaces []string) {
	b.setEnv(envExcludeInterfaces, strings.Join(interfaces, ","))
}

// SetLogLevel replaces edit_manifest "log_level".
func (b *Builder) SetLogLevel(level string) {
	b.setEnv(envLogLevel, level)
}

// SetNodeSelector replaces edit_manifest "node_selector".
func (b *Builder) SetNodeSelector(key, value string) {
	spec := &b.ds.Spec.Template.Spec
	if spec.NodeSelector == nil {
		spec.NodeSelector = map[string]string{}
	}
	spec.NodeSelector[key] = value
}

// SetTLSPlaintextMinBytes replaces edit_manifest "tls_plaintext_min_bytes".
func (b *Builder) SetTLSPlaintextMinBytes(n int) {
	b.setEnv(envTLSPlaintextMinBytes, strconv.Itoa(n))
}

// SetTLSPlaintextPreviewBytes replaces edit_manifest "tls_plaintext_preview_bytes".
func (b *Builder) SetTLSPlaintextPreviewBytes(n int) {
	b.setEnv(envTLSPlaintextPreviewBytes, strconv.Itoa(n))
}

// SetTLSProcessAllowlist replaces edit_manifest "tls_process_allowlist".
func (b *Builder) SetTLSProcessAllowlist(processes []string) {
	b.setEnv(envTLSPlaintextAllowlist, strings.Join(processes, ","))
}
