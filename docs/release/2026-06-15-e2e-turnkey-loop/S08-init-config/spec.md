---
title: S08-init-config
description: `sworn init` + turnkey zero-config defaults. The only required input is one API key.
---

# Slice: `S08-init-config`

## User outcome

A fresh user runs `sworn init`, sets one API key, and `sworn run` works on sensible
defaults — 100% turnkey to value, no plumbing. This is the mass-market self-serve
requirement. `sworn init` also **adopts the Baton protocol in the target repo**:
it vendors `docs/baton/` and splices the seven-rule fragment into the repo's
`AGENTS.md`, so the per-project adoption step that the bare Baton install leaves
manual (and that a populated global `~/.claude/CLAUDE.md` masks) becomes a single
command. Adoption matters precisely for repos where *other* contributors' agents
read `AGENTS.md` and never see a user's private global instructions.

## Entry point

CLI: `sworn init` (`cmd/sworn/init.go`); config loading used everywhere.

## In scope

- Config loading with precedence (env > file > default); `sworn init` scaffolds a
  config; implementer + verifier model config; BYO-key; a **safe-hosted default**
  placeholder (explicit selection required until the S10 benchmark picks one).
- **Baton adoption:** `sworn init` materialises `docs/baton/` (from the embedded
  protocol, S04) and idempotently splices the seven-rule fragment into the repo's
  `AGENTS.md` under an `## Engineering Process — Baton` section, creating the file
  if absent and leaving an existing section untouched on re-run. Records the
  vendored protocol version.

## Out of scope

- Enterprise config (sovereignty, SSO, tenancy) — post-MVP, high-touch onboarding.

## Planned touchpoints

- `internal/config/`, `internal/adopt/` (AGENTS.md splice + docs/baton/ materialise),
  `cmd/sworn/init.go`, `cmd/sworn/main.go` (dispatch — shared)

## Acceptance checks

- [ ] After `sworn init` + one key, `sworn run` works with defaults (no other setup).
- [ ] Config precedence is env > file > default.
- [ ] A missing key produces a clear, actionable error (not a crash or false PASS).
- [ ] `sworn init` is idempotent.
- [ ] `sworn init` writes `docs/baton/` and an `## Engineering Process — Baton`
      section into `AGENTS.md` (creating `AGENTS.md` if absent); re-running does not
      duplicate the section or clobber an existing one.

## Required tests

- **Unit**: config precedence resolution; idempotent init; missing-key error path.
- **Unit**: adoption splices the fragment into a repo with no `AGENTS.md`, and is a
  no-op (no duplicate section) when the section already exists.

## Risks

- Key leakage to disk/logs — never log keys; document config location.

## Deferrals allowed?

No.
