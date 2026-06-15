---
title: S08-init-config
description: `sworn init` + turnkey zero-config defaults. The only required input is one API key.
---

# Slice: `S08-init-config`

## User outcome

A fresh user runs `sworn init`, sets one API key, and `sworn run` works on sensible
defaults — 100% turnkey to value, no plumbing. This is the mass-market self-serve
requirement.

## Entry point

CLI: `sworn init` (`cmd/sworn/init.go`); config loading used everywhere.

## In scope

- Config loading with precedence (env > file > default); `sworn init` scaffolds a
  config; implementer + verifier model config; BYO-key; a **safe-hosted default**
  placeholder (explicit selection required until the S10 benchmark picks one).

## Out of scope

- Enterprise config (sovereignty, SSO, tenancy) — post-MVP, high-touch onboarding.

## Planned touchpoints

- `internal/config/`, `cmd/sworn/init.go`, `cmd/sworn/main.go` (dispatch — shared)

## Acceptance checks

- [ ] After `sworn init` + one key, `sworn run` works with defaults (no other setup).
- [ ] Config precedence is env > file > default.
- [ ] A missing key produces a clear, actionable error (not a crash or false PASS).
- [ ] `sworn init` is idempotent.

## Required tests

- **Unit**: config precedence resolution; idempotent init; missing-key error path.

## Risks

- Key leakage to disk/logs — never log keys; document config location.

## Deferrals allowed?

No.
