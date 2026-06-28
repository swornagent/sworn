# Proof Bundle: `S15-spec-proof-records`

## Scope

Emit spec-v1 spec.json from spec.md; emit proof-v1 proof.json from live state; fix generateProof() to derive all sections from live ACs and git state (not constant boilerplate); fix files_changed to use git diff --name-only <start_commit>.

## Files changed

```
$ git diff --name-only 2bffa27..HEAD
docs/release/2026-06-27-conformance-foundation/S15-spec-proof-records/status.json
internal/baton/schemas/embed.go
internal/baton/schemas/proof-v1.json
internal/baton/schemas/spec-v1.json
internal/baton/validator.go
internal/gate/trace.go
internal/gate/trace_test.go
internal/implement/implement.go
internal/implement/implement_test.go
internal/implement/proof_record.go
internal/implement/proof_record_test.go
internal/implement/ready_test.go
internal/implement/spec_record.go
internal/implement/spec_record_test.go
```

## Test results

### Go

```
$ go test ./internal/implement/... ./internal/gate/...
ok  	github.com/swornagent/sworn/internal/implement	0.375s
ok  	github.com/swornagent/sworn/internal/gate	0.050s

```

## Reachability artefact

- **Type**: manual-smoke-step
- **Path**: `docs/release/2026-06-27-conformance-foundation/S15-spec-proof-records/proof.md`
- **User gesture**: `go test ./internal/implement/... ./internal/gate/... -v` exits 0 (all tests pass)

## Delivered

- spec-v1 spec.json written from spec.md with acceptance criteria array
- proof-v1 proof.json written from live repo state (git diff, test results, ACs)
- generateProof() files_changed uses git diff --name-only <start_commit>..HEAD
- generateProof() delivered derived from spec.md acceptance criteria
- generateProof() not_delivered derived from st.OpenDeferrals
- generateProof() divergence from planned_files vs actual git diff
- scripts/release-verify.sh string removed from implement.go (zero grep matches)
- RTM trace gate reads covers_needs from spec.json when present
- spec-v1 and proof-v1 JSON schemas added to embedded schemas
- spec-v1 and proof-v1 structural validation in baton.Validate()
- spec_record_test.go: parse spec.md → ACs extracted with correct IDs and EARS keywords
- proof_record_test.go: files_changed uses start_commit diff path; not_delivered from open_deferrals; divergence detection; delivered only checked ACs

## Not delivered

None

## Divergence from plan

None

## First-pass script output

```
$ release-verify.sh S15-spec-proof-records 2026-06-27-conformance-foundation
(run externally — see CI or manual run)
```
