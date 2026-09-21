# ADR-0015: A portable Manager seat, engine-enforced seat authority, and the v1.0.0 gate

- Status: Proposed (awaiting Brad)
- Builds on ADR-0014 (attended autonomy). Changes none of its decisions;
  sharpens decision 3 ("the first seat is a Claude Code skill") into a
  contract any harness can fill, and states what v1.0.0 has to prove.

## Context

ADR-0014 put an orchestrator seat above the engine and said other seats "are
the same interface driven differently". Track A delivered the first one: the
`/sworn-orchestrate` skill drove release 2026-09-18-seal-time-gates from
launch to a merged outcome with no interventions.

That seat is portable in principle and tied to one harness in practice:

1. Its instructions are a Claude Code `SKILL.md`.
2. Its tick is woken by that harness's monitor tool watching a shell script.
3. It launches the run with `systemd-run` and a `curl` POST of `sworn_start`
   held open as a second unit. The drive lives inside the caller's HTTP
   request, and policy entry M7 exists only to re-issue it.
4. Its decision journal is a markdown file in the ops home, written with the
   harness's file tools.
5. Deep journal reads are delegated to subagents, which only some harnesses
   have.

The Principal wants the seat filled by any agent harness (Claude Code, Codex,
OpenCode, Cline, Hermes Agent and whatever follows), and wants to reach a
running release from a terminal, a cloud session or a phone, using whatever
that harness offers on that surface.

Two findings make this more than packaging:

- **Seat authority is policy text, not an engine fact.** `sworn_approve`
  takes `actor_class` and `actor_authority` as call arguments.
  `internal/runtime/approval.go` says so itself: "ActorAuthority is an exact
  manifest binding, not a credential or identity proof." The serve host has
  one optional bearer token for every caller. "The Manager never approves a
  plan" is therefore a sentence in `docs/policy/manager.md` that a model is
  asked to honour. With one trusted harness on loopback that was tolerable.
  With arbitrary harnesses on arbitrary surfaces it is not.
- **v1.0.0 has no definition.** `v1.0.0-rc.1` was tagged 2026-07-31. Main is
  more than a thousand commits past it, and the vocabulary rename since was a
  hard cut: records written before it cannot be read by the current binary.
  The candidate describes a product that no longer exists, and nothing says
  what the final release must demonstrate.

## Decision

### 1. The Manager seat is filled by any MCP-capable agent harness

The seat contract is Sworn's product MCP surface and nothing else. A harness
that needs a shell, a file path, a skill format or a harness-specific wake
mechanism in order to fill the seat has found a defect in the contract.

As with workers (ADR-0013), the requirement is on capabilities, never on
product names. A harness can fill the seat when it can:

- connect to an MCP server and call tools with structured arguments;
- hold one tool call open for a bounded wait;
- load an instruction text and a policy text and act within them.

Nothing else is assumed: no subagents, no filesystem, no scheduler, no
persistent memory.

### 2. What moves behind the MCP surface

- **`sworn_wait`.** Blocks until the run's status signature changes, the run
  reaches a terminal state, or a bounded timeout passes, then returns the
  same projection as `sworn_status`. This is the tick. Loop state lives in
  the engine, as ADR-0014 asked.
- **Decision records.** `sworn_record_decision` writes the seat's decision
  (situation as typed cause plus evidence identifiers, decision, rationale,
  policy entry, policy version) as a Sworn record before the command that
  enacts it; a read tool returns the bounded recent log. This is Track D's
  first item, brought forward because a file in an ops home is not a record
  any other harness can resume from.
- **The brief and the policy ship from the binary.** The Manager brief and
  the project's `docs/policy/manager.md` are served as an MCP prompt and
  resource. `sworn seat prompt` prints the same text for harnesses that load
  instructions from a file. The skill directory remains as one packaging of
  that text, not its source. The policy version the seat loaded is recorded
  with every decision.
- **The serve host owns the drive.** `sworn_start` returns once the start is
  admitted; the host keeps driving. A caller's connection is never the
  lifetime of a run. Policy entry M7 retires with the held-open start.
- **A bounded "explain" read.** A tool that returns the typed cause, the
  refusal detail, the check output excerpt and the identifiers they rest on
  for one park or one failed work. This is where Track B's "why" audit
  lands, and it replaces the subagent journal dive for harnesses that have
  no subagents.

### 3. Seat authority is enforced by the engine

- **Scoped credentials.** The serve host issues credentials in three scopes:
  observer (read), manager (read, wait, decision records, the control and
  attention actions the policy delegates) and principal. A call outside the
  credential's scope is refused with a typed code that names the scope, so a
  seat that tries learns why. Loopback without a credential keeps today's
  behaviour for the local operator only until the scoped path exists, then
  takes the same rule.
- **A harness may carry a request for a Type-1 decision, never the decision.**
  Plan approval, a revision that changes a contract, roster changes, target
  moves and promotion are confirmed by the human on a surface the harness's
  model cannot operate: the TUI, the browser board, the CLI, or a
  confirmation page reached from a link the engine issues for that exact
  digest. What a model can call through MCP is "ask the Principal", which
  opens the attention and returns its identifier.
