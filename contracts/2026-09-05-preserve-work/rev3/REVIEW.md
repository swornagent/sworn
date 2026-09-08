# Recovery proposal validation

Status: proposed, not externally approved or natively recorded.
Plan SHA-256: 5bde61401734129a75f4eaa8f74306098f0cdcfe8a903a608a8cafe5d368203b.

Repair source is committed locally at
3a0f20fede0a021d5f8b03ef6ca6054760b6a9c1 on fix/preservation-recovery-feedback.
It is not merged to the release or main and is not a Sworn Verifier PASS.
The compressed recovery input's product-only delta matches that commit
exactly against 68c4deefcd7a6eac8b91ea346ae623bfae4373f7:
SHA-256 eea487af37b7c873204acdbed76174681023d5e447c5b9365528f7ec78b4d880,
130353 decoded bytes, 25 paths. CI/AGENTS deadline changes are outside the
patch and are prepared separately in this proposal's base.

Completed operator validation on the repaired Go source:

- Full product/tool suite PASS; runtime 360.444s.
- Full unfiltered serial E2E suite PASS, 2314.061s (38m34s), 45m deadline.
- Full product/tool race suite PASS; runtime 654.000s (10m54s), 20m deadline.
- Vet, asserted empty gofmt listing, git diff whitespace check and
  go mod tidy -diff PASS.
- Linux non-CGO trimmed operator build PASS; Darwin arm64 cross-build PASS.
- All four exact profile/model doctor checks PASS with the repaired binary.
  These are readiness checks, not a new paid live certification.

The original 35m E2E and implicit 10m race commands both hit their package-wide
alarms. Those failed runs remain recorded as failures. No individual test
failure or data-race warning preceded either alarm. Tests/assertions were not
removed; corrected harness budgets let the complete suites finish.

Native plan pin is idempotent; native scope lint PASSes all four slices. All
acceptance criteria are unchanged. S2-S4 differ only in the E2E/race deadline
strings in checks and host_checks. S1 additionally binds the new retained
input and repair constraints. Decoding and git apply --check succeed against
this planning checkout's compatible prepared product base.

Before native recording: obtain actual external approval of the exact plan,
commit the preparation files and bind that exact target/contract-tree commit;
repeat applicability against it. Existing approved release target remains
74241e177d96bc158ab40c49d468fd27fcf5b86f, and the old run remains parked.
Operator test evidence does not substitute for exact-candidate native checks
or independent verification of every acceptance boundary.
