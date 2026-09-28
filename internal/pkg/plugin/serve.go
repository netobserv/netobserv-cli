package plugin

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// newServeCommand serves completed capture artifacts from a shared output volume.
func newServeCommand() *cobra.Command {
	var directory, listen string
	cmd := &cobra.Command{Use: "serve", Args: cobra.NoArgs, SilenceUsage: true}
	cmd.Flags().StringVar(&directory, "directory", "/output", "Capture output directory")
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:8080", "HTTP listen address (use loopback for authenticated Kubernetes port forwarding)")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		info, err := os.Stat(directory)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", directory)
		}
		listener, err := net.Listen("tcp", listen)
		if err != nil {
			return err
		}
		server := &http.Server{Handler: captureFileHandler(directory), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: time.Minute}
		return serveCaptureFiles(cmd.Context(), server, listener)
	}
	return cmd
}

func captureFileHandler(directory string) http.Handler {
	files := http.FileServer(http.Dir(directory))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		files.ServeHTTP(w, r)
	})
}

func serveCaptureFiles(ctx context.Context, server *http.Server, listener net.Listener) error {
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				_ = server.Close()
			}
		case <-finished:
		}
	}()
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
