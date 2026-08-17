```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-17-google-native-driver",
  "revision": 1,
  "previous_plan": null,
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-17-google-native-driver/1",
  "tracks": [
    {
      "id": "T1-native",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-gemini-native-adapter",
          "outcome": "Gemini becomes economically real through Google's front door: a native generateContent adapter family whose implicit caching is visible and accounted (77% observed on recorded fixtures), whose thinking level is an operator-chosen profile knob, and whose thought signatures ride as the first-class fields they are - so the roster's fastest implementer stops paying full freight on every resent byte.",
          "contract_path": "contracts/2026-08-17-google-native-driver/S1-gemini-native-adapter.json",
          "digest": "sha256:d654bbd0ef409e451230d8d0c101b76f6c6cfd8269e1f290093448bfb4f9dc1a",
          "depends_on": [],
          "consumes": [],
          "touchpoints": ["internal/driver"]
        }
      ]
    }
  ]
}
```

# Goal

Gemini 3.7 Flash entered the roster through Google's OpenAI-compatibility
side door, and the meter told the story: ~2.2M tokens per minute against a
3M ceiling, every turn re-paying the full context, two provider-limit
parks in one evening. The measurements are unambiguous - identical
realistic bodies show no cache reporting at all on the compat surface and
a 77% reported implicit-cache hit on the native surface. The model was
never the problem; the door was.

This release builds the native generateContent adapter family: visible
caching in the usage receipts, thinking level as the operator's knob
(ruled HIGH for implementation work), thought signatures as the
first-class fields the native surface makes them, tools and multi-call
turns in the native shape, and the same closed-allowlist strictness as
every other surface. The compat adapter stays certified as the fallback.

The paused loop-economics release resumes on this driver: the fast
implementer returns paying cache prices, and S4's batching then cuts the
remaining bill.

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-17-google-native-driver/1`. Planning did not approve
itself. One slice, scope internal/driver per the
scope-from-acceptance-evidence rule (sworn#199): adapter, dialect,
fixtures and certification tests all land in that package.

Roles: DeepSeek implements and grok-4.6 verifies - the subject vendor
holds neither role, which is the independence discipline at its cleanest.

Sequencing: loop-economics is paused mid-S1 with receipts safe on its
refs; this release merges first, moves the target head, and economics
re-records at resume - the known, cheap dance. This release does not
alter trust rules, receipt schemas, budgets, or any other adapter. It
renames nothing on the wire.
