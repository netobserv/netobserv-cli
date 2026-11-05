// Package tlsresolver resolves the OpenShift APIServer TLS security profile into the numeric
// TLS settings (min version, cipher suites, curves) consumed by the collector gRPC server and the
// FLP agent through the TLS_MIN_VERSION / TLS_CIPHER_SUITES / TLS_CURVE_PREFERENCES env vars.
//
// The conversion logic mirrors the netobserv-operator's internal/pkg/tlsconfig package so that the
// CLI honors the cluster tlsSecurityProfile exactly like the operator/FLP/ebpf-agent do.
package tlsresolver

import (
	"crypto/tls"

	sharedtls "github.com/netobserv/netobserv-operator/pkg/tlsconfig"
	configv1 "github.com/openshift/api/config/v1"
)

// ComposeTLSConfig creates a tls.Config from an OpenShift TLS security profile.
// It always returns a valid config using secure defaults as a base, so callers can safely use it
// even when an error is returned. The error should be surfaced (e.g. logged) but must not prevent
// TLS from being applied.
func ComposeTLSConfig(profile *configv1.TLSSecurityProfile) (*tls.Config, error) {
	return sharedtls.ComposeTLSConfig(profile)
}

// ConfigToEnvMap encodes the version/ciphers/curves of a tls.Config as the TLS_* ConfigMap entries
// consumed by the collector and the FLP agent. Values are decimal uint16 strings so that new TLS
// versions, cipher suites and curves are supported without code changes on either side. Only
// non-empty fields are emitted.
func ConfigToEnvMap(tlsCfg *tls.Config) map[string]string {
	data := map[string]string{}
	for _, env := range sharedtls.ConfigToEnvVars(tlsCfg) {
		data[env.Name] = env.Value
	}
	return data
}
