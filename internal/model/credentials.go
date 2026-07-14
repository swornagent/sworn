package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Provider credentials live in ONE place: an XDG-conventional JSON file next to
// config.json.
//
//	$SWORN_CREDENTIALS_PATH  — exact path (tests, CI)
//	$SWORN_HOME              — directory (joined with "credentials.json")
//	Linux                    — $XDG_CONFIG_HOME/sworn/credentials.json,
//	                           else ~/.config/sworn/credentials.json
//	macOS                    — ~/Library/Application Support/sworn/credentials.json
//
// They used to live in ~/.sworn/.env — a dotenv file, outside XDG, beside a
// config.json that WAS in XDG. Worse, that file was loaded into the process
// environment by exactly one command (`sworn run` called model.LoadDotEnv; nothing
// else did). So a key written by `sworn init` was visible to the loop and invisible
// to `sworn llm-check`, `sworn verify`, `sworn reqverify` and the MCP server — they
// resolved a model correctly and then failed for want of a key that was sitting on
// disk the whole time.
//
// The fix is structural, not a sixth call site: the model layer needs the key, so
// the MODEL LAYER resolves it. Bootstrapping that lives in a caller is a step every
// other caller can forget, and the one that forgets is never the one you test.

// CredentialsPathEnv overrides the credentials file location outright.
const CredentialsPathEnv = "SWORN_CREDENTIALS_PATH"

// Credentials is the on-disk record: a provider name to its API key.
//
// Keys are addressed by PROVIDER, not by env-var name, because the env var is a
// transport detail and the provider is the fact.
type Credentials struct {
	Providers map[string]string `json:"providers"`
}

// CredentialsPath returns the XDG-conventional credentials file path.
func CredentialsPath() string {
	if p := os.Getenv(CredentialsPathEnv); p != "" {
		return p
	}
	if dir := os.Getenv("SWORN_HOME"); dir != "" {
		return filepath.Join(dir, "credentials.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "sworn", "credentials.json")
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "sworn", "credentials.json")
	}
	return filepath.Join(home, ".config", "sworn", "credentials.json")
}

// canonicalKeyEnv maps a provider to the env var the WIDER ECOSYSTEM already uses.
//
// No SWORN_ prefix. A key is a key: if you have OPENAI_API_KEY exported for every
// other tool on your machine, sworn should read it rather than demand you duplicate
// it under a private name. The SWORN_-prefixed vars are gone.
var canonicalKeyEnv = map[string]string{
	"openai":             "OPENAI_API_KEY",
	"openai-responses":   "OPENAI_API_KEY",
	"openai-completions": "OPENAI_API_KEY",
	"anthropic":          "ANTHROPIC_API_KEY",
	"google":             "GOOGLE_API_KEY",
	"xai":                "XAI_API_KEY",
	"groq":               "GROQ_API_KEY",
	"mistral":            "MISTRAL_API_KEY",
	"deepseek":           "DEEPSEEK_API_KEY",
	"openrouter":         "OPENROUTER_API_KEY",
	"cloudflare":         "CLOUDFLARE_API_KEY",
	"azure":              "AZURE_OPENAI_API_KEY",
	"github":             "GITHUB_TOKEN",
	"aws-access-key":     "AWS_ACCESS_KEY_ID",
	"aws-secret-key":     "AWS_SECRET_ACCESS_KEY",
}

var (
	credsOnce   sync.Once
	credsCached Credentials
)

// loadCredentials reads the credentials file once per process. A missing or
// unreadable file is not an error — the env vars may carry everything.
func loadCredentials() Credentials {
	credsOnce.Do(func() {
		credsCached = Credentials{Providers: map[string]string{}}
		p := CredentialsPath()
		if p == "" {
			return
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return
		}
		var c Credentials
		if err := json.Unmarshal(raw, &c); err != nil {
			return
		}
		if c.Providers != nil {
			credsCached = c
		}
	})
	return credsCached
}

// ResetCredentialsCacheForTest clears the process-wide credentials cache so a test
// can point SWORN_CREDENTIALS_PATH somewhere else and be believed.
func ResetCredentialsCacheForTest() {
	credsOnce = sync.Once{}
	credsCached = Credentials{}
}

// ProviderKey resolves a provider's API key:
//
//  1. the canonical env var (OPENAI_API_KEY, ANTHROPIC_API_KEY, …) — so a
//     12-factor deployment or CI can inject it without a file;
//  2. credentials.json (XDG).
//
// Returns "" when neither has it. The caller reports the miss with a remedy.
func ProviderKey(provider string) string {
	if envName, ok := canonicalKeyEnv[provider]; ok {
		if v := os.Getenv(envName); v != "" {
			return v
		}
	}
	return loadCredentials().Providers[provider]
}

