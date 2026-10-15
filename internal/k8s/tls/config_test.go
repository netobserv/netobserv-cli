package tlsresolver

import (
	"crypto/tls"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"

	"github.com/netobserv/flowlogs-pipeline/pkg/tlsprofile"
)

func TestComposeTLSConfig_Presets(t *testing.T) {
	t.Run("nil profile yields secure defaults", func(t *testing.T) {
		cfg, err := ComposeTLSConfig(nil)
		assert.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.NotZero(t, cfg.MinVersion)
	})

	t.Run("Intermediate resolves to TLS 1.2 with ciphers and curves", func(t *testing.T) {
		cfg, err := ComposeTLSConfig(&configv1.TLSSecurityProfile{Type: configv1.TLSProfileIntermediateType})
		assert.NoError(t, err)
		assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
		assert.NotEmpty(t, cfg.CipherSuites)
		assert.NotEmpty(t, cfg.CurvePreferences)
	})

	t.Run("Modern resolves to TLS 1.3", func(t *testing.T) {
		cfg, err := ComposeTLSConfig(&configv1.TLSSecurityProfile{Type: configv1.TLSProfileModernType})
		assert.NoError(t, err)
		assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	})

	t.Run("Old resolves to TLS 1.0", func(t *testing.T) {
		cfg, err := ComposeTLSConfig(&configv1.TLSSecurityProfile{Type: configv1.TLSProfileOldType})
		assert.NoError(t, err)
		assert.Equal(t, uint16(tls.VersionTLS10), cfg.MinVersion)
	})

	t.Run("unknown type returns error but a usable base config", func(t *testing.T) {
		cfg, err := ComposeTLSConfig(&configv1.TLSSecurityProfile{Type: "bogus"})
		assert.Error(t, err)
		assert.NotNil(t, cfg)
		assert.NotZero(t, cfg.MinVersion)
	})

	t.Run("custom nil Custom field returns error", func(t *testing.T) {
		_, err := ComposeTLSConfig(&configv1.TLSSecurityProfile{Type: configv1.TLSProfileCustomType})
		assert.Error(t, err)
	})
}

func TestComposeTLSConfig_Custom(t *testing.T) {
	profile := &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileCustomType,
		Custom: &configv1.CustomTLSProfile{
			TLSProfileSpec: configv1.TLSProfileSpec{
				MinTLSVersion: configv1.VersionTLS12,
				Ciphers:       []string{"ECDHE-RSA-AES128-GCM-SHA256"},
				Groups:        []configv1.TLSGroup{configv1.TLSGroupX25519},
			},
		},
	}

	cfg, err := ComposeTLSConfig(profile)
	assert.NoError(t, err)

	data := ConfigToEnvMap(cfg)
	assert.Equal(t, "771", data[tlsprofile.EnvMinVersion])
	assert.Equal(t, "49199", data[tlsprofile.EnvCipherSuites])  // TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
	assert.Equal(t, "29", data[tlsprofile.EnvCurvePreferences]) // tls.X25519
}

func TestConfigToEnvMap(t *testing.T) {
	t.Run("nil config yields empty map", func(t *testing.T) {
		assert.Empty(t, ConfigToEnvMap(nil))
	})

	t.Run("encodes each field as decimal", func(t *testing.T) {
		cfg := &tls.Config{
			MinVersion:       tls.VersionTLS12,
			CipherSuites:     []uint16{49199, 49195},
			CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256},
		}
		data := ConfigToEnvMap(cfg)
		assert.Equal(t, "771", data[tlsprofile.EnvMinVersion])
		assert.Equal(t, "49199,49195", data[tlsprofile.EnvCipherSuites])
		assert.Equal(t, "29,23", data[tlsprofile.EnvCurvePreferences])
	})
}
