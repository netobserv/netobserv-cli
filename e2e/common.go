package e2e

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/sirupsen/logrus"
)

const (
	// How long StartCommand lets the command run before snapshotting its output.
	// The command keeps running afterwards, up to its own --max-time.
	StartCommandWait  = 20 * time.Second
	RunCommandTimeout = 60 * time.Second
)

var (
	StartupDate = time.Now().Format("20060102-150405")

	// Commands started by StartCommand, which outlive the call. Their PTY master must stay
	// open: once the last reference is dropped, the runtime finalizer closes the file
	// descriptor, the kernel sends SIGHUP to the foreground process group and the command dies
	// early -- running its EXIT trap, which tears the capture down before the test looked at it.
	startedCommands   []startedCommand
	startedCommandsMu sync.Mutex
)

type startedCommand struct {
	cmd  *exec.Cmd
	ptmx *os.File
}

// trackStarted keeps a reference to the PTY master until StopStartedCommands is called, and
// drains it so its buffer never fills up.
func trackStarted(cmd *exec.Cmd, ptmx *os.File) {
	startedCommandsMu.Lock()
	startedCommands = append(startedCommands, startedCommand{cmd: cmd, ptmx: ptmx})
	startedCommandsMu.Unlock()

	go func() {
		_, _ = io.Copy(io.Discard, ptmx)
	}()
}

// StopStartedCommands kills every command left running by StartCommand. Call it before deleting
// the capture resources, otherwise a still-running CLI would reach its own EXIT trap later on and
// clean up whatever the next test has created in the meantime.
func StopStartedCommands(log *logrus.Entry) {
	startedCommandsMu.Lock()
	pending := startedCommands
	startedCommands = nil
	startedCommandsMu.Unlock()

	for _, sc := range pending {
		if sc.cmd.Process != nil {
			// pty.Start gives the command its own session, so the negated PID addresses the
			// whole process group: the CLI script and the oc processes it spawned.
			if err := syscall.Kill(-sc.cmd.Process.Pid, syscall.SIGKILL); err != nil {
				log.Debugf("Could not kill process group %d: %v", sc.cmd.Process.Pid, err)
			}
			_, _ = sc.cmd.Process.Wait()
		}
		_ = sc.ptmx.Close()
	}
}

// start command with tty support and wait for some time before returning output
// the command will keep running after this call
func StartCommand(log *logrus.Entry, commandName string, arg ...string) (string, error) {
	log.WithFields(logrus.Fields{"cmd": commandName, "arg": arg}).Info("Starting command")

	cmd := exec.Command(commandName, arg...)
	cmd.Env = append(cmd.Environ(), "isE2E=true")

	outPipe, _ := cmd.StdoutPipe()
	errPipe, _ := cmd.StderrPipe()

	var sbOut strings.Builder
	var sbErr strings.Builder

	go func(_ io.ReadCloser) {
		reader := bufio.NewReader(errPipe)
		for {
			line, err := reader.ReadString('\n')
			// Write line even if there's an error, as long as we got data
			if len(line) > 0 {
				sbErr.WriteString(line)
			}
			if err != nil {
				break
			}
		}
	}(errPipe)

	go func(_ io.ReadCloser) {
		reader := bufio.NewReader(outPipe)
		for {
			line, err := reader.ReadString('\n')
			// Write line even if there's an error, as long as we got data
			if len(line) > 0 {
				sbOut.WriteString(line)
			}
			if err != nil {
				break
			}
		}
	}(outPipe)

	log.Debug("Starting ...")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Errorf("Start returned error: %v", err)
		return "", err
	}
	// The command keeps running after this function returns, so the PTY must not be closed here.
	trackStarted(cmd, ptmx)

	log.Debugf("Waiting %v ...", StartCommandWait)
	time.Sleep(StartCommandWait)

	log.Debug("Returning result while command still running")
	// Combine stderr first (errors more visible), then stdout
	return sbErr.String() + sbOut.String(), nil
}

