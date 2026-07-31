# Baton 1.0 protocol

Baton describes how software work is handed from one AI agent to the next
without losing the approved goal or merging an unchecked result. Sworn is the
reference engine that runs the team.

The normal path is:

```text
approve the work
  -> explain the approach
  -> Captain checks it
  -> build it
  -> fresh Verifier checks it
  -> merge exactly what passed
```

After `FAIL`, the same independent Verifier may check the repair under the
[direct-repair rule](PROTOCOL.md#direct-repair-continuation), or a new Verifier
may start fresh. Both check the complete current candidate.

Read:

1. [CORE.md](CORE.md) for the five promises Baton protects;
2. [PROTOCOL.md](PROTOCOL.md) for who does what and what gets saved;
3. [ASSURANCE.md](ASSURANCE.md) for when work needs stronger proof; and
4. [CONFORMANCE.md](CONFORMANCE.md) for how to show an implementation really
   follows Baton.

[RATIONALE.md](RATIONALE.md) explains the boundary.

The plan says what the finished work must do, not every file or command likely
to be involved. Supporting files and extra checks discovered along the way stay
with the same piece of work. A real change to the promised result, its product
inputs, or who may approve it needs a revised plan.

Git keeps earlier plans and attempts. Small machine-written receipts connect
decisions to saved work that cannot quietly change. The board reads those facts
and shows the furthest trustworthy progress; it does not keep a second version
of the truth.

The portable kit contains the five skills, record helpers, tests, and a
read-only terminal and browser board. Drivers, scheduling, retries, recovery,
and telemetry belong to an engine such as Sworn.
