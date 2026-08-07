//go:build linux

package e2e

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// The TUI is one of Sworn's five product surfaces, and it refuses to run
// without a real terminal. Proving its behavior therefore needs a real pty.
// The helpers below open one with plain syscalls, so the proof introduces no
// dependency the product does not already have.

type winsize struct {
	rows, cols, xpixel, ypixel uint16
}

func openPTY(t *testing.T, rows, cols uint16) (*os.File, *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open /dev/ptmx: %v", err)
	}
	unlock := int32(0)
	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSPTLCK,
		uintptr(unsafe.Pointer(&unlock)),
	); errno != 0 {
		master.Close()
		t.Fatalf("unlock pty: %v", errno)
	}
	var number uint32
	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL, master.Fd(), syscall.TIOCGPTN,
		uintptr(unsafe.Pointer(&number)),
	); errno != 0 {
		master.Close()
		t.Fatalf("resolve pty: %v", errno)
	}
	slave, err := os.OpenFile(
		fmt.Sprintf("/dev/pts/%d", number),
		os.O_RDWR|syscall.O_NOCTTY,
		0,
	)
	if err != nil {
		master.Close()
		t.Fatalf("open pty slave: %v", err)
	}
	size := winsize{rows: rows, cols: cols}
	if _, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSWINSZ,
		uintptr(unsafe.Pointer(&size)),
	); errno != 0 {
		master.Close()
		slave.Close()
		t.Fatalf("size pty: %v", errno)
	}
	return master, slave
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b[()][A-Z0-9]|\x1b[=>]`)

func stripANSI(value string) string {
	return ansiPattern.ReplaceAllString(value, "")
}

// ptySession is one real `sworn tui` process attached to a real terminal.
type ptySession struct {
	t       *testing.T
	master  *os.File
	command *exec.Cmd
	done    chan error

	mu     sync.Mutex
	output strings.Builder
}

func startPTYSession(
	t *testing.T,
	binary string,
	environment map[string]string,
	args ...string,
) *ptySession {
	t.Helper()
	master, slave := openPTY(t, 48, 140)
	command := exec.Command(binary, args...)
	overrides := map[string]string{"TERM": "xterm-256color"}
	for key, value := range environment {
		overrides[key] = value
	}
	command.Env = cleanEnvironment(overrides)
	command.Stdin, command.Stdout, command.Stderr = slave, slave, slave
	command.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, Setctty: true, Ctty: 0,
	}
	if err := command.Start(); err != nil {
		master.Close()
		slave.Close()
		t.Fatalf("start %v: %v", args, err)
	}
	// The child owns the slave end now; the parent must let go of it or the
	// master read never sees EOF.
	slave.Close()

	session := &ptySession{t: t, master: master, command: command, done: make(chan error, 1)}
	go func() {
		buffer := make([]byte, 4096)
		for {
			count, err := master.Read(buffer)
			if count > 0 {
				session.mu.Lock()
				session.output.Write(buffer[:count])
				session.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	go func() { session.done <- command.Wait() }()
	t.Cleanup(func() { session.stop() })
	return session
}

func (s *ptySession) screen() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return stripANSI(s.output.String())
}

// waitFor blocks until every wanted fragment has appeared in the terminal
// output, or fails the test with the whole screen for diagnosis.
func (s *ptySession) waitFor(label string, timeout time.Duration, wanted ...string) {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		screen := s.screen()
		missing := false
		for _, fragment := range wanted {
			if !strings.Contains(screen, fragment) {
				missing = true
				break
			}
		}
		if !missing {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	s.t.Fatalf("tui %s did not show %v; screen:\n%s", label, wanted, s.screen())
}

func (s *ptySession) send(keys string) {
	s.t.Helper()
	if _, err := s.master.WriteString(keys); err != nil {
		s.t.Fatalf("write to tui: %v", err)
	}
	// The board polls asynchronously; give the model a scheduling slot so a
	// burst of keys is not coalesced into one Update.
	time.Sleep(60 * time.Millisecond)
}

// openControls opens the board's available-controls overlay. The board loads
// asynchronously, so the keypress is retried until the overlay is really up
// rather than assumed from one frame.
func (s *ptySession) openControls() {
	s.t.Helper()
	s.waitFor("loaded board", 60*time.Second, "available controls")
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		s.clear()
		s.send("a")
		time.Sleep(150 * time.Millisecond)
		if strings.Contains(s.screen(), "AVAILABLE CONTROLS") {
			return
		}
	}
	s.t.Fatalf("tui did not open its controls; screen:\n%s", s.screen())
}

// clear drops everything shown so far, so a later waitFor cannot be satisfied
// by a stale frame.
func (s *ptySession) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.output.Reset()
}

func (s *ptySession) stop() {
	if s.command.Process != nil {
		_, _ = s.master.WriteString("q")
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
			_ = s.command.Process.Kill()
			<-s.done
		}
	}
	s.master.Close()
}

// quit ends the session and requires a clean exit.
func (s *ptySession) quit() {
	s.t.Helper()
	if _, err := s.master.WriteString("q"); err != nil {
		s.t.Fatalf("quit tui: %v", err)
	}
	select {
	case err := <-s.done:
		if err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 0 {
				s.t.Fatalf("tui exit: %v\nscreen:\n%s", err, s.screen())
			}
		}
	case <-time.After(15 * time.Second):
		_ = s.command.Process.Kill()
		<-s.done
		s.t.Fatalf("tui did not exit; screen:\n%s", s.screen())
	}
	s.command.Process = nil
}
