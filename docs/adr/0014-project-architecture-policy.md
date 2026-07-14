# ADR 0014: Sworn enforces a project-owned architecture policy

## Status

Accepted (2026-07-14, architecture review #108).

## Context

Sworn implements Baton architecture-rule checks, but the repository did not
provide a project policy. The checker could therefore report PASS after checking
zero rules. A malformed present policy was also indistinguishable from an
unconfigured adopting repository.

Baton's canonical protocol remains embedded in the binary under ADR-0008.
`docs/baton/` is a legacy protocol-copy location and must not become Sworn's new
source of protocol truth. Project architecture constraints are different: they
describe this repository's code boundaries and intentionally change with the
project.

## Decision

Sworn's project-owned architecture policy lives at `docs/architecture.json`.
The architecture gate:

1. reads that path first;
2. accepts `docs/baton/architecture.json` only as a compatibility fallback for
   adopting repositories;
3. rejects malformed, empty, duplicate, or semantically invalid present policy;
   and
4. permits an entirely missing policy for adopting repositories, while Sworn's
   own test suite requires its policy to exist and remain populated.

The initial policy protects boundaries confirmed by the architecture review:
declared slice touchpoints, control adapters, terminal persistence,
process-global mutation, cancellation-aware subprocesses, credential ownership,
provider environment naming, role/model routing, operator command text, remote
payload safety, and large-file growth review.

Each blocking rule must have a deterministic mutation test or proof that inserts
a deliberate violation and observes a non-PASS result through
`sworn lint design`. Rule configuration is executable policy, not explanatory
documentation.

## Consequences

- ADR-0008 is unchanged: protocol prompts and rules stay embedded; no copied
  Baton tree is restored.
- Sworn-specific architecture decisions now have a machine-checkable projection.
- Policy parse or validation failure is an infrastructure error and cannot be
  reported as a clean design verdict.
- Absence remains non-blocking for repositories that have not opted into an
  architecture policy. Products that require one must add a repository-level
  existence test as Sworn does.
- Grep rules examine changed production lines, not all historical code. The
  policy prevents recurrence; existing violations remain review or remediation
  work unless a dedicated full-tree rule owns them.

## Alternatives considered

### Restore `docs/baton/architecture.json`

Rejected. That path is coupled to the legacy copied-protocol layout removed by
ADR-0008 and by `sworn doctor`. It would make project policy look like another
vendored Baton source.

### Embed Sworn's project policy in the binary

Rejected. The binary embeds the protocol it distributes. Repository-specific
constraints must be reviewed and versioned with the code they govern.

### Require a policy in every adopting repository

Rejected for now. Existing users should not have every design lint fail merely
because they have not adopted optional project architecture rules. A future
project configuration may make policy presence an explicit required setting.
