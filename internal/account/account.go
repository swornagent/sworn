// Package account implements device-code authentication, credential persistence,
// and login-status checks for SwornAgent's CLI. It is the auth layer consumed by
// cmd/sworn/login.go and cmd/sworn/account.go.
package account

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Credentials represents a stored SwornAgent authentication session.
// Fields are tagged for JSON serialisation to match the file format.
//
// Approval note (Coach, approved-ack.md pin 1): json struct tags are required
// for AC3 compliance. Tier is free-text per Coach decision (approved-ack.md pin 5).
type Credentials struct {
	Token     string    `json:"token"`
	Email     string    `json:"email"`
	Tier      string    `json:"tier"`
	ExpiresAt time.Time `json:"expires_at"`
}

// configDir returns the platform-appropriate config directory for sworn.
// On Linux: $HOME/.config/sworn, on macOS: $HOME/Library/Application Support/sworn.
// Uses os.UserConfigDir() for correct XDG/macOS resolution.
func configDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		// Fallback: use HOME directly on platforms where UserConfigDir fails
		home, _ := os.UserHomeDir()
		if home != "" {
			return filepath.Join(home, ".config", "sworn")
		}
		return ""
	}
	return filepath.Join(base, "sworn")
}

// CredentialsPath returns the full path to the credentials JSON file.
func CredentialsPath() string {
	return filepath.Join(configDir(), "credentials.json")
}

// OpenBrowser tries to open a URL in the system browser, falling back to
// printing the URL to stderr. Platform-specific commands are tried in order:
// xdg-open (Linux), open (macOS), start (Windows). See spec Risks section.
//
// Exported so cmd/sworn can reuse it for `sworn account buy` (S06b).
func OpenBrowser(urlStr string) {
	openBrowser(urlStr)
}

// openBrowser tries to open a URL in the system browser, falling back to
// printing the URL to stderr. Platform-specific commands are tried in order:
// xdg-open (Linux), open (macOS), start (Windows). See spec Risks section.
func openBrowser(urlStr string) {
	switch runtime.GOOS {
	case "darwin":
		if err := exec.Command("open", urlStr).Start(); err == nil {
			return
		}
	case "windows":
		if err := exec.Command("cmd", "/c", "start", urlStr).Start(); err == nil {
			return
		}
	default:
		// Linux and everything else: try xdg-open first
		if err := exec.Command("xdg-open", urlStr).Start(); err == nil {
			return
		}
		// Fallback for other Unix-like systems
		if err := exec.Command("open", urlStr).Start(); err == nil {
			return
		}
	}
	// Fallback: print the URL. Never fail silently — the user must know the URL.
	fmt.Fprintf(os.Stderr, "Open this URL in your browser: %s\n", urlStr)
}

// deviceCodeResponse is the JSON response from the device-code endpoint.
type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	VerificationURI string `json:"verification_uri"`
	Interval        int    `json:"interval"`
}

// tokenResponse is the JSON response from the device-token polling endpoint.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	Email       string `json:"email"`
	Tier        string `json:"tier"`
	ExpiresIn   int    `json:"expires_in"`
	Error       string `json:"error,omitempty"`
}

// DeviceCodeFlow performs the OAuth2 device-code flow against the given
// auth endpoint. It POSTs to <authEndpoint>/device/code, displays the
// verification URI and device code to the user, opens the URI in the system
// browser, and polls <authEndpoint>/device/token until the user authenticates.
//
// authEndpoint is parameterised for testability (mock servers in tests).
// Production uses os.Getenv("SWORN_AUTH_URL") with an ldflags fallback
// (Coach decision, approved-ack.md pin 4).
//
// Returns the access token, email, and any error. Context cancellation
// aborts polling (returns ctx.Err()).
func DeviceCodeFlow(ctx context.Context, authEndpoint string) (token, email string, err error) {
	// Step 1: Request device code.
	codeURL := authEndpoint + "/device/code"
	resp, err := http.PostForm(codeURL, url.Values{})
	if err != nil {
		return "", "", fmt.Errorf("device code request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("reading device code response: %w", err)
	}

	var dcr deviceCodeResponse
	if err := json.Unmarshal(body, &dcr); err != nil {
		return "", "", fmt.Errorf("parsing device code response: %w", err)
	}

	// Step 2: Display verification URI and device code to the user.
	fmt.Fprintf(os.Stderr, "Device code: %s\n", dcr.DeviceCode)
	fmt.Fprintf(os.Stderr, "Verification URL: %s\n", dcr.VerificationURI)
	openBrowser(dcr.VerificationURI)

	// Step 3: Poll the token endpoint.
	interval := dcr.Interval
	if interval <= 0 {
		interval = 2 // default to 2 seconds if server omits interval
	}

	tokenURL := authEndpoint + "/device/token"
	pollData := url.Values{"device_code": {dcr.DeviceCode}}

	for {
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}

		pollResp, err := http.PostForm(tokenURL, pollData)
		if err != nil {
			return "", "", fmt.Errorf("token poll request: %w", err)
		}

		pollBody, err := io.ReadAll(pollResp.Body)
		pollResp.Body.Close()
		if err != nil {
			return "", "", fmt.Errorf("reading token poll response: %w", err)
		}

		var tr tokenResponse
		if err := json.Unmarshal(pollBody, &tr); err != nil {
			return "", "", fmt.Errorf("parsing token response: %w", err)
		}

		if tr.AccessToken != "" {
			// Successfully authenticated.
			return tr.AccessToken, tr.Email, nil
		}

		if tr.Error == "authorization_pending" || tr.Error == "slow_down" {
			// Still waiting for the user; continue polling.
			// slow_down indicates the client should increase the interval.
			if tr.Error == "slow_down" {
				interval += 1
			}
			continue
		}

		if tr.Error != "" {
			return "", "", fmt.Errorf("authentication error: %s", tr.Error)
		}
	}
}

