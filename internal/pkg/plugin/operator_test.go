package plugin

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHeadlessCaptureStaysForeground(t *testing.T) {
	for _, mode := range []string{"flows", "packets", "metrics"} {
		t.Run(mode, func(t *testing.T) {
			args := []string{"--headless", "--copy=true"}
			if mode == "packets" {
				args = append(args, "--port=443")
			}
			o, err := parseOptions(mode, args)
			mustNoError(t, err)
			assert.True(t, o.headless)
			assert.False(t, o.background)
			assert.Contains(t, strings.Join(o.collectorCommand(), " "), "background=true")
			m, err := buildManifests(o, false, nil)
			mustNoError(t, err)
			assert.Equal(t, []string{"sleep", "infinity"}, m.pod.Spec.Containers[0].Command)
		})
	}
}

func TestCaptureFileServer(t *testing.T) {
	directory := t.TempDir()
	mustNoError(t, os.WriteFile(filepath.Join(directory, "flows.json"), []byte("[]"), 0600))
	handler := captureFileHandler(directory)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/flows.json", nil))
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "[]", response.Body.String())
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/flows.json", nil))
	assert.Equal(t, http.StatusMethodNotAllowed, response.Code)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	mustNoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- serveCaptureFiles(ctx, &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}, listener)
	}()
	resp, err := http.Get("http://" + listener.Addr().String() + "/flows.json")
	mustNoError(t, err)
	_, err = io.Copy(io.Discard, resp.Body)
	mustNoError(t, err)
	mustNoError(t, resp.Body.Close())
	cancel()
	select {
	case err := <-result:
		mustNoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("server did not stop")
	}
}

func TestCaptureServerDefaultsToLoopback(t *testing.T) {
	command := newServeCommand()
	listen, err := command.Flags().GetString("listen")
	mustNoError(t, err)
	host, _, err := net.SplitHostPort(listen)
	mustNoError(t, err)
	assert.True(t, net.ParseIP(host).IsLoopback(), "unauthenticated artifacts must not be exposed on the pod network")
}
