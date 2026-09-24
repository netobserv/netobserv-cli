package plugin

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func archive(t *testing.T, entries map[string]string, typ byte) *bytes.Buffer {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for name, data := range entries {
		mustNoError(t, w.WriteHeader(&tar.Header{Name: name, Mode: 0600, Typeflag: typ, Size: int64(len(data))}))
		_, err := w.Write([]byte(data))
		mustNoError(t, err)
	}
	mustNoError(t, w.Close())
	return &b
}
func TestCopyOutput(t *testing.T) {
	dir := t.TempDir()
	b := archive(t, map[string]string{"./flow/capture.txt": "{\"SrcPort\":80},\n{\"SrcPort\":443},\n", "./pcap/capture.pcap": "binary data"}, tar.TypeReg)
	mustNoError(t, extractOutput(b, dir))
	data, err := os.ReadFile(filepath.Join(dir, "flow/capture.json"))
	mustNoError(t, err)
	var flows []map[string]int
	mustNoError(t, json.Unmarshal(data, &flows))
	assert.Len(t, flows, 2)
	_, err = os.Stat(filepath.Join(dir, "flow/capture.txt"))
	assert.True(t, os.IsNotExist(err))
	data, err = os.ReadFile(filepath.Join(dir, "pcap/capture.pcap"))
	mustNoError(t, err)
	assert.Equal(t, "binary data", string(data))
}
func TestCopyRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../escape", "/escape", "a/../../escape"} {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, extractOutput(archive(t, map[string]string{name: "bad"}, tar.TypeReg), t.TempDir()))
		})
	}
	root := t.TempDir()
	outside := t.TempDir()
	mustNoError(t, os.Symlink(outside, filepath.Join(root, "flow")))
	assert.Error(t, extractOutput(archive(t, map[string]string{"flow/escape": "bad"}, tar.TypeReg), root))
	_, err := os.Stat(filepath.Join(outside, "escape"))
	assert.True(t, os.IsNotExist(err))
}
func TestCopyInvalidFlowKeepsSource(t *testing.T) {
	dir := t.TempDir()
	assert.Error(t, extractOutput(archive(t, map[string]string{"flow/a.txt": "{invalid},\n"}, tar.TypeReg), dir))
	_, err := os.Stat(filepath.Join(dir, "flow/a.txt"))
	mustNoError(t, err)
}

func TestCopyConsumesTarPadding(t *testing.T) {
	stream := archive(t, map[string]string{"pcap/capture.pcap": "packets"}, tar.TypeReg)
	_, err := stream.Write(make([]byte, 32*1024))
	mustNoError(t, err)
	mustNoError(t, extractOutput(stream, t.TempDir()))
	assert.Zero(t, stream.Len(), "drain the complete stdout stream before closing it")
}
