# Baton 1.0 schemas

The delivery-record surface contains exactly four shapes:

- `delivery-plan-v1.json` — approved delivery and embedded work contracts;
- `submission-v1.json` — exact candidate, checks, and evidence;
- `delivery-verdict-v1.json` — fresh review of one immutable submission; and
- `delivery-board-v1.json` — read-only derived projection.

`assurance-policy-v1.json` separately validates the content-addressed Standard
check and pack registry referenced by a plan. `control-receipt-v1.json`
validates engine-stamped authority approval, verifier dispatch, and integration
receipts. They are policy input and engine facts, not extra delivery records.

Schema validity does not prove the cross-record or repository invariants in
[`../baton/CONFORMANCE.md`](../baton/CONFORMANCE.md). Engines must enforce both.
Record and policy bindings use canonical JSON digests; artifact pointers use
exact raw-byte digests as defined there.

Schemas from Baton 0.x are available at the `v0.16.0` tag and are unsupported by
Baton 1.x.
