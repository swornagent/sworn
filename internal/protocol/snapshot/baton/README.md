# Baton 1.0 release candidate

Start with [CORE.md](CORE.md). The complete normative surface is:

1. [CORE.md](CORE.md) — five trust principles;
2. [PROTOCOL.md](PROTOCOL.md) — records, loop, routing, invalidation;
3. [ASSURANCE.md](ASSURANCE.md) — risk-selected assurance packs; and
4. [CONFORMANCE.md](CONFORMANCE.md) — observable engine obligations.

[RATIONALE.md](RATIONALE.md) maps the Baton 0.x lessons into the smaller model.
The four JSON record contracts plus the non-record assurance-policy and
control-receipt schemas live in [`../schemas/`](../schemas/).

Prompts, commands, installers, role manuals, model selection, and Git procedures
are intentionally outside the protocol.

Final `v1.0.0` requires a real engine to pass every published engine case; the
portable record suite alone is not a release claim.
