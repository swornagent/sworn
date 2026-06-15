# ADR 0001 — One binary, embedded protocol, package-manager distribution

Status: accepted (2026-06-15)

## Context

SwornAgent (product) is built on Baton (open protocol). Earlier framing risked a
bad developer experience: "git clone baton + install swornagent + wire them"
(up-to-verification = baton, verification = swornagent). For the "download this
and get full E2E automated development" promise, that fragmentation is fatal.
Separately, the bash TUI (`coach top`) has hit its limits.

## Decision

1. **One binary, subcommands** — not separate binaries. `swornagent verify`
   (gate, now), `swornagent top` (TUI), later `swornagent run` / `plan` /
   `implement` / `merge`. Pattern: git / docker / kubectl / gh. One install, one
   version, one update.

2. **TUI in Go (Bubble Tea + Lip Gloss + Bubbles)** as the `swornagent top`
   subcommand — replaces the bash `coach top`.

3. **Conceptual separation ≠ distribution separation** (the OCI/Docker model):
   - **Baton** = the open *specification* + reference rule/prompt content. Lives
     in its own repo for the ecosystem to read/implement ("govern any agent").
     Not a thing an end user installs-and-wires.
   - **SwornAgent** = the installable product that **embeds** the Baton protocol
     (rules, verifier prompt, verdict contract, phase/gate model) via `go:embed`
     at build time.
   - The developer installs ONE thing — `swornagent` — protocol baked in. Two
     repos, one install.

4. **Package-manager distribution from the start** — Homebrew, `go install`,
   `npm` wrapper, container (GHCR), and the GitHub Action all wrap the SAME one
   binary. The git/curl bash install is the legacy + contributor path, not the
   product DX.

## Consequences

- The bash reference loop (`coach-loop`, `captain-*`) is a **bridge** reference
  implementation (ships sooner); the destination is `swornagent run` (Go, embedded).
  The wedge (`swornagent verify`) ships first regardless.
- "Package it right from the start" → favours investing in `swornagent run` sooner
  rather than shipping a franken-bash reference impl users must later migrate off.
- Moat preserved: Baton-the-spec stays a separate open standard; embedding a copy
  in the product is implementation, not coupling (Docker embedding OCI doesn't make
  OCI less a standard).

## End-state DX

`brew install swornagent` → `swornagent run` → full plan→implement→verify→merge,
protocol embedded, no wiring.
