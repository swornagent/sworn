// Package registry is the single resolution authority for loop dispatch: an
// explicit prefix -> driver table with fail-fast role checking and
// dispatch-free enumeration (S05-driver-registry, N-04/N-09).
//
// Registration is an explicit table (Default), not init() self-registration,
// and resolution is an explicit prefix lookup with no smart fallback — both
// human-decided at planning (Brad, 2026-07-02; recorded in the slice's
// status.json design_decisions). The one-shot utility path
// (model.FromEnv/model.NewClient) is NOT replaced by this package; it remains
// the constructor path for gates/bench.
//
// Placement note (recorded divergence, S04 precedent): the spec's AC-01 names
// the literal file internal/driver/registry.go, but that file would be
// package driver, which must import neither internal/model (ADR-0012,
// enforced by TestNoWireImports) nor internal/driver/inprocess (import
// cycle: inprocess imports driver). Default(cfg) needs both, so the registry
// lives in this subpackage — still under internal/driver/, still covered by
// `go test ./internal/driver/...`. Default is the qualified-name equivalent
// of AC-01's DefaultRegistry: registry.Default(cfg) reads as "the default
// registry" without the package-name stutter.
package registry

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/swornagent/sworn/internal/account"
	"github.com/swornagent/sworn/internal/driver"
	"github.com/swornagent/sworn/internal/driver/inprocess"
	"github.com/swornagent/sworn/internal/model"
)

// Entry is one registered driver: the prefixes it owns plus the environment
// probes enumeration uses. Probes are injected at registration so the
// Registry type itself never touches env or filesystem — unit tests supply
// fakes and nothing can dispatch by construction.
type Entry struct {
	// Driver is the registered driver.
	Driver driver.Driver
	// Prefixes are the canonical model-ID prefixes (without the trailing
	// slash, e.g. "openai") this driver owns.
	Prefixes []string
	// Probe reports availability in this environment WITHOUT dispatching
	// (PATH lookup, key presence, credentials-file read) plus a
	// human-readable detail string. Nil means availability is unknown.
	Probe func() (available bool, detail string)
	// ViaProxy reports whether the given prefix currently resolves through
	// the sworn proxy (per-prefix account.Endpoint check — the same condition
	// model.FromEnv routes on). Nil means never via proxy (e.g. keyless
	// subprocess drivers).
	ViaProxy func(prefix string) bool
}

// Info is one enumeration row: everything `sworn capabilities` renders about
// a registered driver, produced without any model dispatch (AC-05).
type Info struct {
	Name string
	// Prefixes are the canonical prefixes, sorted, without trailing slash.
	Prefixes []string
	// DeprecatedAliases maps deprecated alias prefix -> canonical prefix
	// for aliases resolving to this driver (e.g. openai-responses -> openai).
	DeprecatedAliases map[string]string
	Roles             driver.RoleSet
	Available         bool
	Detail            string
	// ViaProxy lists the prefixes (sorted) that currently resolve through
	// the sworn proxy — the S06b routing made visible (sworn#69).
	ViaProxy []string
}

// Registry is an explicit, ordered prefix -> driver table. The zero value is
// not usable; construct with New (tests) or Default (the compiled-in table).
type Registry struct {
	entries  []Entry
	byPrefix map[string]int    // canonical prefix -> entries index
	aliases  map[string]string // deprecated alias -> canonical prefix
	// Warnf receives deprecation warnings from Resolve. Defaults to
	// os.Stderr when nil; tests inject a capture function.
	Warnf func(format string, args ...any)
}

// New returns an empty registry. Production code uses Default; New exists so
// tests can build registries with fake drivers and probes.
func New() *Registry {
	return &Registry{
		byPrefix: make(map[string]int),
		aliases:  make(map[string]string),
	}
}

// Register adds a driver entry to the table. Registering a prefix twice is a
// programming error in the compiled-in table and panics, mirroring
// internal/command.Register.
func (r *Registry) Register(e Entry) {
	if e.Driver == nil {
		panic("registry: Register called with nil Driver")
	}
	if len(e.Prefixes) == 0 {
		panic(fmt.Sprintf("registry: driver %q registered with no prefixes", e.Driver.Name()))
	}
	idx := len(r.entries)
	for _, p := range e.Prefixes {
		if _, dup := r.byPrefix[p]; dup {
			panic(fmt.Sprintf("registry: prefix %q already registered", p))
		}
		if _, dup := r.aliases[p]; dup {
			panic(fmt.Sprintf("registry: prefix %q already registered as an alias", p))
		}
		r.byPrefix[p] = idx
	}
	r.entries = append(r.entries, e)
}

// RegisterAlias records a deprecated alias for an already-registered
// canonical prefix. Resolving through the alias succeeds with a deprecation
// warning (sworn#31).
func (r *Registry) RegisterAlias(alias, canonical string) {
	if _, ok := r.byPrefix[canonical]; !ok {
		panic(fmt.Sprintf("registry: alias %q targets unregistered prefix %q", alias, canonical))
	}
	if _, dup := r.byPrefix[alias]; dup {
		panic(fmt.Sprintf("registry: alias %q already registered as a prefix", alias))
	}
	if _, dup := r.aliases[alias]; dup {
		panic(fmt.Sprintf("registry: alias %q already registered", alias))
	}
	r.aliases[alias] = canonical
}

