```baton-plan-v1
{
  "schema_version": "baton.plan/v1",
  "release": "release-id",
  "repository": "owner/repository",
  "target_ref": "refs/heads/main",
  "release_ref": "refs/heads/release-wt/release-id",
  "record_root": ".baton/releases",
  "approval_ref": "approval://release-id/1",
  "tracks": [
    {
      "id": "T1",
      "ref": "refs/heads/track/release-id/T1",
      "depends_on": [],
      "touch_surfaces": ["src"],
      "work": [
        {
          "id": "W1",
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
          "constraints": ["Keep .baton/releases behaviorally inert."],
          "depends_on": []
        }
      ]
    }
  ]
}
```

# Goal

State the approved release outcome and why it matters.

# Authority

Name the external decision-maker and the protected approval reference that will
bind these exact bytes.

# Scope

Summarise included and excluded product surfaces without repeating metadata.

# Acceptance

Explain how each acceptance identifier is observable.

# Ordered tracks and work

Describe why the ordering and track boundaries are safe.

# Dependencies and touch surfaces

Call out dependency edges, shared boundaries, and ownership assumptions.

# Checks

Describe the required checks and where their raw output will be retained.

# Constraints

Record non-negotiable safety, compatibility, and delivery limits.