- **The seat is attested like a worker.** Each decision record carries the
  credential identifier, the harness name and version as the client reported
  them, the model where known, and the policy version. Routing at the seat
  is as auditable as routing at the workers.
- **Concurrent seats are safe by construction.** Control commands already
  carry `expected_generation` and `expected_epoch`; a second seat acting on a
  stale view is refused, not merged. Several sessions may observe; the
  engine does not need a seat lease to stay correct.

### 4. One engine, many surfaces

The engine runs where the repository, the sandbox and the worker credentials
are. Surfaces attach and detach. Because the seat is stateless between ticks
and its log is a Sworn record, a Principal can start a release from a
terminal, check it from a phone and hand the seat to a cloud session, and
each resumes from the record rather than from a transcript.

Remote reach is a transport question and never an authority question:

- The open product carries everything needed to expose the MCP surface
  through infrastructure the user operates (TLS, scoped credentials, a
  private network or reverse proxy of their choice).
- A relay that gives a node a stable public MCP endpoint may exist. Nodes
  connect outbound to it. It carries typed requests and safe events; it never
  holds repository, record or approval authority, and a relay outage never
  stops a local run.
- Section 3 holds on every transport. A remote manager credential can do
  exactly what a local one can, and a remote Type-1 confirmation needs the
  same human step on a surface the model cannot operate.

### 5. The v1.0.0 gate

v1.0.0 is cut when this sentence is demonstrated, not asserted:

> The Principal approves a plan digest at the start and merges the promotion
> at the end. Everything between is a Manager seat driving the engine inside
> a ratified policy. When the seat cannot decide, the Principal receives a
> typed escalation they can answer from the message alone. Nobody rescues a
> run by hand.

"Autonomous" means the human touches only Type-1 decisions. "Safely" means
the authority boundary holds whichever model and harness is driving, and no
evidence is ever false. "As planned" means the approved contract is what
lands. Running unattended remains a non-goal.

The gates, tracked as a checklist in `docs/releases/v1.0.0-gate.md`:

1. **Engine-enforced seat authority** (section 3).
2. **No known path to false evidence or a run that walls forever.** The
   blocking set is named in the checklist; cost and convenience work is not
   in it.
3. **A qualifying streak** of consecutive releases with no hand operator
   work, covering between them: three or more slices in one release;
   parallel tracks; a foreign repository; a non-Claude implementer; at least
   two real parks resolved inside the policy; and at least two different
   harnesses in the seat, one on a non-Anthropic model. Hand work resets the
   count.
4. **Launch counts.** The clock runs from approval to merged. Plan pin, lint,
   record, manifest and serve start are part of the run the seat drives.
5. **Legibility.** The seat never opens a raw journal or the source to learn
   why. If it has to, that run does not qualify and the gap is filed.
6. **A stateless seat, proven.** A seat session is killed mid-release and a
   fresh one in a different harness finishes from the decision records. One
   policy entry travels from decision record to proposal to ratification.
7. **Seat conformance.** A scripted MCP-only client drives a fake run through
   a park, an attention, a stale-generation refusal and a Type-1 scope
   refusal. It is the definition of what a harness must be able to do, and
   it runs in CI so the surface cannot drift.
8. **Released artefact.** Streak runs use the installed release candidate,
   not a binary built from main.
9. **A declared compatibility surface.** The manifest, contract, record and
   journal schemas, the MCP tools and the refusal codes that v1 freezes are
   written down, and a journal written by the release candidate is read by
   the final binary. After v1.0.0 a hard cut like the vocabulary rename
   needs a migration.
10. **Stranger test.** Someone other than the author goes from
    `docs/install.md` to a merged toy release using the documentation alone.

## Consequences and queue

- **Track B (legibility) is unchanged and is next**: sworn#294, then
  #324 to #327. Gate 5 makes it a v1 blocker rather than a convenience.
- **A new Track F, portable seat**, follows it: `sworn_wait`, decision
  records over MCP, the brief and policy as MCP prompt and resource, the
  serve-owned drive, scoped credentials with typed scope refusals, the
  out-of-band Type-1 confirmation, and the conformance client. It absorbs
  Track D's first item and delivers gates 1, 6 and 7.
- **Track D keeps** the policy proposal path and the preflight registry.
- **Track C (advisory autopilot) is unchanged** and is a good streak release.
- Then a fresh release candidate is cut, and the streak runs on real work.
- `docs/policy/manager.md` loses M7 when the serve host owns the drive, and
  its "Authority the Manager holds" section becomes a description of what
  the manager scope permits rather than a request.

## Not changed

ADR-0014's decisions; the human-scoped release trigger; capability-based
selection (ADR-0013); every attestation seam; the rule that no learned or
delegated fact bypasses human authority.

## Settled with the Principal (2026-09-22)

- The streak is five consecutive qualifying runs.
- Remote reach at the manager scope is in the v1.0.0 engine scope: scoped
  credentials and the TLS path ship in the open engine, because gate 1 needs
  them anyway. Nothing about a relay gates the engine's version.
- Gate 10 blocks the v1.0.0 tag. The stranger is a person other than the
  author, working from the public documentation alone. A clean-machine run by
  a fresh harness session holding only that documentation is the rehearsal
  for it and does not satisfy it.
