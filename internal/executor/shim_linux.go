//go:build linux

package executor

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const engineDeathExitCode = 125

// RunShim owns the final lifetime link between the Sworn engine and the
// contained process. The engine holds stdin open for the whole invocation. If
// that pipe closes before Bubblewrap exits, the shim terminates its child and
// returns; systemd then removes every remaining process in the service cgroup.
func RunShim(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if err := validateContainedArgv(argv); err != nil {
		_, _ = fmt.Fprintf(stderr, "sworn executor shim: %v\n", err)
		return 126
	}
	command := exec.Command(argv[0], argv[1:]...)
	command.Stdin = nil
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		_, _ = fmt.Fprintf(stderr, "sworn executor shim: start contained process: %v\n", err)
		if errors.Is(err, exec.ErrNotFound) {
			return 127
		}
		return 126
	}

	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	engineGone := make(chan struct{}, 1)
	go func() {
		_, _ = io.Copy(io.Discard, stdin)
		engineGone <- struct{}{}
	}()

	select {
	case err := <-done:
		return processExitCode(err)
	case <-engineGone:
		if command.Process != nil {
			_ = command.Process.Signal(syscall.SIGTERM)
		}
		timer := time.NewTimer(shutdownGrace)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			if command.Process != nil {
				_ = command.Process.Kill()
			}
			<-done
		}
		return engineDeathExitCode
	}
}

func validateContainedArgv(argv []string) error {
	if len(argv) == 0 || !filepath.IsAbs(argv[0]) {
		return errors.New("contained argv requires an absolute executable")
	}
	for _, argument := range argv {
		if strings.ContainsRune(argument, '\x00') || len(argument) > 1<<20 {
			return errors.New("contained argv contains an invalid argument")
		}
	}
	return nil
}
