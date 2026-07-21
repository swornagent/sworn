# Baton Assurance 1.0

The five core principles apply to every delivery. Assurance packs add evidence
or review only when the risk justifies it; they do not create alternate loops.

## Profiles

### Standard

Every work unit starts here. Standard requires:

- schema-valid plan, submission, and verdict records;
- exact candidate and actual-path capture;
- relevant deterministic project checks;
- acceptance-linked evidence with an honest test boundary;
- a fresh independent verifier; and
- compare-and-swap integration or an honest stop at `verified` or
  `ready_to_integrate` when integration is not yet authorized or unlatched.

No Captain, design session, model-based gate cascade, journey document, RTM, or
maintainability LLM review is universally required.

### Assured

Assured adds one or more named packs. A plan may request it directly. Project
policy or the engine may upgrade Standard work when actual changes match a risk
trigger. A verifier may also request an upgrade. No actor may downgrade it
without new authority.

The engine SHOULD apply deterministic triggers before constructing the
submission. If the verifier discovers a missing pack, it returns `SPEC_BLOCK`.
The engine or authorizer must then issue an approved plan revision that names the
new pack and policy digest before gathering evidence and creating a new
submission for the unchanged candidate. Standing authority may make that
revision automatic, but it still creates a new plan digest and authority
receipt. The old submission is never relabeled.

## Policy registry

`assurance-policy-v1` is a strict I-JSON registry, not a delivery record. Its
digest is RFC 8785 canonical JSON SHA-256 over the whole registry. Its non-empty
`checks` array binds each Standard check ID to one content-addressed
`application/json` definition. Every baseline definition MUST resolve, match its
raw-byte digest, and parse strictly before builder dispatch. The definition owns
the check's stable semantics; an ID alone cannot silently acquire a different
command or meaning.

The registry also maps unique versioned pack IDs, such as `security@1`, to
content-addressed `application/json` definitions. Only packs selected by the work
contract must resolve for that work. Every selected definition must match its
raw-byte digest and parse strictly; unknown, ambiguous, or unavailable selected
packs fail closed. An unavailable unselected pack does not block Standard work.
The submission repeats the policy locator and digest for the verifier.

## Pack contract

A pack definition is project or engine policy. Baton leaves its inner schema
and procedure to that engine, but it should cover:

1. **name and version** — stable identity included in the policy digest;
2. **trigger** — deterministic facts such as paths, dependency changes, data
   classifications, or declared effects;
3. **required evidence** — checks, observations, or attestations that must be in
   the submission;
4. **verifier questions** — additional claims the fresh verifier must assess;
5. **authority gate**, when needed — the decision or human action that cannot be
   delegated implicitly.

Pack procedures belong in the engine or project policy, not in Baton's universal
role instructions. They can add checks or observations but cannot enlarge the
approved plan's authority.

## Recommended packs

Baton standardizes no mandatory catalogue, but common packs include:

- `security@1` — authentication, authorization, secrets, trust boundaries;
- `privacy@1` — personal or sensitive data collection, access, retention;
- `money@1` — calculations or effects that move or represent money;
- `data-change@1` — migrations, destructive writes, compatibility, rollback;
- `public-contract@1` — APIs, schemas, stored formats, backwards compatibility;
- `production@1` — deployment, infrastructure, process-global state, rollback;
- `regulated@1` — legal, compliance, accessibility, or jurisdictional evidence;
- `design-decision@1` — irreversible or system-shaping choices needing explicit
  authorizer review; and
- `system-journey@1` — user-critical behavior requiring assembled or live proof.

Names are illustrative and recognized only when registered by the active
policy. A project can define fewer, broader packs. Active pack versions and the
policy digest are always bound into the submission.

## Admission rule

A proposed universal requirement belongs in Baton Core only if removing it
would break trust for nearly every delivery. Otherwise it belongs in:

- a deterministic engine invariant;
- a project check;
- an assurance pack; or
- nowhere, if retry and fresh verification already make the failure cheap.

Incident history is evidence for improving a trigger or invariant. It is not by
itself a reason to add prose that every future model must read.
