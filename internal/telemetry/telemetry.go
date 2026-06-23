// Package telemetry provides anonymous usage telemetry for sworn.
//
// Telemetry is opt-in only, collected during sworn init, and managed
// post-init via sworn telemetry on|off|status. Every sworn invocation
// that is not a telemetry meta-command fires one non-blocking POST to
// https://api.sworn.sh/v1/events.
//
// Schema v1 fields:
//
//	{
//	  "v": 1,
//	  "install_id": "<UUID>",
//	  "cmd": "run",
//	  "sub": "parallel",
//	  "duration_ms": 1234,
//	  "exit_code": 0,
//	  "sworn_version": "0.1.0",
//	  "go_version": "go1.26",
//	  "os": "linux",
//	  "arch": "amd64"
//	}
//
// No code, file paths, slice IDs, model names, or user identity is
// ever collected. The endpoint is best-effort: Fire() is non-blocking
// and silently drops any error.
package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ConfigDir returns the filesystem directory for sworn config and
// telemetry sentinel files. Hardcoded to ~/.config/sworn/ matching
// spec ACs exactly (Coach Pin 5, option (a)).
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "sworn"), nil
}

// sentinelPath returns the full path to a sentinel file inside the
// config directory. Returns "" when ConfigDir fails.
func sentinelPath(name string) string {
	dir, err := ConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, name)
}

// fileExists reports whether the path exists and is not a directory.
func fileExists(path string) bool {
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// --- IsEnabled ---------------------------------------------------------

// IsEnabled returns true when telemetry is opted in.
//
// Logic:
//  1. SWORN_NO_TELEMETRY=1 env var present → disabled (session override)
//  2. ~/.config/sworn/.no-telemetry exists → disabled (permanent opt-out)
//  3. ~/.config/sworn/.telemetry-enabled exists → enabled
//  4. Neither file exists → disabled (no consent yet; init not run)
func IsEnabled() bool {
	if os.Getenv("SWORN_NO_TELEMETRY") == "1" {
		return false
	}
	if fileExists(sentinelPath(".no-telemetry")) {
		return false
	}
	if fileExists(sentinelPath(".telemetry-enabled")) {
		return true
	}
	return false
}

// --- InstallID ---------------------------------------------------------

var (
	installIDOnce sync.Once
	installID     string

	// httpClient is atomically read by Fire()'s goroutine,
	// allowing safe set/restore in tests without data races.
	httpClient atomic.Pointer[http.Client]
)

func init() {
	httpClient.Store(http.DefaultClient)
}

// HTTPClient returns the current HTTP client used by Fire().
func HTTPClient() *http.Client { return httpClient.Load() }

// SetHTTPClient replaces the HTTP client used by Fire().
// Used in tests to inject a client with a custom transport.
func SetHTTPClient(c *http.Client) { httpClient.Store(c) }

// InstallID returns the persistent install UUID.
//
// On first call, reads ~/.config/sworn/install-id. If the file does not
// exist, creates it with a new UUIDv4. Subsequent calls return the
// cached in-memory value. Returns "" on any I/O error (fail-open — the
// event fires without an install_id).
func InstallID() string {
	installIDOnce.Do(func() {
		p := sentinelPath("install-id")
		if p == "" {
			return
		}
		data, err := os.ReadFile(p)
		if err == nil {
			id := strings.TrimSpace(string(data))
			if id != "" {
				installID = id
				return
			}
		}
		// File missing or empty — generate new UUID.
		id, err := newUUID()
		if err != nil {
			return // installID stays ""
		}
		dir, _ := ConfigDir()
		if dir == "" {
			return
		}
		if mkErr := os.MkdirAll(dir, 0700); mkErr != nil {
			return
		}
		if writeErr := os.WriteFile(p, []byte(id+"\n"), 0600); writeErr != nil {
			return
		}
		installID = id
	})
	return installID
}

// resetInstallIDForTest clears the cached install ID so the next call
// to InstallID() re-reads (or re-creates) the file. Used in tests only.
func resetInstallIDForTest() {
	installIDOnce = sync.Once{}
	installID = ""
}

// newUUID generates a random UUIDv4 string.
func newUUID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	// Set version 4.
	buf[6] = (buf[6] & 0x0f) | 0x40
	// Set variant bits.
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16]), nil
}

// --- Event schema ------------------------------------------------------

