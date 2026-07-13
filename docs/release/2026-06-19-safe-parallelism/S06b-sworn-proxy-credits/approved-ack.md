# Coach ack — S06b-sworn-proxy-credits

**Date:** 2026-06-21
**Acked by:** Coach (human, brad)
**Design reviewed:** `design.md` @ track T3-commercial
**Verdict:** PROCEED to `in_progress` once the three pins below are applied.

The design is sound and well-scoped. The design-review role paged three times across
re-dispatches, each surfacing a different facet of the same billing/payment surface
(rate, credential security, failure path). All three are resolved here so re-review
does not re-page. The spec has been amended with matching acceptance checks and tests.

## Pin A — credit unit contract  *(was: "billing rate discrepancy")*

- A "credit" is an **integer count**. `FetchCredits` returns an int; `sworn account`
  displays `Credits: <int>`; `sworn account buy <N>` treats `<N>` as a number of
  credits (same unit). All three agree.
- The credit→token→currency **conversion rate is a backend concern** and stays out of
  scope for this slice (consistent with the already-excluded external-billing/webhook work).
  Document the unit in the `docs/api-contract.md` stub.

## Pin B — proxy URL is a credential-trust boundary  *(was: "UX security requirement")*

- `model.FromEnv` attaches the sworn **bearer token** to the proxy request. Honouring
  an arbitrary `SWORN_PROXY_URL` in production is a token-exfiltration vector.
- **Decision:** mirror S06a pin 4 exactly. Compile in the default proxy host via an
  ldflags var (same pattern as `SWORN_AUTH_URL`). `SWORN_PROXY_URL` is a **test-only**
  override, not a production config knob. The bearer token is only ever sent to the
  compiled-in default host unless the override is explicitly set; when it is set, the
  client **warns on stderr** that credentials are being routed to a non-default host.
- This lands as a real acceptance check (`TestFromEnvProxyDefaultHost`,
  `TestFromEnvProxyOverrideWarns`) so the fresh verifier gates on it (Rule 7).

## Pin C — payment / credit-exhaustion failure path  *(was: "payment failure handling ambiguity")*

- **Decision:** when the proxy returns `402 Payment Required` (insufficient credits),
  the client returns a non-nil error reading
  `"out of SwornAgent credits — run \`sworn account buy\` to add more"`. It must
  **never** silently downgrade to a direct provider call. Gated by
  `TestFromEnvInsufficientCredits`.

## Notes

- `SWORN_DIRECT=1` bypass remains as specced (sends the provider key, not the sworn
  token) — not affected by pin B.
- No spec deviation requiring `/replan-release`: these are additive clarifications and
  acceptance checks, not a change to the user outcome or touchpoints.

Implementer: address pins A–C inline during implementation; the amended `spec.md`
acceptance checks are the contract. Proceed to `in_progress`.
