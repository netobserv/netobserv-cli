// Package tlsresolver resolves the OpenShift APIServer TLS security profile into the numeric
// TLS settings (min version, cipher suites, curves) consumed by the collector gRPC server and the
// FLP agent through the TLS_MIN_VERSION / TLS_CIPHER_SUITES / TLS_CURVE_PREFERENCES env vars.
//
// The conversion logic mirrors the netobserv-operator's internal/pkg/tlsconfig package so that the
// CLI honors the cluster tlsSecurityProfile exactly like the operator/FLP/ebpf-agent do.
package tlsresolver

import (
	"crypto/tls"
	"fmt"
	"strconv"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/crypto"

	"github.com/netobserv/flowlogs-pipeline/pkg/tlsprofile"
)

// ComposeTLSConfig creates a tls.Config from an OpenShift TLS security profile.
// It always returns a valid config using secure defaults as a base, so callers can safely use it
// even when an error is returned. The error should be surfaced (e.g. logged) but must not prevent
// TLS from being applied.
func ComposeTLSConfig(profile *configv1.TLSSecurityProfile) (*tls.Config, error) {
	base := crypto.SecureTLSConfig(&tls.Config{})
	if profile == nil {
		return base, nil
	}

	minVersion, cipherSuites, curves, err := profileToTLSConfig(profile)
	if minVersion != 0 {
		base.MinVersion = minVersion
	}
	if len(cipherSuites) > 0 {
		base.CipherSuites = cipherSuites
	}
	if len(curves) > 0 {
		base.CurvePreferences = curves
	}
	return base, err
}

// profileToTLSConfig converts an OpenShift TLSSecurityProfile to TLS version, cipher suites and
// curve preferences. Returns partial results alongside any error: callers should apply
// non-zero/non-empty values even when an error is returned.
func profileToTLSConfig(profile *configv1.TLSSecurityProfile) (uint16, []uint16, []tls.CurveID, error) {
	if profile == nil {
		return 0, nil, nil, nil
	}

	var minVersionStr configv1.TLSProtocolVersion
	var cipherNames []string
	var groups []configv1.TLSGroup

	switch profile.Type {
	case configv1.TLSProfileOldType, configv1.TLSProfileIntermediateType, configv1.TLSProfileModernType:
		spec := configv1.TLSProfiles[profile.Type]
		minVersionStr = spec.MinTLSVersion
		cipherNames = spec.Ciphers
		groups = spec.Groups
	case configv1.TLSProfileCustomType:
		if profile.Custom == nil {
			return 0, nil, nil, fmt.Errorf("custom TLS profile specified but Custom field is nil")
		}
		minVersionStr = profile.Custom.MinTLSVersion
		cipherNames = profile.Custom.Ciphers
		groups = profile.Custom.Groups
	default:
		return 0, nil, nil, fmt.Errorf("unknown TLS profile type %q", profile.Type)
	}

	minVersion, versionErr := tlsVersionFromString(minVersionStr)

	ianaCipherNames := crypto.OpenSSLToIANACipherSuites(cipherNames)
	cipherSuites := make([]uint16, 0, len(ianaCipherNames))
	for _, name := range ianaCipherNames {
		if id := cipherSuiteByName(name); id != 0 {
			cipherSuites = append(cipherSuites, id)
		}
	}

	curves := make([]tls.CurveID, 0, len(groups))
	for _, group := range groups {
		if id, ok := curveIDByGroup(group); ok {
			curves = append(curves, id)
		}
	}

	return minVersion, cipherSuites, curves, versionErr
}

// curveIDByGroup maps an OpenShift TLSGroup to its Go crypto/tls.CurveID equivalent.
// Returns false for groups Go's crypto/tls doesn't support, so callers can silently skip them
// rather than fail the whole config.
func curveIDByGroup(group configv1.TLSGroup) (tls.CurveID, bool) {
	switch group {
	case configv1.TLSGroupX25519:
		return tls.X25519, true
	case configv1.TLSGroupSecP256r1:
		return tls.CurveP256, true
	case configv1.TLSGroupSecP384r1:
		return tls.CurveP384, true
	case configv1.TLSGroupSecP521r1:
		return tls.CurveP521, true
	case configv1.TLSGroupX25519MLKEM768:
		return tls.X25519MLKEM768, true
	case configv1.TLSGroupSecP256r1MLKEM768:
		return tls.SecP256r1MLKEM768, true
	case configv1.TLSGroupSecP384r1MLKEM1024:
		return tls.SecP384r1MLKEM1024, true
	default:
		return 0, false
	}
}

// tlsVersionFromString converts a TLS version string to its uint16 constant.
// Returns an error for unknown versions instead of silently defaulting.
func tlsVersionFromString(version configv1.TLSProtocolVersion) (uint16, error) {
	switch version {
	case configv1.VersionTLS10:
		return tls.VersionTLS10, nil
	case configv1.VersionTLS11:
		return tls.VersionTLS11, nil
	case configv1.VersionTLS12:
		return tls.VersionTLS12, nil
	case configv1.VersionTLS13:
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("unknown TLS version %q", version)
	}
}

// cipherSuiteByName returns the cipher suite ID for a given IANA name, or 0 if not found.
func cipherSuiteByName(name string) uint16 {
	for _, suite := range tls.CipherSuites() {
		if suite.Name == name {
			return suite.ID
		}
	}
	// Insecure cipher suites (for compatibility with the Old profile)
	for _, suite := range tls.InsecureCipherSuites() {
		if suite.Name == name {
			return suite.ID
		}
	}
	return 0
}

// ConfigToEnvMap encodes the version/ciphers/curves of a tls.Config as the TLS_* ConfigMap entries
// consumed by the collector and the FLP agent. Values are decimal uint16 strings so that new TLS
// versions, cipher suites and curves are supported without code changes on either side. Only
// non-empty fields are emitted.
func ConfigToEnvMap(tlsCfg *tls.Config) map[string]string {
	data := map[string]string{}
	if tlsCfg == nil {
		return data
	}

	if tlsCfg.MinVersion != 0 {
		data[tlsprofile.EnvMinVersion] = strconv.FormatUint(uint64(tlsCfg.MinVersion), 10)
	}
	if len(tlsCfg.CipherSuites) > 0 {
		data[tlsprofile.EnvCipherSuites] = uint16SliceToString(tlsCfg.CipherSuites)
	}
	if len(tlsCfg.CurvePreferences) > 0 {
		ids := make([]uint16, len(tlsCfg.CurvePreferences))
		for i, c := range tlsCfg.CurvePreferences {
			ids[i] = uint16(c)
		}
		data[tlsprofile.EnvCurvePreferences] = uint16SliceToString(ids)
	}
	return data
}

func uint16SliceToString(ids []uint16) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatUint(uint64(id), 10)
	}
	return strings.Join(parts, ",")
}
