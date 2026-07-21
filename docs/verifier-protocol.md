# Independent verifier protocol boundary

Sworn's verifier protocol separates model judgment from delivery authority.
The model may emit one strict assessment, which is parsed into an immutable
capability before use. The engine alone creates the verifier dispatch and
stamps the Baton delivery-verdict envelope.

This is a protocol foundation, not yet an executable verifier path. Store
admission, effect recovery, the native Codex adapter, current-authority gates,
and outcome routing remain subsequent v0.3.0 slices.

## Three closed records

| Record | Owner | Authority |
| --- | --- | --- |
| `control-receipt-v1` / `verifier_dispatch` | engine | Exact submission and candidate requested for isolated review |
| `sworn-verifier-assessment-v1` | verifier model | Decision content only; no delivery authority |
| `delivery-verdict-v1` | engine | Baton envelope bound to the exact assessment and review inputs |

The assessment contains its local schema marker plus only `outcome`, `summary`,
`acceptance_results`, `assurance_results`, and `findings`. It cannot supply a
verdict ID, submission identity, dispatch pointer, agent, review run, freshness
claim, or timestamp. Parsers accept exactly one strict I-JSON object; they do
not scan prose or remove Markdown fences. Verdict construction accepts only
the resulting exact capability, not a programmatically constructed assessment.

## Dispatch construction

`protocol.BuildVerifierDispatch` accepts an immutable exact submission plus a
dispatch ID, workspace label, and durable creation time. It derives the
submission digest and candidate commit/tree. It rejects dispatch before
submission creation and reuse of the builder run identity before a verifier
turn can be spent.

The constructor stamps Baton's closed verifier-dispatch isolation fields:

- fresh context is required;
- builder transcript is absent;
- the target ref is not writable;
- Git remotes are absent; and
- inherited write credentials are absent.

Parsing proves that those fields and values are present. It does not prove that
the eventual process or materialized workspace actually had those properties;
the effect and Store closure must establish that before dispatch.

## Verdict construction and binding

`protocol.BuildDeliveryVerdict` copies only the model-owned assessment fields.
The engine supplies the verdict identity, agent, review times, and the exact
dispatch artifact pointer. The builder derives all submission, delivery, work,
and review-run identities from immutable inputs and returns RFC 8785 canonical
record bytes.

The pure binding validator checks:

- exact plan, work contract, target, policy locator/digest, and assurance
  selection;
- submission ID/digest and candidate equality;
- dispatch role, isolation constants, submission, candidate, run, and time;
- raw dispatch artifact digest and exact pointer equality;
- a verifier run distinct from the builder run;
- exact acceptance-result and assurance-pack sets;
- evidence existence, reverse links, and required evidence boundary;
- finding references; and
- declared passing check outcomes and evidence references for `PASS`.

It also implements Baton's four outcome/finding rules without inferring an
outcome from prose or transport state. A verifier timeout, malformed response,
or execution failure is not an `INCONCLUSIVE` verdict.

This pure layer assumes the exact submission came from the prior Store-owned
admission edge. It does not independently re-prove Git objects, diff or scope,
policy contents and required-check selection, authority-receipt authenticity,
or check/evidence artifact bytes.

## Digest and admission boundary

Baton record digests cover canonical JSON. Artifact pointers cover exact raw
bytes. The published dispatch fixture therefore has raw digest
`sha256:25d5e84ec61e8c72c25b257e62d1397cd313cebd97d2038fa783f9926b22bf22`
and canonical record digest
`sha256:eb84857c04903c647ba2f29fb038409eca798abd85617ce325aad30a593eedb2`;
they are intentionally different. The published PASS verdict remains
`sha256:4f1e638be19a8fa258aed350a10006a9eca169bf98952d4bbed8e4e3edf5dc0d`.

The pure validator receives a pointer and its asserted raw bytes, verifies the
digest, and binds that exact pointer into the verdict. It does not prove that
the pointer's locator resolves from durable storage. Store admission must later
resolve it and re-resolve the bound submission, check, and evidence artifacts
before `PASS`. Admission must additionally prove current authority, actual
executor/profile facts, write-once IDs, event order, current verdict selection,
and candidate retention. A parsed or constructed verdict grants no integration
capability by itself.
