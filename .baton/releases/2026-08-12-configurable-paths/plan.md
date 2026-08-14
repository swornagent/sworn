```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-12-configurable-paths",
  "revision": 1,
  "previous_plan": null,
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-12-configurable-paths/1",
  "tracks": [
    {
      "id": "T1-paths",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-project-config-and-paths",
          "outcome": "Every host location Sworn reads or writes comes from configuration with a sensible default, so an operator can place records, journals, workspaces, credentials, artefacts and host tools wherever their machine and project require - while the synthetic guest filesystem inside containment stays fixed and unconfigurable.",
          "contract_path": "contracts/2026-08-12-configurable-paths/S1-project-config-and-paths.json",
          "digest": "sha256:5ad7cd8120a8cf8a0867f2a74d1c6ebcff587c85ce1069b0258f25b5ba268763",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/runtime",
            "internal/gitx",
            "internal/driver",
            "cmd/sworn"
          ]
        },
        {
          "id": "S2-sworn-owned-project-surfaces",
          "outcome": "A Sworn project separates what people read from what the engine runs on: reviewable specifications live under docs/sworn/, machine-written authority and run state live under .sworn/, commit messages carry a configured prefix rather than a hardcoded foreign project name, and every release recorded before the move stays readable.",
          "contract_path": "contracts/2026-08-12-configurable-paths/S2-sworn-owned-project-surfaces.json",
          "digest": "sha256:a1dadbd3685e3927a32a5677f091c779e2221c3378b189f5f96441df74dfa8f0",
          "depends_on": [
            "S1-project-config-and-paths"
          ],
          "consumes": [
            "S1-project-config-and-paths"
          ],
          "touchpoints": [
            "internal/baton",
            "internal/gitx",
            "internal/driver",
            "internal/runtime"
          ]
        }
      ]
    }
  ]
}
```

# Goal

Sworn is about to be promoted to v1 and put in front of people who did not
build it. Two things it currently ships would confuse them, and both are
accidents of how it grew rather than decisions anyone made.

The first is naming. A Sworn project acquires a `.baton/` directory it never
asked for, and every control commit in its history reads
`baton(<release>/<slice>): verifier pass` while product commits alongside them
read `sworn(...)`. A new user reasonably concludes they have installed two
things. Baton is the protocol Sworn implements; a user of the product should
not have to learn that to read their own git log.

The second is that host locations are hardcoded. The records root, the journal
root, the workspace factory root, the credentials directory, and the paths of
`git`, the containment binary and other host tools are literals in the source.
A machine that keeps its tools somewhere else - nix, homebrew, any non-Debian
layout - cannot run Sworn without patching it, and an operator has no way to
say where release artefacts should live.

This release makes both configurable, with defaults chosen so that an
unconfigured project behaves sensibly and a promoted v1 introduces no name
belonging to another project.

It also settles where things live, on one axis: **what a person reads, versus
what a person occasionally interrogates.** Authored plans and slice contracts
are read - by reviewers, by anyone asking what a release committed to - and
they belong under `docs/sworn/`. Records, journals and working files are
interrogated during an audit or a debugging session, and they belong under
`.sworn/`. Today that boundary does not exist: the most-read document in a
release, its plan, is an untracked draft, while the machine's frozen copy of
it is the only tracked one.

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-12-configurable-paths/1`. Planning did not approve itself.

Two slices in one serial track. S2 consumes S1 because the records root, the
documents root and the commit prefix are read through the configuration
mechanism S1 introduces; splitting them differently would mean building that
mechanism twice. Scopes are package directories per the revision-5 lesson:
acceptance changes behaviour that existing tests pin, so the pinning tests are
part of each deliverable.

Three properties are load-bearing and stated as constraints rather than left
to judgement:

The guest filesystem inside containment is **not** operator territory. Paths
such as the guest workspace root and the bind and mask targets are constructed
by the engine to make containment work; they are not the user's directories,
and a configuration route to them is an escape vector rather than a
convenience. S1 requires a test proving no configuration input can reach them.

Anything determining **where durable release truth lives** is project-scoped
and read from a committed file. If a records root could be set per user, two
operators of one repository would write receipts to different places and
neither could read the other's release. S1 requires that a user-scoped
override of a project-scoped location is refused by name rather than silently
honoured.

The **containment mask must follow the configured records root**. Today
`.baton` is masked so no model-directed worker can forge its own receipts. A
configurable root with a hardcoded mask would leave a configured root
unprotected, which is the one part of this work with a security consequence.

Wire vocabulary is deliberately untouched. Effect kinds, receipt shapes and
journal schemas keep their current names because they are durable identifiers
inside every journal and receipt already written; renaming them would strand
the history this system exists to make verifiable. The rename here is confined
to what a user sees: directories, and the text of commit messages.

Development targets `refs/heads/release/v1.0.0`. Promotion to `main` remains a
separate human-gated step, and this release is sequenced before it precisely
so that adopters never have to migrate.

This release does not alter trust rules, receipt schemas, role independence, or
the containment boundary for any production binary. It adds no provider and
ports no platform-gated file.
