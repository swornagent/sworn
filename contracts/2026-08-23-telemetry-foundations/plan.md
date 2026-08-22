```sworn-release-manifest-v1
{
  "schema_version": "sworn.release-manifest/v1",
  "release": "2026-08-23-telemetry-foundations",
  "revision": 3,
  "previous_plan": "cfdd148ba4154f93681a93ec16bb68342c146fd0",
  "repository": "sworn",
  "target_ref": "refs/heads/release/v1.0.0",
  "approval_ref": "operator://2026-08-23-telemetry-foundations/3",
  "tracks": [
    {
      "id": "T1-telemetry",
      "depends_on": [],
      "slices": [
        {
          "id": "S1-usage-truth",
          "outcome": "The engine's own journals answer the questions the operator today answers with provider consoles and hand probes: every dispatch on a certified surface lands a usage receipt with the full token split the wire actually carried - input, output, cache read, cache write, reasoning - plus wall-clock duration, and an attempt that genuinely cannot report says so loudly with the surface named instead of defaulting silent. The eval summary stops rendering a partial sum as a total: coverage rides in-band with every aggregate, and turn economics - turns, tool calls, call mix per role - become first-class eval facts.",
          "contract_path": "contracts/2026-08-23-telemetry-foundations/rev3/S1-usage-truth.json",
          "digest": "sha256:d4f0ebfe05c22bec48283300c7a7565251b3e1359fc97f24feaba09a52bc58c4",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver",
            "internal/journal",
            "internal/observe",
            "internal/runtime",
            "test/e2e"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/driver, internal/observe, and internal/runtime by import only; no cmd test pins usage-receipt or eval-record content, and the CLI surface is untouched by this slice"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/driver and internal/runtime by import only; no cockpit test pins token capture or eval aggregation, and presentation is untouched by this slice"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/driver (via internal/cockpit) and internal/runtime by import only; no tui test pins usage or eval content"
            }
          ]
        },
        {
          "id": "S2-genai-spans",
          "outcome": "Any OTel backend renders sworn's runs as native LLM traffic: each dispatch exports a span carrying the GenAI semantic conventions - the real certified model id, the token split including cache and reasoning components, and wall-clock duration - with sworn's own facts riding as sworn.* attributes, and all spans of one run sharing one trace instead of randomized per-record identities.",
          "contract_path": "contracts/2026-08-23-telemetry-foundations/S2-genai-spans.json",
          "digest": "sha256:65165a6db3fd7c8477a5541df43a5123c6664ab70ce7c9d96ffccd0e9df6a432",
          "depends_on": [
            "S1-usage-truth"
          ],
          "consumes": [],
          "touchpoints": [
            "internal/observe"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/observe by import only; no cmd test pins span vocabulary, and the serve wiring is untouched by this slice"
            }
          ]
        },
        {
          "id": "S3-runside-export",
          "outcome": "The run owns its telemetry: the engine exports its own OTel from the run process, so a plain sworn run lands in the operator's backend with no cockpit required - through two independently configured channels, the operator's private stream and an opt-in share stream whose versioned schema allowlist makes exporting a prompt-shaped attribute structurally impossible - and sworn init walks the operator through the telemetry step instead of requiring source-reading.",
          "contract_path": "contracts/2026-08-23-telemetry-foundations/S3-runside-export.json",
          "digest": "sha256:c137efd42a95837403537d64eeb91936f074789500b200ab0f6ad7bb7a2785b6",
          "depends_on": [
            "S2-genai-spans"
          ],
          "consumes": [],
          "touchpoints": [
            "cmd/sworn",
            "internal/observe",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/runtime by import only; the cockpit's view surfaces are unchanged - serve stops being the export path but its projection behavior is untouched"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/runtime (via internal/cockpit and directly) by import only; no tui test pins telemetry export"
            }
          ]
        },
        {
          "id": "S4-degradation-truth",
          "outcome": "The degradation budget measures degradation relative to what the adapter can actually do, and a degradation park names itself everywhere it surfaces: routine transport weather on an adapter that rehydrates by design no longer walks a long run into a park, while real repeated context loss still does - and when it does, status, board, and webhook name the cause, the count, the budget, and the manifest knob that unblocks it, instead of a flat 'parked' dressed up as a failed work item.",
          "contract_path": "contracts/2026-08-23-telemetry-foundations/S4-degradation-truth.json",
          "digest": "sha256:5703542f0bd20b0603599e65ab2bb428487d8aaa781b6a59dc11337621b3a07c",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/cockpit",
            "internal/driver",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/driver, internal/runtime, and internal/cockpit by import only; no cmd test pins degradation counting or park presentation"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/runtime and internal/driver via internal/cockpit by import only; the observe continuation aggregate is constrained unchanged by this slice"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/cockpit, internal/driver, and internal/runtime by import only; the tui renders the cockpit's park presentation and pins none of its wording"
            }
          ]
        },
        {
          "id": "S5-provider-limit-evidence",
          "outcome": "A provider limit is diagnosable from sworn's own surfaces in seconds instead of hours: the provider's own words survive into the durable failure record and the live stream at least once per dispatch, and a hard wall - a monthly spend cap, a no-window quota - fails the dispatch immediately with that message instead of politely burning the paced-retry budget into it for hours.",
          "contract_path": "contracts/2026-08-23-telemetry-foundations/S5-provider-limit-evidence.json",
          "digest": "sha256:8cfb6a27266b9da8d955eb80f51eea6fc1fad25afa9319e765c0a2bdab70689a",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/driver and internal/runtime by import only; no cmd test pins transport error content"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/driver and internal/runtime by import only; no cockpit test pins provider error detail"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/runtime and internal/driver via internal/cockpit by import only; no observe test pins transport error content"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/driver (via internal/cockpit) and internal/runtime by import only; no tui test pins provider error detail"
            }
          ]
        },
        {
          "id": "S6-failed-observation-persistence",
          "outcome": "A failed dispatch leaves evidence, not just a fingerprint: the observation a failed attempt was digested from is durably readable - its transport status, duration, diagnostic code, terminal events, and usage - so diagnosing a runner_error means reading the journal, never brute-forcing a digest hunt against bytes the engine threw away.",
          "contract_path": "contracts/2026-08-23-telemetry-foundations/S6-failed-observation-persistence.json",
          "digest": "sha256:7e8b063fb8f43a5c849a4fe45379520fe44278967b747017d36b6041368f28e5",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/journal",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/journal and internal/runtime by import only; no cmd test pins failed-attempt observation storage"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/journal and internal/runtime by import only; no cockpit test pins attempt observation storage"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/journal and internal/runtime via internal/cockpit by import only; eval reads the usage receipt, whose shape this slice does not touch"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/journal and internal/runtime by import only; no tui test pins attempt observation storage"
            }
          ]
        },
        {
          "id": "S7-implementer-stream",
          "outcome": "The implementer stops being the only unobservable role: google-native dispatches stream, their deltas render live exactly as deepseek and qwen deliberation already does, and thought summaries can be requested - so a forty-minute gemini build shows its work while it happens instead of freezing the log until completion, and the smart-observer doctrine gets its oversight input for the role that writes the code.",
          "contract_path": "contracts/2026-08-23-telemetry-foundations/S7-implementer-stream.json",
          "digest": "sha256:ea0c5f2895f6fcb0b4571e990100d8755c620f6eecff3328438b1327c609b5a4",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/driver by import only; no cmd test pins the gemini transport mode"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/driver by import only; no cockpit test pins adapter streaming"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no observe test pins adapter streaming"
            },
            {
              "package": "internal/runtime",
              "reason": "its tests reach internal/driver by import only; dispatch semantics are constrained unchanged - streamed and unstreamed dispatches produce equivalent receipts by A3"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/driver via internal/cockpit by import only; no tui test pins adapter streaming"
            }
          ]
        },
        {
          "id": "S8-tool-result-observation",
          "outcome": "What a worker sees becomes observable: every tool result that crosses back into the model also lands in the run's durable event stream - byte-identical to what the model received up to an honest bound, carrying full identity - so an operator, captain, or future orchestrator can act on evidence like 'the suite returned ISOLATION_UNAVAILABLE 155 times' instead of watching a worker react to invisible facts, and hand-reading raw transcripts stops being the only diagnostic path.",
          "contract_path": "contracts/2026-08-23-telemetry-foundations/S8-tool-result-observation.json",
          "digest": "sha256:f5ecfbe8ddf0ade87cb02047c4bac9511960714691a80c9631700e0becf6c845",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/driver",
            "internal/journal",
            "internal/runtime"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/driver, internal/journal, and internal/runtime by import only; no cmd test pins tool-result events"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/driver, internal/journal, and internal/runtime by import only; the SSE invalidate-tick contract is constrained untouched and no cockpit test pins the new event kind"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/journal and internal/runtime via internal/cockpit by import only; the eval observer ignores the new event kind by construction"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/driver, internal/journal, and internal/runtime by import only; presentation of the new events is out of scope"
            }
          ]
        },
        {
          "id": "S9-dispatch-responsibility",
          "outcome": "Dispatch events name what was dispatched again: the association body for the dispatch event family carries the responsibility alongside effect, work, and slice - restoring the additivity operator-surface S5 promised and did not keep - and the conformance suite reads it directly instead of joining through sealed submissions to learn what the event stream used to say.",
          "contract_path": "contracts/2026-08-23-telemetry-foundations/S9-dispatch-responsibility.json",
          "digest": "sha256:0a794574498afb4e8f3762d3e07117b0acd5b5b2d25493b8d89fd999530e2e82",
          "depends_on": [],
          "consumes": [],
          "touchpoints": [
            "internal/runtime",
            "test/e2e"
          ],
          "waivers": [
            {
              "package": "cmd/sworn",
              "reason": "its tests reach internal/runtime by import only; no cmd test pins dispatch event bodies"
            },
            {
              "package": "internal/cockpit",
              "reason": "its tests reach internal/runtime by import only; the webhook and projector classify by kind and offset, and body enrichment is additive beneath them"
            },
            {
              "package": "internal/observe",
              "reason": "its tests reach internal/runtime via internal/cockpit by import only; the continuation aggregate parses kinds, never dispatch bodies"
            },
            {
              "package": "internal/tui",
              "reason": "its tests reach internal/runtime by import only; no tui test pins dispatch event bodies"
            }
          ]
        }
      ]
    }
  ]
}
```