// Resolve maps a model ID to its registered driver and fail-fast checks the
// requested role BEFORE any dispatch. There is no fallback to a different
// driver, ever (explicit prefix, no magic — decided 2026-07-02). Errors name
// what IS registered: the unknown-prefix error enumerates every registered
// prefix (AC-02); the role error names the driver, the missing role, and
// which registered drivers DO declare it (AC-03).
func (r *Registry) Resolve(modelID string, role driver.Role) (driver.Driver, error) {
	prefix, _, err := splitModelID(modelID)
	if err != nil {
		return nil, err
	}

	canonical := prefix
	if c, ok := r.aliases[prefix]; ok {
		r.warnf("warning: model prefix %q is deprecated — use %q instead (sworn#31; the alias is kept for one release)\n",
			prefix+"/", c+"/")
		canonical = c
	}

	idx, ok := r.byPrefix[canonical]
	if !ok {
		return nil, fmt.Errorf("registry: no driver for prefix %q — registered prefixes: %s",
			prefix+"/", r.prefixList())
	}

	d := r.entries[idx].Driver
	if !d.Roles().Has(role) {
		return nil, fmt.Errorf("registry: driver %q cannot serve role %q — declared roles: %s; drivers declaring %q: %s",
			d.Name(), role, d.Roles(), role, r.declaring(role))
	}
	return d, nil
}

// Drivers enumerates every registered entry — name, prefixes, deprecated
// aliases, role set, availability, and which prefixes currently route
// through the sworn proxy — without making any model dispatch (AC-05). The
// probes are the only environment access and they are PATH lookups,
// struct-field checks, and a credentials-file read by construction.
func (r *Registry) Drivers() []Info {
	infos := make([]Info, 0, len(r.entries))
	for i, e := range r.entries {
		info := Info{
			Name:     e.Driver.Name(),
			Prefixes: append([]string(nil), e.Prefixes...),
			Roles:    e.Driver.Roles(),
		}
		sort.Strings(info.Prefixes)
		for alias, canonical := range r.aliases {
			if r.byPrefix[canonical] == i {
				if info.DeprecatedAliases == nil {
					info.DeprecatedAliases = make(map[string]string)
				}
				info.DeprecatedAliases[alias] = canonical
			}
		}
		if e.Probe != nil {
			info.Available, info.Detail = e.Probe()
		}
		if e.ViaProxy != nil {
			for _, p := range info.Prefixes {
				if e.ViaProxy(p) {
					info.ViaProxy = append(info.ViaProxy, p)
				}
			}
		}
		infos = append(infos, info)
	}
	return infos
}

// warnf routes a deprecation warning to the injected Warnf, defaulting to
// stderr. The message never contains keys or payload content.
func (r *Registry) warnf(format string, args ...any) {
	if r.Warnf != nil {
		r.Warnf(format, args...)
		return
	}
	fmt.Fprintf(os.Stderr, format, args...)
}

// prefixList renders every registered prefix — canonical and alias — sorted,
// with a trailing slash, aliases marked. Used by the unknown-prefix error so
// "why can't the loop use X" is answered by the error text (AC-02).
func (r *Registry) prefixList() string {
	items := make([]string, 0, len(r.byPrefix)+len(r.aliases))
	for p := range r.byPrefix {
		items = append(items, p+"/")
	}
	for alias, canonical := range r.aliases {
		items = append(items, fmt.Sprintf("%s/ (deprecated alias of %s/)", alias, canonical))
	}
	sort.Strings(items)
	return strings.Join(items, ", ")
}

