```baton-plan-v2
{
  "schema_version": "baton.plan/v2",
  "release": "release-id",
  "revision": 1,
  "previous_plan": null,
  "repository": "owner/repository",
  "target_ref": "refs/heads/main",
  "approval_ref": "approval://release-id/1",
  "tracks": [
    {
      "id": "T1",
      "depends_on": [],
      "slices": [
        {
          "id": "S1",
          "outcome": "One observable delivered outcome.",
          "scope": {
            "include": ["src/owned-surface"],
            "exclude": []
          },
          "acceptance": [
            {
              "id": "A1",
              "text": "The approved observable outcome is demonstrated."
            }
          ],
          "checks": ["project-check-command"],
          "constraints": ["Merge only the exact candidate that passes verification."],
          "depends_on": [],
          "consumes": []
        }
      ]
    }
  ]
}
```

# Goal

State the approved release outcome and why it matters.

# Authority

Name the external decision-maker and protected approval reference that binds
these exact bytes.

For revision 1, set `previous_plan` to `null`. Every later revision increments
`revision` and sets `previous_plan` to the exact Git blob object of the prior
bytes at this same repository path.

# Scope

Summarise included and excluded product surfaces without repeating metadata.

# Acceptance

Explain how each acceptance identifier is observable.

# Ordered tracks and slices

Describe why the ordering and track boundaries are safe.

# Dependencies and inputs

Call out dependency edges, consumed slice outputs, shared boundaries, and
ownership assumptions. A revision invalidates only changed contracts and the
actual consumers of changed passed product trees.

# Checks

Describe the required checks. Their normalized result digest belongs in the
candidate and Verifier receipts; raw output may stay in the engine evidence
store.

# Constraints

Record non-negotiable safety, compatibility, and delivery limits.
