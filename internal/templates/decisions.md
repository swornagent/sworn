# Decision Registry

Append-only log of every design, architecture, and data decision made across all
releases. The planner checks this registry before asking any question that has
already been answered.

## Entry format

Each entry follows this structure:

```
## <YYYY-MM-DD> — <Short decision title>
- **Type**: design | architecture | data | flow | deviation
- **Release**: <release-name> (slice <slice-id>)
- **Decision**: <one sentence — what was chosen>
- **Rationale**: <why this option over the alternatives>
- **Applies to**: <free text — when future slices should re-use this decision>
- **Overrides**: <link to prior decision if this supersedes one>
```

---

## Example entries

### 2026-01-15 — Use shadcn/ui as component library

- **Type**: design
- **Release**: 2026-01-10-initial-scaffold (slice S05-design-system)
- **Decision**: Adopt shadcn/ui with Radix primitives as the project's component library
- **Rationale**: Headless Radix primitives give full accessibility coverage (WCAG 2.1 AA by
  construction); shadcn's copy-paste model avoids a transitive dependency and lets us own the
  source; Tailwind token integration is native. Alternatives considered: MUI (heavy, hard to
  match brand), Headless UI (fewer primitives), custom (too expensive for initial velocity)
- **Applies to**: All UI components across the project. Any new UI surface must use
  shadcn/ui + Radix primitives unless the required component does not exist there.
- **Overrides**: none

### 2026-02-28 — AES-256-GCM for PII encryption at rest

- **Type**: data
- **Release**: 2026-02-20-encryption-layer (slice S03-pii-encrypt)
- **Decision**: Encrypt all PII fields (name, email, tax identifiers, dollars) at rest with
  AES-256-GCM, keyed per tenant, with key material stored in a KMS external to the database
- **Rationale**: GCM provides authenticated encryption (confidentiality + integrity); per-tenant
  keys limit blast radius of a single key compromise; KMS separation means database dump alone
  reveals nothing. AES-256 chosen over AES-128 for compliance headroom (FIPS 140-2 Level 3).
  ChaCha20-Poly1305 considered but not chosen — AES has broader hardware acceleration coverage
  on our target cloud instances.
- **Applies to**: Any new field containing personally identifiable or financial data. Existing
  plaintext fields must migrate per the data migration policy.
- **Overrides**: none

### 2026-03-12 — Monorepo with shared packages, not separate repos

- **Type**: architecture
- **Release**: 2026-03-05-repo-reorg (slice S01-monorepo-structure)
- **Decision**: Single monorepo with `packages/` for shared code (types, utils, config)
  rather than publishing internal packages to a private registry
- **Rationale**: Shared-schema drift is the #1 source of production incidents in multi-repo
  setups; a monorepo makes schema changes atomic across consumers. CI can still partition
  builds by package. Publishing overhead (version bumps, install lag) eliminated.
  Alternatives considered: multi-repo with git submodules (chore overhead too high for team
  size), private npm/Go registry (operational cost outweighs benefit at <20 packages).
- **Applies to**: All shared library code. Internal-only; external consumers get published
  packages from CI.
- **Overrides**: none