// declaring names the registered drivers (sorted) whose RoleSet declares
// role, or "(none)" when no driver does — the AC-03 error vocabulary.
func (r *Registry) declaring(role driver.Role) string {
	var names []string
	for _, e := range r.entries {
		if e.Driver.Roles().Has(role) {
			names = append(names, e.Driver.Name())
		}
	}
	if len(names) == 0 {
		return "(none)"
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// splitModelID splits "provider/model", mirroring model.parseModelID's
// wording so both resolution surfaces speak the same error language.
func splitModelID(modelID string) (prefix, name string, err error) {
	idx := strings.IndexByte(modelID, '/')
	if idx < 0 {
		return "", "", fmt.Errorf("registry: invalid model ID %q (want provider/model)", modelID)
	}
	prefix, name = modelID[:idx], modelID[idx+1:]
	if prefix == "" || name == "" {
		return "", "", fmt.Errorf("registry: invalid model ID %q (provider and model required)", modelID)
	}
	return prefix, name, nil
}

// responsesPrefixes and chatPrefixes are the compiled-in prefix table
// (design D2, Type-2 noted default in status.json): openai/ routes to the
// Responses driver (sworn#31), and the chat identity keeps the full
// chat-capable OAI-compat set plus anthropic/ so the loop's reach through
// the registry matches what model.NewClient serves chat-capably today.
// Verify-only providers (google, vertex, bedrock, azure, oci, ollama) are
// deliberately NOT registered — they stay on the one-shot utility path.
var (
	responsesPrefixes = []string{"openai"}
	chatPrefixes      = []string{
		"openai-completions", "deepseek", "groq", "mistral",
		"openrouter", "cloudflare", "github", "anthropic",
	}
)

// Default returns the compiled-in registry (AC-01's DefaultRegistry): the
// four drivers of this release — claude subprocess, codex subprocess,
// in-process Responses, in-process chat/completions — with their prefix
// mappings and real environment probes.
func Default(cfg model.ProviderConfig) *Registry {
	r := New()
	proxy := proxyRouting()

	r.Register(Entry{
		Driver:   driver.NewClaudeDriver(),
		Prefixes: []string{"claude-cli"},
		Probe:    binaryProbe("claude"),
	})
	r.Register(Entry{
		Driver:   driver.NewCodexDriver(),
		Prefixes: []string{"codex"},
		Probe:    binaryProbe("codex"),
	})
	r.Register(Entry{
		Driver:   inprocess.NewOAIResponses(cfg),
		Prefixes: responsesPrefixes,
		Probe:    keyProbe(cfg, responsesPrefixes, proxy),
		ViaProxy: proxy,
	})
	r.Register(Entry{
		Driver:   inprocess.NewOAIChat(cfg),
		Prefixes: chatPrefixes,
		Probe:    keyProbe(cfg, chatPrefixes, proxy),
		ViaProxy: proxy,
	})
	// sworn#31: openai-responses/ stays one release as a deprecated alias of
	// openai/ (both are the Responses API).
	r.RegisterAlias("openai-responses", "openai")
	return r
}

// keyFor maps a registered in-process prefix to its ProviderConfig key. The
// openai trio shares OpenAIKey; every other prefix has a dedicated field.
func keyFor(cfg model.ProviderConfig, prefix string) string {
	switch prefix {
	case "openai", "openai-completions", "openai-responses":
		return cfg.OpenAIKey
	case "deepseek":
		return cfg.DeepSeekKey
	case "groq":
		return cfg.GroqKey
	case "mistral":
		return cfg.MistralKey
	case "openrouter":
		return cfg.OpenRouterKey
	case "cloudflare":
		return cfg.CloudflareKey
	case "github":
		return cfg.GitHubToken
	case "anthropic":
		return cfg.AnthropicKey
	}
	return ""
}

// binaryProbe reports whether the named CLI binary is on PATH. Finding the
// binary proves presence, NOT login state — the detail string says so
// explicitly rather than implying dispatch-readiness.
func binaryProbe(name string) func() (bool, string) {
	return func() (bool, string) {
		if _, err := exec.LookPath(name); err != nil {
			return false, fmt.Sprintf("binary %q not found on PATH", name)
		}
		return true, fmt.Sprintf("binary %q found on PATH; login not probed", name)
	}
}

// keyProbe reports availability for an in-process entry: a prefix is
// available when its API key is present in cfg OR it currently routes
// through the sworn proxy. Struct-field checks plus the proxy predicate
// only — nothing here can dispatch.
func keyProbe(cfg model.ProviderConfig, prefixes []string, viaProxy func(string) bool) func() (bool, string) {
	return func() (bool, string) {
		var withKey, proxied []string
		for _, p := range prefixes {
			if keyFor(cfg, p) != "" {
				withKey = append(withKey, p+"/")
			} else if viaProxy != nil && viaProxy(p) {
				proxied = append(proxied, p+"/")
			}
		}
		sort.Strings(withKey)
		sort.Strings(proxied)
		switch {
		case len(withKey) > 0 && len(proxied) > 0:
			return true, fmt.Sprintf("API keys present: %s; via sworn proxy: %s",
				strings.Join(withKey, ", "), strings.Join(proxied, ", "))
		case len(withKey) > 0:
			return true, "API keys present: " + strings.Join(withKey, ", ")
		case len(proxied) > 0:
			return true, "no API keys; via sworn proxy: " + strings.Join(proxied, ", ")
		default:
			return false, "no API keys present; no sworn proxy login"
		}
	}
}

// proxyRouting returns the per-prefix proxy predicate: true when sworn login
// credentials are present, SWORN_DIRECT is unset, and account.Endpoint
// yields a proxy URL for the prefix — the exact condition model.FromEnv
// routes on, evaluated per prefix (never a blanket login flag). Reads env
// and the credentials file only; cannot dispatch.
func proxyRouting() func(prefix string) bool {
	return func(prefix string) bool {
		if os.Getenv("SWORN_DIRECT") == "1" {
			return false
		}
		creds, err := account.Load(filepath.Dir(account.CredentialsPath()))
		if err != nil || creds == nil || !account.IsLoggedIn(creds) {
			return false
		}
		return account.Endpoint(creds, prefix+"/probe") != ""
	}
}
