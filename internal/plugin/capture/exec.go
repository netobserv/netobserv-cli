package capture

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"

	"github.com/netobserv/network-observability-cli/internal/k8s/client"
)

// ExecOptions configures ExecInPod, replacing `oc exec -i --tty -n <ns> <pod> -- <command>`.
type ExecOptions struct {
	Namespace string
	Pod       string
	Container string
	Command   []string
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	TTY       bool
}

// ExecInPod runs Command inside a running pod/container over a SPDY exec stream, replacing
// `oc exec`. When TTY is set, stdin's file descriptor is put into raw mode for the duration of
// the call and terminal resizes (SIGWINCH) are forwarded to the remote process, the same
// behavior `oc`/`kubectl exec -t` provide.
func ExecInPod(ctx context.Context, client *k8sclient.Client, opts *ExecOptions) error {
	req := client.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(opts.Namespace).
		Name(opts.Pod).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: opts.Container,
			Command:   opts.Command,
			Stdin:     opts.Stdin != nil,
			Stdout:    opts.Stdout != nil,
			Stderr:    opts.Stderr != nil,
			TTY:       opts.TTY,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(client.RESTConfig, http.MethodPost, req.URL())
	if err != nil {
		return fmt.Errorf("cannot create exec stream for pod %s/%s: %w", opts.Namespace, opts.Pod, err)
	}

	stdin, releaseStdin := isolateTerminalStdin(opts.Stdin)
	defer releaseStdin()

	streamOpts := remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
		Tty:    opts.TTY,
	}

	run := func() error {
		streamCtx := ctx
		if opts.Stdin != nil {
			// remotecommand logs a noisy "Copying stdin failed: EOF" message
			// when an interactive command exits and the server closes stdin. The
			// stream error is still returned normally; only the transport logger
			// is silenced for this expected shutdown path.
			streamCtx = klog.NewContext(ctx, logr.Discard())
		}
		return executor.StreamWithContext(streamCtx, streamOpts)
	}

	if opts.TTY {
		return runWithRawTerminal(run, &streamOpts)
	}
	return run()
}

// isolateTerminalStdin gives remotecommand its own terminal descriptor. The Kubernetes exec
// client copies stdin in a background goroutine that can outlive StreamWithContext by a small
// amount; sharing os.Stdin directly lets that goroutine consume the first answer to the cleanup
// prompt after the collector exits.
func isolateTerminalStdin(stdin io.Reader) (io.Reader, func()) {
	file, ok := stdin.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return stdin, func() {}
	}

	fd, err := unix.Dup(int(file.Fd()))
	if err != nil {
		return stdin, func() {}
	}
	duplicate := os.NewFile(uintptr(fd), file.Name())
	reader := &terminalStdin{file: duplicate, closed: make(chan struct{}), done: make(chan struct{})}

	return reader, func() {
		close(reader.closed)
		select {
		case <-reader.done:
		case <-time.After(time.Second):
		}
		_ = duplicate.Close()
	}
}

type terminalStdin struct {
	file   *os.File
	closed chan struct{}
	done   chan struct{}
	once   sync.Once
}

func (r *terminalStdin) Read(p []byte) (int, error) {
	for {
		select {
		case <-r.closed:
			r.finish()
			return 0, io.EOF
		default:
		}

		fds := []unix.PollFd{{Fd: int32(r.file.Fd()), Events: unix.POLLIN}}
		if _, err := unix.Poll(fds, 50); err != nil {
			if err == unix.EINTR {
				continue
			}
			r.finish()
			return 0, err
		}
		if fds[0].Revents&unix.POLLIN == 0 {
			continue
		}

		n, err := r.file.Read(p)
		if err != nil {
			r.finish()
		}
		return n, err
	}
}

func (r *terminalStdin) finish() {
	r.once.Do(func() { close(r.done) })
}

// runWithRawTerminal puts stdin into raw mode (if it's actually a terminal; a no-op otherwise,
// e.g. under the e2e test harness's piped stdin) and wires up SIGWINCH-driven resize events on
// streamOpts for the duration of run.
func runWithRawTerminal(run func() error, streamOpts *remotecommand.StreamOptions) error {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return run()
	}

	sizeQueue := newTerminalSizeQueue(fd)
	defer sizeQueue.stop()
	streamOpts.TerminalSizeQueue = sizeQueue

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("cannot set terminal to raw mode: %w", err)
	}
	defer func() { _ = term.Restore(fd, oldState) }()

	return run()
}

// terminalSizeQueue is a small, dependency-light implementation of
// remotecommand.TerminalSizeQueue (kubectl's own equivalent lives in
// k8s.io/kubectl/pkg/util/term, which would pull in k8s.io/cli-runtime as a dependency just for
// this) built directly on golang.org/x/term, which the project already depends on indirectly.
type terminalSizeQueue struct {
	sizes chan remotecommand.TerminalSize
	sigCh chan os.Signal
}

func newTerminalSizeQueue(fd int) *terminalSizeQueue {
	q := &terminalSizeQueue{
		sizes: make(chan remotecommand.TerminalSize, 1),
		sigCh: make(chan os.Signal, 1),
	}
	signal.Notify(q.sigCh, syscall.SIGWINCH)
	go q.watch(fd)
	q.sizes <- terminalSize(fd) // seed the initial size immediately
	return q
}

func (q *terminalSizeQueue) watch(fd int) {
	for range q.sigCh {
		select {
		case q.sizes <- terminalSize(fd):
		default: // a resize is already queued; the latest size will be picked up on the next tick anyway
		}
	}
}

func (q *terminalSizeQueue) Next() *remotecommand.TerminalSize {
	size, ok := <-q.sizes
	if !ok {
		return nil
	}
	return &size
}

func (q *terminalSizeQueue) stop() {
	signal.Stop(q.sigCh)
	close(q.sigCh)
}

func terminalSize(fd int) remotecommand.TerminalSize {
	width, height, err := term.GetSize(fd)
	if err != nil {
		return remotecommand.TerminalSize{}
	}
	return remotecommand.TerminalSize{Width: uint16(width), Height: uint16(height)} //nolint:gosec // terminal dimensions never approach uint16 overflow
}
