# Sworn S0 Baton RC2 asset scope

Date: 2026-07-24
Status: candidate scope approved; publication binding on hold
Sworn design head: `f1d90e902238040eaaf175e82aff69fb3b32f1a4`
Baton candidate commit: `893f6fe8b6a52cebc8e7ccecc745ed5d138f3184`
Baton candidate tree: `8770f15e6f6919dc92458f071205eb7552800d3a`

## Decision

S0 embeds only the Baton bytes that define its runtime-facing identity. The
JavaScript reference, fixtures, and portable-kit runners remain development
and CI evidence. Sworn does not ship a second copy of Baton's implementation
or make a generated manifest a competing runtime authority.

This capture records candidate evidence only. Final generation and admission
remain closed until the annotated `v1.0.0-rc.2` tag and release archive exist.

## Production embed

The compiled inventory is closed to these 11 path-preserving blobs, sorted
lexicographically. Candidate size is 47,374 bytes.

| Path | Candidate SHA-256 |
| --- | --- |
| `VERSION` | `0c654c00f94741d78169de333d4d9e866be0667b41f9c54cafe3c6b700b15a43` |
| `baton/PROTOCOL.md` | `8e6eb570b2eeb27d84b64fb182d71f7591995ba6a1318500769a9db9144eca5a` |
| `conformance/engine-adapter.md` | `dbb3d5c3d22b79a3da4e98fb96f4db1eaa16d2bda04567f4d181bda001705450` |
| `conformance/manifest.json` | `3bf2535cc1e92ac132576dd0c646062b9d33a0ba33201823f1d92409a6387a92` |
| `operations/baton-design-review.md` | `ead3a7d0e22a794ca5430fdbaca5c29f3ae5d5f6fad7c102d1f2bd878f28e356` |
| `operations/baton-implement.md` | `2444bead5b1a32188003ce515ac8862bd04d373b740bd89646a86ac5341c2f88` |
| `operations/baton-merge.md` | `94b8fb6026c903569cd375cafd11d27868759072dde256265556c710387ae62c` |
| `operations/baton-plan.md` | `e5c3ace4177cb10c9b0d3b5e569aa7cbe43bfdb3b7f4a17071a925a5ba3b77d3` |
| `operations/baton-verify.md` | `a6f0e9b9bf95cb59e5030b7f95f72d8d3545b52ef771c7d20e7be44a20e45bed` |
| `reference/driver/contract.md` | `660a1ce7b44cdd150d902fddc80043814b5d6dc4fc28c29a7daed9973abe60bf` |
| `schemas/work-status-v1.json` | `70219641e954afefa35fe20cf702eeabac3ce7c9290d09d5ce29082bf4a497c1` |

Startup recomputes every digest and requires the exact closed inventory. The
release identity separately binds:

- annotated tag object, peeled commit, and peeled tree;
- deterministic published package archive digest;
- five operation-document digests and versions;
- schema and conformance-manifest digests; and
- generated support-package digest admitted from release evidence.

## Development-only closure

Golden-vector generation may execute these exact reference modules, but they
are never embedded in or executed by the production binary:

- `reference/records/actions.mjs`;
- `reference/records/git.mjs`;
- `reference/records/records.mjs`; and
- `reference/records/transition.mjs`.

The portable conformance corpus includes:

- all raw, valid-status, invalid-schema, and invalid-semantic fixtures named by
  `conformance/manifest.json`;
- `conformance/fixtures/driver/**` and
  `reference/driver/fake-driver.mjs`;
- `conformance/fixtures/board/**` and `reference/board/{oracle,terminal,web}.mjs`;
  and
- all five operation documents consumed by the fake driver.

The exact release archive remains authoritative for the Python checker, pinned
requirements, overhead baseline, manifest-named Node suites, helpers,
installers, generated adapters, and generator scripts. CI runs that archive
directly instead of copying its transitive test tree into Sworn.

## Intentional omissions

Production does not embed:

- any `reference/**/*.mjs` implementation;
- fixtures or generated golden vectors;
- Baton tests, Python, installers, adapter generators, generated Skills,
  templates, examples, baselines, or explanatory documents; or
- raw `adapters/generated/generated-manifest.json` bytes.

The generated adapter manifest is release-admission evidence. Sworn validates
it and copies its declared identity into the compiled release identity; it does
not consult the manifest as a mutable runtime authority. Its support-package
digest is not the published archive digest.

## Publication gate

Before S0 may bind or embed these bytes:

1. `v1.0.0-rc.2` must exist as an annotated tag;
2. its tag object, peeled commit, and peeled tree must be recorded;
3. a deterministic package archive and SHA-256 must be published;
4. all 11 production blobs and the development corpus must be revalidated
   against the tagged tree; and
5. the generated adapter support-package digest must match the release
   evidence.

Until all five hold, candidate hashes are informative only and S0 admission
fails closed.
