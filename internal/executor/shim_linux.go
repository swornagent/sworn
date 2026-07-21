//go:build linux

package executor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	engineDeathExitCode     = 125
	shimStartMarkerArgument = "--sworn-start-marker"
	bubblewrapStatusFD      = 3
)

// RunShim owns the final lifetime link between the Sworn engine and the
// contained process. The engine holds stdin open for the whole invocation. If
// that pipe closes before Bubblewrap exits, the shim terminates its child and
// returns; systemd then removes every remaining process in the service cgroup.
func RunShim(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	startMarker, broker, containedArgv, err := parseShimArgv(argv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sworn executor shim: %v\n", err)
		return 126
	}
	var credential *os.File
	if broker != nil {
		credential, err = receiveCredentialFromBroker(*broker)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "sworn executor shim: receive credential capability: %v\n", err)
			return 126
		}
		defer credential.Close() //nolint:errcheck
	}
	statusReader, statusWriter, err := os.Pipe()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "sworn executor shim: create Bubblewrap status pipe: %v\n", err)
		return 126
	}
	defer statusReader.Close() //nolint:errcheck
	defer statusWriter.Close() //nolint:errcheck
	bubblewrapArgs := append(
		[]string{"--json-status-fd", fmt.Sprint(bubblewrapStatusFD)},
		containedArgv[1:]...,
	)
	command := exec.Command(containedArgv[0], bubblewrapArgs...)
	command.Stdin = nil
	command.Stdout = stdout
	command.Stderr = stderr
	command.ExtraFiles = []*os.File{statusWriter}
	if credential != nil {
		command.ExtraFiles = append(command.ExtraFiles, credential)
	}
	if err := command.Start(); err != nil {
		_, _ = fmt.Fprintf(stderr, "sworn executor shim: start contained process: %v\n", err)
		if errors.Is(err, exec.ErrNotFound) {
			return 127
		}
		return 126
	}
	_ = statusWriter.Close()

	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	started := make(chan error, 1)
	statusDone := make(chan error, 1)
	go observeBubblewrapStatus(statusReader, startMarker, started, statusDone)
	engineGone := make(chan struct{}, 1)
	go func() {
		_, _ = io.Copy(io.Discard, stdin)
		engineGone <- struct{}{}
	}()

	select {
	case startErr := <-started:
		if startErr != nil {
			_ = stopShimChild(command, done)
			<-statusDone
			_, _ = fmt.Fprintf(stderr, "sworn executor shim: establish contained process: %v\n", startErr)
			return 126
		}
		select {
		case runErr := <-done:
			if statusErr := <-statusDone; statusErr != nil {
				_, _ = fmt.Fprintf(stderr, "sworn executor shim: read Bubblewrap status: %v\n", statusErr)
				return 126
			}
			return processExitCode(runErr)
		case <-engineGone:
			_ = stopShimChild(command, done)
			<-statusDone
			return engineDeathExitCode
		}
	case runErr := <-done:
		startErr := <-started
		statusErr := <-statusDone
		if startErr != nil || statusErr != nil {
			if startErr == nil {
				startErr = statusErr
			}
			_, _ = fmt.Fprintf(stderr, "sworn executor shim: establish contained process: %v\n", startErr)
			return 126
		}
		return processExitCode(runErr)
	case <-engineGone:
		_ = stopShimChild(command, done)
		<-statusDone
		return engineDeathExitCode
	}
}

func parseShimArgv(argv []string) (string, *credentialBrokerClient, []string, error) {
	if len(argv) < 3 || argv[0] != shimStartMarkerArgument {
		return "", nil, nil, errors.New("start marker argument is required")
	}
	marker := argv[1]
	if !filepath.IsAbs(marker) || filepath.Clean(marker) != marker || strings.ContainsRune(marker, '\x00') {
		return "", nil, nil, errors.New("start marker must be a clean absolute path")
	}
	var broker *credentialBrokerClient
	containedStart := 2
	if argv[2] == shimCredentialBrokerArgument {
		if len(argv) < 7 {
			return "", nil, nil, errors.New("credential broker arguments are incomplete")
		}
		candidate := credentialBrokerClient{
			runtimePath: argv[3],
			socketName:  argv[4],
			token:       argv[5],
		}
		if err := validateCredentialBrokerClient(candidate); err != nil {
			return "", nil, nil, err
		}
		broker = &candidate
		containedStart = 6
	}
	contained := argv[containedStart:]
	if err := validateContainedArgv(contained); err != nil {
		return "", nil, nil, err
	}
	return marker, broker, contained, nil
}

func observeBubblewrapStatus(
	reader io.Reader,
	startMarker string,
	started chan<- error,
	done chan<- error,
) {
	decoder := json.NewDecoder(reader)
	reported := false
	for {
		var status struct {
			ChildPID int `json:"child-pid"`
		}
		err := decoder.Decode(&status)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if !reported {
					started <- errors.New("Bubblewrap did not report a started child")
				}
				done <- nil
				return
			}
			if !reported {
				reported = true
				started <- fmt.Errorf("decode Bubblewrap status: %w", err)
			}
			done <- fmt.Errorf("decode Bubblewrap status: %w", err)
			return
		}
		if !reported && status.ChildPID > 0 {
			reported = true
			started <- createStartMarker(startMarker)
		}
	}
}

func createStartMarker(path string) error {
	descriptor, err := syscall.Open(
		path,
		syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0o600,
	)
	if err != nil {
		return fmt.Errorf("create private start marker: %w", err)
	}
	if err := syscall.Close(descriptor); err != nil {
		return fmt.Errorf("close private start marker: %w", err)
	}
	return nil
}

func validateStartMarker(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect private start marker: %w", err)
	}
	statistics, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 ||
		info.Size() != 0 || statistics.Nlink != 1 || int(statistics.Uid) != os.Geteuid() {
		return errors.New("private start marker binding is invalid")
	}
	return nil
}

func stopShimChild(command *exec.Cmd, done <-chan error) error {
	if command.Process != nil {
		_ = command.Process.Signal(syscall.SIGTERM)
	}
	timer := time.NewTimer(shutdownGrace)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		return <-done
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
