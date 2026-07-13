// Package schemas embeds canonical Baton JSON Schemas into the binary.
// Schema files are validated at build time via //go:embed — a missing
// or unreadable schema file is a compile error, not a runtime surprise.
//
// Stdlib only — zero runtime dependencies.
package schemas

import _ "embed"

// SliceStatusV1 is the canonical slice-status-v1.json schema, embedded
// at build time. It validates every status.json written by the Sworn
// orchestrator.
//
//go:embed slice-status-v1.json
var SliceStatusV1 []byte

// BoardV1 is the canonical board-v1.json schema, embedded at build time.
// It validates every board.json written by the board package.
//
//go:embed board-v1.json
var BoardV1 []byte

// SpecV1 is the canonical spec-v1.json schema, embedded at build time.
// It validates every spec.json written by the implementer.
//
//go:embed spec-v1.json
var SpecV1 []byte

// ProofV1 is the canonical proof-v1.json schema, embedded at build time.
// It validates every proof.json written by the implementer.
//
//go:embed proof-v1.json
var ProofV1 []byte

// JourneysV1 is the canonical journeys-v1.json schema, embedded at build time.
// It validates every journeys.json written by the journey package.
//
//go:embed journeys-v1.json
var JourneysV1 []byte

// AttestationsV1 is the canonical attestations-v1.json schema, embedded at
// build time. It validates every attestations.json written by the journey
// package.
//
//go:embed attestations-v1.json
var AttestationsV1 []byte

// VerifierVerdictV1 is the canonical verifier-verdict-v1.json schema, embedded
// at build time. It validates the Rule-7 verifier's structured-output verdict
// (ADR-0011 authoring path) — the schema-constrained replacement for the prose
// HasPrefix scrape that parsed the verdict out of model free text.
//
//go:embed verifier-verdict-v1.json
var VerifierVerdictV1 []byte

// ContractsV1 is the canonical contracts-v1.json schema (baton v0.10.0),
// embedded at build time. VENDORED-ADVISORY: it is stored and version-declared
// so `sworn doctor` can report it, but sworn does NOT yet grade against it —
// the `sworn lint contracts` grader is the follow-on contract-edge-gates
// release. It is byte-identical to the published $id at the pinned tag; do not
// fork the shape under the same $id.
//
//go:embed contracts-v1.json
var ContractsV1 []byte

// AssemblyProofV1 is the canonical assembly-proof-v1.json schema (baton
// v0.10.0), embedded at build time. VENDORED-ADVISORY: stored and
// version-declared but not yet graded — the `sworn assemble` grader that
// emits/reads this proof is the follow-on release. Byte-identical to the
// published $id at the pinned tag; do not fork the shape under the same $id.
//
//go:embed assembly-proof-v1.json
var AssemblyProofV1 []byte

// CapabilityPolicyV1 is the canonical capability-policy-v1.json schema (baton
// v0.11.0), embedded at build time. VENDORED-ADVISORY: stored and
// version-declared but not yet graded — the capability-based eligibility gate
// (role.requires ∩ registry.provides; ADR-0013) is the follow-on release.
// Byte-identical to the published $id at the pinned tag; do not fork the shape
// under the same $id.
//
//go:embed capability-policy-v1.json
var CapabilityPolicyV1 []byte

// LLMCheckReportV1 is the canonical llm-check-report-v1.json schema (baton
// v0.12.0), embedded at build time. It validates the report returned by each of
// the six deterministic LLM checks.
//
// GRADED, not advisory: the schema is the fail-closed half of the security
// fix in sworn#103. Grading splits `severity` (impact) from `blocking`
// (disposition) and makes the verdict DERIVED — FAIL iff at least one finding is
// blocking — enforced in both directions, so a PASS carrying a blocking finding
// is schema-invalid. That is the exact payload (a critical finding beside a
// self-declared PASS) that used to pass the security gate green.
//
//go:embed llm-check-report-v1.json
var LLMCheckReportV1 []byte

// ProjectContextV1 is the canonical project-context-v1.json schema (baton
// v0.13.0), embedded at build time. It validates the adopting project's declared
// identity and stakes — the record read from .sworn/project.json to fill the
// {{project_context}} and {{project_stakes}} substitutions in every LLM check.
//
// GRADED: internal/project validates the record against this schema and fails
// closed on violation. The stakes half is load-bearing — at high stakes a `medium`
// security finding blocks instead of advising — so a malformed record must not be
// silently treated as "no stakes".
//
//go:embed project-context-v1.json
var ProjectContextV1 []byte

// SchemaMap maps a short schema name (e.g. "slice-status-v1") to its
// embedded bytes. Callers use this to look up the schema by the name
// they store in the $schema field.
var SchemaMap = map[string][]byte{
	"slice-status-v1":      SliceStatusV1,
	"board-v1":             BoardV1,
	"spec-v1":              SpecV1,
	"proof-v1":             ProofV1,
	"journeys-v1":          JourneysV1,
	"attestations-v1":      AttestationsV1,
	"verifier-verdict-v1":  VerifierVerdictV1,
	"contracts-v1":         ContractsV1,
	"assembly-proof-v1":    AssemblyProofV1,
	"capability-policy-v1": CapabilityPolicyV1,
	"llm-check-report-v1":  LLMCheckReportV1,
	"project-context-v1":   ProjectContextV1,
}