// Save writes credentials to a JSON file at <dir>/credentials.json.
// Creates the directory (mode 0700) if it does not exist. Writes the file
// with mode 0600 (user-readable only). Following Coach decision
// (approved-ack.md pin 6), permissions are silently enforced at write time;
// no Load() check.
func Save(creds Credentials, dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating credentials directory: %w", err)
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling credentials: %w", err)
	}

	path := filepath.Join(dir, "credentials.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing credentials file: %w", err)
	}

	return nil
}

// Load reads credentials from <dir>/credentials.json. Returns nil, nil if the
// file does not exist (not logged in). Other errors (permissions, corrupt JSON)
// are surfaced. No Load() permissions warning per Coach decision
// (approved-ack.md pin 6).
func Load(dir string) (*Credentials, error) {
	path := filepath.Join(dir, "credentials.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading credentials: %w", err)
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}

	return &creds, nil
}

// IsLoggedIn returns true if creds is non-nil and has not expired.
// Returns false for nil creds or expired tokens.
func IsLoggedIn(creds *Credentials) bool {
	if creds == nil {
		return false
	}
	return time.Now().Before(creds.ExpiresAt)
}

// CreditsPath returns the full path to the credits cache JSON file.
func CreditsPath() string {
	return filepath.Join(configDir(), "credits.json")
}

// creditsResponse is the JSON response from the credits API endpoint.
type creditsResponse struct {
	Credits int `json:"credits"`
}

// FetchCredits queries the SwornAgent account API for the current credit
// balance and caches the result in ~/.config/sworn/credits.json. It uses
// the provided context for timeout control so it can be called non-blocking
// from `sworn run` startup without delaying the main flow.
//
// The credit unit is an integer count (Coach ack pin A). The
// credit→token→currency conversion rate is a backend concern, out of scope
// for this slice.
func FetchCredits(ctx context.Context, creds *Credentials) (int, error) {
	if creds == nil || creds.Token == "" {
		return 0, fmt.Errorf("account: not logged in")
	}

	// Derive the API host from the proxy default host (same compiled-in
	// pattern). SWORN_PROXY_URL override applies for testing.
	host := defaultProxyHost
	if override := os.Getenv("SWORN_PROXY_URL"); override != "" {
		host = strings.TrimRight(override, "/")
	}

	creditsURL := host + "/account/credits"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, creditsURL, nil)
	if err != nil {
		return 0, fmt.Errorf("account: build credits request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+creds.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("account: fetch credits: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("account: credits API returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("account: reading credits response: %w", err)
	}

	var cr creditsResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return 0, fmt.Errorf("account: parsing credits response: %w", err)
	}

	// Cache the result.
	cachePath := CreditsPath()
	cacheDir := filepath.Dir(cachePath)
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return cr.Credits, fmt.Errorf("account: creating cache dir: %w", err)
	}
	cacheData, _ := json.MarshalIndent(cr, "", "  ")
	_ = os.WriteFile(cachePath, cacheData, 0600)

	return cr.Credits, nil
}

// LoadCachedCredits reads the cached credit balance from
// ~/.config/sworn/credits.json. Returns 0, false if the cache is absent
// or unparseable.
func LoadCachedCredits() (int, bool) {
	data, err := os.ReadFile(CreditsPath())
	if err != nil {
		return 0, false
	}
	var cr creditsResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return 0, false
	}
	return cr.Credits, true
}