# Why

Telemetry not captured is lost forever - that is R2's whole urgency. The
eval capture that is supposed to be the data moat has been recording
nulls while the drivers demonstrably held the numbers (sworn#209), and
every operational diagnosis of the last fortnight came from outside
sworn's own surfaces: the Google monthly spend cap was found by hand-
curling the raw 429 body the engine discarded - twice in one week
(sworn#227); the sonnet verifier's duplicated runner_error cost a
brute-force digest hunt because only an observation's sha256 survives a
failed attempt; a 40-minute gemini build showed nothing but lease
heartbeats (sworn#225); and 798 tool calls in a measured run produced
zero observable results (sworn#195). Meanwhile export exists only under
sworn serve, so a plain run emits nothing at all, and what it would
emit carries no model id, no GenAI conventions, and no run-level trace.

This release makes the run own its facts: capture them truthfully (S1),
speak them in the industry vocabulary (S2), export them from the run
itself through a private channel and a structurally-allowlisted share
channel (S3), stop miscounting routine transport weather as degradation
while making real parks name themselves (S4), keep the provider's own
words when a limit hits and refuse to pace into hard walls (S5),
persist the evidence of failed attempts (S6), stream the implementer's
work live (S7), journal what workers see (S8), and restore the
dispatched responsibility to dispatch events (S9).

# What is being pinned

Standing rulings this plan encodes:

1. Export moves RUN-SIDE (Brad 2026-08-20): the engine emits its own
   OTel because the run owns its facts; serve becomes purely a view,
   never the export path (S3).
2. Two channels, one wire format: private (operator's own, verbose)
   and share (opt-in, defaults to the project gateway, overridable);
   the share schema allowlist is enforced in-engine, making the
   privacy promise structural (S3).
3. Spans adopt OTel GenAI semantic conventions; sworn facts ride as
   sworn.* attributes; verdict-to-score mapping is backend-side - the
   engine stays vendor-neutral (S2).
4. Telemetry must stay non-interfering: the existing conformance
   evidence that telemetry cannot affect delivery keeps holding with
   run-side export in play (S3-A4).
5. Degradation is measured relative to declared adapter capability -
   absence of a thing the adapter never had is not degradation - and
   park surfaces name the budget, the count, and the manifest knob
   (S4, ruling sought on sworn#227's direction and taken here).
6. Provider limit evidence is kept, bounded: the "never provider
   content" posture on retry metadata is overruled for bounded error
   messages, and hard exhaustion is distinguished from window
   exhaustion (S5, the sworn#227 postscript asks).
7. Failed attempts persist their digested observation bytes - the
   digest stays the identity, the bytes become readable (S6).
8. Tool results are observable at one canonical seam, bounded
   honestly, redacted at emit, identity-carrying (S8, sworn#195).
9. APPROVAL OF S9 IS THE sworn#229 RULING: dispatch event bodies
   restore responsibility additively. Strike S9 before approval to
   keep that ruling open instead.

# Authority

To be approved by the human operator against these exact bytes under
`operator://2026-08-23-telemetry-foundations/1`. Planning did not
approve itself.

One track, nine slices, one mechanism each per the planner doctrine:
capture truth, span vocabulary, and export topology are separate seams
(S1-S3, chained because each projects the previous); degradation
counting, provider-limit evidence, and failed-observation persistence
are three distinct failure-surface mechanisms (S4-S6); adapter
streaming and tool-result observation are two distinct visibility
seams - one presentation-only, one durable (S7-S8); the dispatch-body
restoration is a rider whose approval doubles as the sworn#229 ruling
(S9). The verifier judges each slice by its worker-runnable checks and
evidence anchors; e2e conformance remains declared CI evidence
(ADR-0010), first executed by the CI run on the merge.

Roles: deepseek-v4-pro implements via the deepseek-pro profile;
claude-sonnet-5 verifies via the claude native profile (proven on
drive-survival r3); qwen3.8-max captains - via qwencloud once the
weekly token plan resets 2026-08-25 20:47 UTC, or via the openrouter
profile before then, probed at launch; deepseek-v4-flash plans and
recovers. This release does not alter trust rules, receipt identity,
admission gates, containment, or what any control verb is permitted to
do; journal schema changes (S6, S8) ship with honest migrations, and
all schema growth on receipts, eval records, events, and telemetry
payloads is additive and versioned.

# Revision 2

Captain escalation ee07ebf9 on S1-usage-truth raised one plan decision:
A4's honest sworn.eval schema bump requires the eval schema-version
allowlist at internal/journal/observer.go, which revision 1 left outside
S1's scope. This revision takes the captain's recommended option -
S1's edit surface gains internal/journal, bounded by an appended
constraint to exactly that allowlist, with no table change - consistent
with the approved standing ruling that all schema growth is additive
and versioned. S2-S9 are retained byte-identical with unchanged
digests; only S1 is invalidated, and its outcome, acceptance, and
consumed products are unchanged, so no downstream slice's consumed
input changes. The revised contract lives at a new path (rev2/) because
recorded contract files are immutable; its canonical digest
(sha256:03d55337715f949c71dc3b89afcdde46398a207ac37dd42b745a76963806eda7)
is byte-for-byte the digest the in-run planner's sealed revision-2
proposal declared - this revision is that proposal, operator-carried
per the summary-before-plan doctrine, under
operator://2026-08-23-telemetry-foundations/2.

# Revision 3

The r2 captain escalation on S1 (design attempt 2) found the second and
final out-of-scope pin of the eval schema version: two e2e conformance
assertions (test/e2e/turn_recovery_linux_test.go:711, :1642) pin
journal.EvalSchemaVersionV2 on live-stamped records, and test/e2e was
outside S1's surface. An operator sweep of the whole tree confirms
these two sites and the revision-2 observer.go allowlist are the only
pins outside S1's existing scope - no further revision of this class
remains. S1's edit surface gains test/e2e bounded to exactly those
assertions; everything else is unchanged from revision 2, and S2-S9
remain byte-identical with unchanged digests. The captain's receipt
also settled both implementer adjudications (canonical contract digests
verified binding; no-conversion cost strategy confirmed) - they need
not recur. Operator-carried under
operator://2026-08-23-telemetry-foundations/3 per the
summary-before-plan doctrine.