// SaveCredentials writes the credentials file at 0600, creating its XDG directory.
// It merges into whatever is already there — writing one provider's key never drops
// another's.
func SaveCredentials(updates map[string]string) error {
	p := CredentialsPath()
	if p == "" {
		return os.ErrNotExist
	}

	// Read the CURRENT file rather than the cache: another process may have
	// written since we started, and a credential we silently drop is one the user
	// has to find and re-enter.
	current := Credentials{Providers: map[string]string{}}
	if raw, err := os.ReadFile(p); err == nil {
		var c Credentials
		if err := json.Unmarshal(raw, &c); err == nil && c.Providers != nil {
			current = c
		}
	}
	for k, v := range updates {
		if v == "" {
			continue
		}
		current.Providers[k] = v
	}

	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(p, raw, 0600); err != nil {
		return err
	}

	ResetCredentialsCacheForTest() // the file changed; do not serve a stale cache
	return nil
}

// ConfiguredProviders returns the providers that have a key, from either source.
// `sworn doctor` renders this — a key you cannot see is a key you cannot debug.
func ConfiguredProviders() []string {
	var out []string
	seen := map[string]bool{}
	for provider := range canonicalKeyEnv {
		if seen[provider] || ProviderKey(provider) == "" {
			continue
		}
		seen[provider] = true
		out = append(out, provider)
	}
	return out
}

// --- migration off the legacy locations ---

// legacyKeyEnv maps a provider to the SWORN_-prefixed env var it used to read.
// Kept ONLY so MigrateLegacyCredentials can find keys already on a machine; no
// resolution path consults these.
var legacyKeyEnv = map[string]string{
	"openai":     "SWORN_OPENAI_API_KEY",
	"anthropic":  "SWORN_ANTHROPIC_API_KEY",
	"google":     "SWORN_GOOGLE_API_KEY",
	"xai":        "SWORN_XAI_API_KEY",
	"groq":       "SWORN_GROQ_API_KEY",
	"mistral":    "SWORN_MISTRAL_API_KEY",
	"deepseek":   "SWORN_DEEPSEEK_API_KEY",
	"openrouter": "SWORN_OPENROUTER_API_KEY",
	"cloudflare": "SWORN_CLOUDFLARE_API_KEY",
	"azure":      "SWORN_AZURE_OPENAI_API_KEY",
	"github":     "SWORN_GITHUB_TOKEN",
}

// LegacyEnvPath is where keys used to live: a dotenv file OUTSIDE the XDG dir.
func LegacyEnvPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".sworn", ".env")
}

// FindLegacyCredentials reports keys still living in the old places — the
// SWORN_-prefixed env vars, and ~/.sworn/.env — WITHOUT writing anything.
//
// `sworn doctor` uses this to tell a user their keys are somewhere sworn no longer
// looks, which is the failure this migration exists to prevent: silently losing a
// key that is right there on disk.
func FindLegacyCredentials() map[string]string {
	found := map[string]string{}

	// The legacy dotenv file (never parsed into the environment except by one
	// command, which is how this whole class of bug started).
	if p := LegacyEnvPath(); p != "" {
		if raw, err := os.ReadFile(p); err == nil {
			vals := parseDotEnv(string(raw))
			for provider, legacy := range legacyKeyEnv {
				if v := vals[legacy]; v != "" {
					found[provider] = v
				}
				// A canonical name in the legacy file counts too.
				if v := vals[canonicalKeyEnv[provider]]; v != "" {
					found[provider] = v
				}
			}
		}
	}

	// SWORN_-prefixed env vars still exported in the user's shell.
	for provider, legacy := range legacyKeyEnv {
		if v := os.Getenv(legacy); v != "" {
			found[provider] = v
		}
	}
	return found
}

// MigrateLegacyCredentials copies any legacy keys into credentials.json and returns
// the providers it moved. It never overwrites a provider already present in the
// credentials file, and it never deletes the legacy sources — migration must not be
// able to lose a key.
func MigrateLegacyCredentials() ([]string, error) {
	legacy := FindLegacyCredentials()
	if len(legacy) == 0 {
		return nil, nil
	}

	existing := loadCredentials()
	updates := map[string]string{}
	var moved []string
	for provider, key := range legacy {
		if existing.Providers[provider] != "" {
			continue // already declared; the file wins
		}
		updates[provider] = key
		moved = append(moved, provider)
	}
	if len(updates) == 0 {
		return nil, nil
	}
	sort.Strings(moved)
	if err := SaveCredentials(updates); err != nil {
		return nil, err
	}
	return moved, nil
}

// parseDotEnv extracts KEY=VALUE pairs from dotenv content, tolerating comments,
// blanks, and surrounding quotes.
func parseDotEnv(content string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}
