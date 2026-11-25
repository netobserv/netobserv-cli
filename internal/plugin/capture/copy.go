package capture

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/netobserv/network-observability-cli/internal/k8s/client"
)

// CopyFromPod replaces `oc cp <ns>/<pod>:<remoteDir> <localDir>`: it execs `tar cf -` in the pod
// and extracts the resulting stream locally, the same technique `oc`/`kubectl cp` themselves use.
//
// The tar stream is buffered through a temp file rather than piped directly into the tar reader:
// archive/tar's Reader stops reading as soon as it recognizes the end-of-archive marker, which
// can leave a few trailing bytes of a single large remotecommand Write unconsumed. An io.Pipe's
// Write blocks until every byte it was given has been read, so that last partial Write — and
// with it the whole exec call — would never return.
func CopyFromPod(ctx context.Context, client *k8sclient.Client, namespace, pod, remoteDir, localDir string, progress io.Writer) error {
	tmp, err := os.CreateTemp("", "netobserv-cli-copy-*.tar")
	if err != nil {
		return fmt.Errorf("cannot create temporary file: %w", err)
	}
	defer func() {
		_ = os.Remove(tmp.Name())
	}()
	defer tmp.Close()

	var stderr bytes.Buffer
	copyProgress := newCopyProgress(tmp, progress)
	if err := ExecInPod(ctx, client, &ExecOptions{
		Namespace: namespace,
		Pod:       pod,
		Command:   []string{"tar", "cf", "-", "-C", remoteDir, "."},
		Stdout:    copyProgress,
		Stderr:    &stderr,
	}); err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return fmt.Errorf("cannot copy files from pod %s/%s: %w: %s", namespace, pod, err, message)
		}
		return fmt.Errorf("cannot copy files from pod %s/%s: %w", namespace, pod, err)
	}
	copyProgress.Finish()

	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("cannot rewind temporary file: %w", err)
	}
	if err := extractTar(tmp, localDir); err != nil {
		return fmt.Errorf("cannot extract files copied from pod %s/%s: %w", namespace, pod, err)
	}
	return nil
}

const copyProgressInterval = 10 << 20

type copyProgress struct {
	writer io.Writer
	out    io.Writer
	total  int64
	last   int64
}

func newCopyProgress(writer, out io.Writer) *copyProgress {
	return &copyProgress{writer: writer, out: out}
}

func (p *copyProgress) Write(data []byte) (int, error) {
	n, err := p.writer.Write(data)
	p.total += int64(n)
	if p.out != nil && p.total-p.last >= copyProgressInterval {
		p.report()
	}
	return n, err
}

func (p *copyProgress) Finish() {
	if p.out != nil {
		p.report()
	}
}

func (p *copyProgress) report() {
	fmt.Fprintf(p.out, "Copied %s\n", formatBytes(p.total))
	p.last = p.total
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	for _, suffix := range []string{"KB", "MB", "GB"} {
		value /= unit
		if value < unit || suffix == "GB" {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%d B", size)
}

func extractTar(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, hdr.Name)
		if !isWithinDir(destDir, target) {
			return fmt.Errorf("tar entry %q escapes destination directory", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeTarFile(tr, target); err != nil {
				return err
			}
		}
	}
}

func writeTarFile(r io.Reader, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

func isWithinDir(dir, target string) bool {
	dir = filepath.Clean(dir)
	target = filepath.Clean(target)
	return target == dir || strings.HasPrefix(target, dir+string(os.PathSeparator))
}
