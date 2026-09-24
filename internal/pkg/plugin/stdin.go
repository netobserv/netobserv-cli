package plugin

import (
	"errors"
	"io"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// execInput lets the remote stdin copier stop without closing the caller's file.
// In particular, it must release the terminal before the local copy prompt reads it.
type execInput struct {
	fd     int
	done   chan struct{}
	once   sync.Once
	readMu sync.Mutex
}

func newExecInput(f *os.File) *execInput {
	return &execInput{fd: int(f.Fd()), done: make(chan struct{})}
}

func (r *execInput) stop() {
	r.once.Do(func() { close(r.done) })
	// Wait for an in-flight read to finish before returning stdin to the prompt.
	r.readMu.Lock()
	r.fd = -1 // This adapter no longer owns access to the caller's descriptor.
	r.readMu.Unlock()
}

func (r *execInput) Read(p []byte) (int, error) {
	r.readMu.Lock()
	defer r.readMu.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	for {
		select {
		case <-r.done:
			return 0, io.EOF
		default:
		}
		fds := []unix.PollFd{{Fd: int32(r.fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, 50)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if n == 0 {
			continue
		}
		select {
		case <-r.done:
			return 0, io.EOF
		default:
		}
		n, err = unix.Read(r.fd, p)
		if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if n == 0 {
			return 0, io.EOF
		}
		return n, err
	}
}
