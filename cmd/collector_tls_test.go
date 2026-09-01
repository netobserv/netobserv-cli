package cmd

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// writeSelfSignedCert generates a throwaway self-signed cert/key pair, writes it into dir and returns
// the cert and key paths.
func writeSelfSignedCert(t *testing.T, dir string) (string, string) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "collector-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}

	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")
	writeFile(t, certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	writeFile(t, keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	return certPath, keyPath
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func TestBuildCollectorTLSOptions(t *testing.T) {
	t.Run("returns nil when the cert file is missing", func(t *testing.T) {
		dir := t.TempDir()
		assert.Nil(t, buildCollectorTLSOptions(filepath.Join(dir, "missing.crt"), filepath.Join(dir, "missing.key")))
	})

	t.Run("returns nil when the key file is missing", func(t *testing.T) {
		dir := t.TempDir()
		certPath, _ := writeSelfSignedCert(t, dir)
		assert.Nil(t, buildCollectorTLSOptions(certPath, filepath.Join(dir, "missing.key")))
	})

	t.Run("returns nil when the key pair is invalid", func(t *testing.T) {
		dir := t.TempDir()
		certPath := filepath.Join(dir, "tls.crt")
		keyPath := filepath.Join(dir, "tls.key")
		writeFile(t, certPath, []byte("not a cert"))
		writeFile(t, keyPath, []byte("not a key"))
		assert.Nil(t, buildCollectorTLSOptions(certPath, keyPath))
	})

	t.Run("returns one server option for a valid key pair", func(t *testing.T) {
		dir := t.TempDir()
		certPath, keyPath := writeSelfSignedCert(t, dir)
		opts := buildCollectorTLSOptions(certPath, keyPath)
		assert.Len(t, opts, 1)
	})
}

func TestBuildMockClientTLSConfig(t *testing.T) {
	t.Run("returns nil when the cert file is missing", func(t *testing.T) {
		dir := t.TempDir()
		assert.Nil(t, buildMockClientTLSConfig(filepath.Join(dir, "missing.crt")))
	})

	t.Run("returns an insecure TLS 1.3 config when the cert file exists", func(t *testing.T) {
		dir := t.TempDir()
		certPath, _ := writeSelfSignedCert(t, dir)
		cfg := buildMockClientTLSConfig(certPath)
		if cfg == nil {
			t.Fatal("expected a non-nil TLS config")
		}
		assert.True(t, cfg.InsecureSkipVerify)
		assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	})
}
