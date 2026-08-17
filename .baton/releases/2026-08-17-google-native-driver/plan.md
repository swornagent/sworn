```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-17-google-native-driver",
  "revision": 2,
  "previous_plan": "9ee5ffa26e789c276a77650320cd03b45bc83dfe",
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-17-google-native-driver/2",
  "tracks": [
    {
      "id": "T1-native",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-gemini-native-adapter",
          "outcome": "Gemini becomes economically real through Google's front door: a native generateContent adapter family whose implicit caching is visible and accounted (77% observed on recorded fixtures), whose thinking level is an operator-chosen profile knob, and whose thought signatures ride as the first-class fields they are - so the roster's fastest implementer stops paying full freight on every resent byte.",
          "contract_path": "contracts/2026-08-17-google-native-driver/rev2/S1-gemini-native-adapter.json",
          "digest": "sha256:aa443f0de81124b64ffed75c0e8b5e3f97ea0096e65cf71f3090a58a267232a8",
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

# Revision 2

Mid-design, the implementer was observed weighing how to encode Sworn's
string-shaped tool results into functionResponse.response, a JSON value.
The seductive answer - parse the string and embed it structured when it
happens to be valid JSON - would make the wire shape depend on content:
receipts would bind bytes the model never saw, and Gemini would diverge
from every other provider on identical tool results. The operator chose
to intervene before a candidate could choose it.

This revision changes one acceptance criterion and adds one constraint:
results ride a fixed single-key envelope {"result": "<the exact string>"}
regardless of content, arguments re-serialise through the engine's
canonical encoder so digested bytes are deterministic, and adversarial
fixture n6 - a tool result whose text is itself valid JSON, recorded
passing through the envelope as text - makes the shortcut mechanically
detectable forever. Scope, checks and every other criterion are
byte-identical to revision 1.

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-17-google-native-driver/2`. Planning did not approve
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
