package plugin

import (
	"archive/tar"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func (c *cluster) copyOutput(ctx context.Context, ns, destination string, errOut io.Writer) error {
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		err := c.exec(ctx, ns, []string{"tar", "cf", "-", "-C", "output", "."}, nil, writer, errOut, false)
		_ = writer.CloseWithError(err)
		done <- err
	}()
	err := extractOutput(reader, destination)

	_ = reader.CloseWithError(err)
	return errors.Join(err, <-done)
}

// Extract through os.Root so even existing symlinks cannot escape destination.
func extractOutput(r io.Reader, destination string) error {
	if err := os.MkdirAll(destination, 0700); err != nil {
		return err
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	tr := tar.NewReader(r)
	var flowFiles []string
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name := path.Clean(h.Name)
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("invalid archive path %q", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err = root.MkdirAll(name, 0700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err = writeArchiveFile(root, name, tr); err != nil {
				return err
			}
			if strings.HasSuffix(name, ".txt") {
				flowFiles = append(flowFiles, name)
			}
		default:
			return fmt.Errorf("unsupported archive entry %q (type %d)", h.Name, h.Typeflag)
		}
	}
	// tar stops at its end markers; drain transport padding before closing stdout.
	if _, err = io.Copy(io.Discard, r); err != nil {
		return err
	}
	for _, name := range flowFiles {
		if err = convertFlows(root, name); err != nil {
			return err
		}
	}
	return nil
}

func convertFlows(root *os.Root, name string) error {
	input, err := root.Open(name)
	if err != nil {
		return err
	}
	defer input.Close()
	target := strings.TrimSuffix(name, filepath.Ext(name)) + ".json"
	output, err := root.OpenFile(target+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = output.Close(); _ = root.Remove(target + ".tmp") }()
	if _, err = io.WriteString(output, "[\n"); err != nil {
		return err
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = strings.TrimSuffix(line, ",")
		if line == "" {
			continue
		}
		if !json.Valid([]byte(line)) {
			return fmt.Errorf("invalid flow record in %s", name)
		}
		if !first {
			if _, err = io.WriteString(output, ",\n"); err != nil {
				return err
			}
		}
		if _, err = io.WriteString(output, line); err != nil {
			return err
		}
		first = false
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	if _, err = io.WriteString(output, "\n]\n"); err != nil {
		return err
	}
	if err = output.Close(); err != nil {
		return err
	}
	if err = root.Rename(target+".tmp", target); err != nil {
		return err
	}
	return root.Remove(name)
}

func writeArchiveFile(root *os.Root, name string, r io.Reader) error {
	if err := root.MkdirAll(path.Dir(name), 0700); err != nil {
		return err
	}
	f, err := root.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, r)
	return errors.Join(copyErr, f.Close())
}