type event struct {
	V            int    `json:"v"`
	InstallID    string `json:"install_id"`
	Cmd          string `json:"cmd"`
	Sub          string `json:"sub"`
	DurationMS   int64  `json:"duration_ms"`
	ExitCode     int    `json:"exit_code"`
	SwornVersion string `json:"sworn_version"`
	GoVersion    string `json:"go_version"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
}

// Fire sends a telemetry event to the telemetry endpoint.
//
// It runs in a goroutine and is non-blocking. Returns immediately.
// Any error (network, non-2xx response, timeout) is silently dropped.
// The caller passes sworn_version from the main package (Coach Pin 8,
// option (a)), avoiding circular imports.
func Fire(cmd, sub, swornVersion string, durationMS int64, exitCode int) {
	// Meta-command exclusion: sworn telemetry * does NOT fire telemetry
	// events (Coach Pin 4, option (a)). sworn version and sworn help
	// still fire — useful version-usage signal.
	if cmd == "telemetry" {
		return
	}

	// TUI / no-args exclusion: running sworn with no subcommand launches
	// the TUI interactively. The empty cmd + session-length duration is
	// junk data — exclude it (swornagent/sworn#7).
	if cmd == "" {
		return
	}
	go func() {
		evt := event{
			V:            1,
			InstallID:    InstallID(),
			Cmd:          cmd,
			Sub:          sub,
			DurationMS:   durationMS,
			ExitCode:     exitCode,
			SwornVersion: swornVersion,
			GoVersion:    trimGoVersion(runtime.Version()),
			OS:           runtime.GOOS,
			Arch:         runtime.GOARCH,
		}
		body, err := json.Marshal(evt)
		if err != nil {
			return // silently drop
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			"https://api.sworn.sh/v1/events", bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Load().Do(req)
		if err != nil {
			return // network error — silently drop
		}
		resp.Body.Close()
	}()
}

// trimGoVersion reduces "go1.26.0" to "go1.26" (major.minor only)
// per Coach Flag (b). If the version string doesn't start with "go",
// returns it unchanged.
func trimGoVersion(v string) string {
	if !strings.HasPrefix(v, "go") {
		return v
	}
	parts := strings.SplitN(v[2:], ".", 3)
	if len(parts) < 2 {
		return v
	}
	return "go" + parts[0] + "." + parts[1]
}

// --- ShowDisclosure ----------------------------------------------------

// ShowDisclosure prints the one-time telemetry disclosure to w.
//
// It only prints when the user is in a neutral/undecided state — neither
// opted in nor opted out. Once the disclosure has been shown, it writes
// the ~/.config/sworn/.telemetry-disclosed sentinel file so that
// subsequent invocations do not re-display.
//
// The neutrality precondition (Coach Pin 6): the disclosure only prints
// if neither .telemetry-enabled nor .no-telemetry exists AND the
// .telemetry-disclosed sentinel is absent. This prevents re-displaying
// the disclosure to a user who has already made a consent decision.
func ShowDisclosure(w io.Writer) {
	dir, err := ConfigDir()
	if err != nil {
		return // silently skip — can't determine config path
	}

	// Neutral-state precondition: only show if the user hasn't made
	// a consent decision yet.
	if fileExists(filepath.Join(dir, ".telemetry-enabled")) ||
		fileExists(filepath.Join(dir, ".no-telemetry")) {
		return // user already made a choice
	}

	disclosedPath := filepath.Join(dir, ".telemetry-disclosed")
	if fileExists(disclosedPath) {
		return // already disclosed this session
	}

	fmt.Fprint(w, `sworn collects anonymous usage telemetry to improve the product.
Data collected: command names, run durations, exit codes, sworn version,
operating system and architecture. No code, file paths, project names,
slice IDs, model names, or user identity is ever collected.

Run 'sworn telemetry status' to check your current setting, or
'sworn telemetry on' / 'sworn telemetry off' to change it.

`)
	// Write sentinel file so we don't print again.
	if err := os.MkdirAll(dir, 0700); err != nil {
		return // skip sentinel write — already printed once this run
	}
	_ = os.WriteFile(disclosedPath, []byte{}, 0600) // best-effort
}

// --- ShowConsent -------------------------------------------------------

// ShowConsent prompts the user for telemetry consent and returns true if
// the user opted in.
//
// Signature (Coach Pin 7):
//
//	ShowConsent(r io.Reader, w io.Writer) (bool, error)
//
// r provides stdin input (user's Y/n response). w receives the prompt.
// Returns (true, nil) when the user answers Y/Enter, (false, nil) when
// the user answers n/N. Returns (false, err) on I/O error reading input.
//
// Non-interactive mode is not handled here — the caller (sworn init in
// T3/S09) checks for --non-interactive and defaults to opted-out without
// calling ShowConsent.
func ShowConsent(r io.Reader, w io.Writer) (bool, error) {
	fmt.Fprint(w, `sworn collects anonymous usage telemetry to improve the product.
Data collected: command names, durations, exit codes, sworn version, OS/arch.
No code, specs, file paths, project names, or user identity is collected.
Schema: https://sworn.dev/telemetry

Enable telemetry? [Y/n]: `)

	var answer string
	_, err := fmt.Fscanln(r, &answer)
	if err != nil {
		// EOF (e.g. empty Enter) or other read error — treat as Y default.
		return true, nil
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "n" || answer == "no" {
		return false, nil
	}
	return true, nil
}