// run command with tty support and wait for stop
func RunCommand(log *logrus.Entry, commandName string, arg ...string) (string, error) {
	log.WithFields(logrus.Fields{"cmd": commandName, "arg": arg}).Info("Running command")

	cmd := exec.Command(commandName, arg...)
	cmd.Env = append(cmd.Environ(), "isE2E=true")

	outPipe, _ := cmd.StdoutPipe()
	errPipe, _ := cmd.StderrPipe()

	var sbOut strings.Builder
	var sbErr strings.Builder
	var wg sync.WaitGroup

	wg.Add(2)
	go func(_ io.ReadCloser) {
		defer wg.Done()
		reader := bufio.NewReader(errPipe)
		for {
			line, err := reader.ReadString('\n')
			// Write line even if there's an error, as long as we got data
			if len(line) > 0 {
				sbErr.WriteString(line)
			}
			if err != nil {
				break
			}
		}
	}(errPipe)

	go func(_ io.ReadCloser) {
		defer wg.Done()
		reader := bufio.NewReader(outPipe)
		for {
			line, err := reader.ReadString('\n')
			// Write line even if there's an error, as long as we got data
			if len(line) > 0 {
				sbOut.WriteString(line)
			}
			if err != nil {
				break
			}
		}
	}(outPipe)

	log.Debug("Starting ...")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Errorf("Start returned error: %v", err)
		return "", err
	}
	defer ptmx.Close() // Ensure PTY is closed after command finishes

	log.Debug("Waiting ...")
	err = cmd.Wait()
	if err != nil {
		log.Errorf("Wait returned error: %v", err)
	}

	log.Debug("Waiting for output goroutines to finish...")
	wg.Wait()

	// TODO: find why this returns -1. That may be related to pty implementation
	/*if cmd.ProcessState.ExitCode() != 0 {
		return sbErr.String() + sbOut.String(), fmt.Errorf("Cmd returned code %d", cmd.ProcessState.ExitCode())
	}*/

	// Combine stderr first (errors more visible), then stdout
	return sbErr.String() + sbOut.String(), nil
}

// run command with tty support and terminate it after timeout
// it will also simulate a keyboard input during the run
func RunCommandAndTerminate(log *logrus.Entry, commandName string, arg ...string) (string, error) {
	log.WithFields(logrus.Fields{"cmd": commandName, "arg": arg}).Info("Running command and terminate")

	cmd := exec.Command(commandName, arg...)
	cmd.Env = append(cmd.Environ(), "isE2E=true")

	outPipe, _ := cmd.StdoutPipe()
	errPipe, _ := cmd.StderrPipe()

	timer := time.AfterFunc(RunCommandTimeout, func() {
		log.Debug("Terminating command...")
		err := cmd.Process.Signal(syscall.SIGTERM)
		if err != nil {
			log.Error(err)
		}
	})
	defer timer.Stop()

	var sbOut strings.Builder
	var sbErr strings.Builder
	var wg sync.WaitGroup

	wg.Add(2)
	go func(_ io.ReadCloser) {
		defer wg.Done()
		reader := bufio.NewReader(errPipe)
		for {
			line, err := reader.ReadString('\n')
			// Write line even if there's an error, as long as we got data
			if len(line) > 0 {
				sbErr.WriteString(line)
			}
			if err != nil {
				break
			}
		}
	}(errPipe)

	go func(_ io.ReadCloser) {
		defer wg.Done()
		reader := bufio.NewReader(outPipe)
		for {
			line, err := reader.ReadString('\n')
			// Write line even if there's an error, as long as we got data
			if len(line) > 0 {
				sbOut.WriteString(line)
			}
			if err != nil {
				break
			}
		}
	}(outPipe)

	log.Debug("Starting ...")
	in, err := pty.Start(cmd)
	if err != nil {
		log.Errorf("Start returned error: %v", err)
		return "", err
	}

	timer = time.AfterFunc(30*time.Second, func() {
		log.Debug("Simulating keyboard typing...")
		_, err := in.Write([]byte("netobserv"))
		if err != nil {
			log.Error(err)
		}
	})
	defer timer.Stop()

	log.Debug("Waiting ...")
	err = cmd.Wait()
	if err != nil {
		log.Errorf("Wait returned error: %v", err)
	}

	log.Debug("Waiting for output goroutines to finish...")
	wg.Wait()

	// TODO: find why this returns -1. That may be related to pty implementation
	/*if cmd.ProcessState.ExitCode() != 0 {
		return sbErr.String() + sbOut.String(), fmt.Errorf("Cmd returned code %d", cmd.ProcessState.ExitCode())
	}*/

	// Combine stderr first (errors more visible), then stdout
	return sbErr.String() + sbOut.String(), nil
}
