package capture

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractTarWritesFilesAndDirs(t *testing.T) {
	dest := t.TempDir()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "sub", Typeflag: tar.TypeDir, Mode: 0o755}))
	content := []byte("hello flows\n")
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "sub/flows.txt", Typeflag: tar.TypeReg, Size: int64(len(content)), Mode: 0o644}))
	_, err := tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())

	require.NoError(t, extractTar(&buf, dest))

	got, err := os.ReadFile(filepath.Join(dest, "sub", "flows.txt"))
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestExtractTarRejectsPathEscape(t *testing.T) {
	dest := t.TempDir()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "../escape.txt", Typeflag: tar.TypeReg, Size: 0, Mode: 0o644}))
	require.NoError(t, tw.Close())

	err := extractTar(&buf, dest)
	assert.Error(t, err)
}

func TestIsWithinDir(t *testing.T) {
	assert.True(t, isWithinDir("/tmp/out", "/tmp/out/file.txt"))
	assert.True(t, isWithinDir("/tmp/out", "/tmp/out"))
	assert.False(t, isWithinDir("/tmp/out", "/tmp/escaped.txt"))
	assert.False(t, isWithinDir("/tmp/out", "/tmp/out-other/file.txt"))
}

func TestCopyProgressReportsFinalSize(t *testing.T) {
	var copied, progress bytes.Buffer
	p := newCopyProgress(&copied, &progress)

	_, err := p.Write([]byte("capture output"))
	require.NoError(t, err)
	p.Finish()

	assert.Equal(t, "capture output", copied.String())
	assert.Contains(t, progress.String(), "Copied 14 B")
}

func TestFormatBytes(t *testing.T) {
	assert.Equal(t, "512 B", formatBytes(512))
	assert.Equal(t, "1.0 KB", formatBytes(1024))
	assert.Equal(t, "1.0 MB", formatBytes(1024*1024))
}

func TestBuildJSONDropsPossiblyIncompleteLastLineAndStripsComma(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "flows_capture_2024.txt")
	content := `{"a":1},
{"a":2},
{"a":3
`
	require.NoError(t, os.WriteFile(txtPath, []byte(content), 0o644))

	jsonPath, err := BuildJSON(txtPath)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "flows_capture_2024.json"), jsonPath)

	got, err := os.ReadFile(jsonPath)
	require.NoError(t, err)
	assert.Equal(t, "[\n{\"a\":1},\n{\"a\":2}\n]\n", string(got))
}
