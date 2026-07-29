# Sworn W8 RC9 release gate

## Scope

Plan revision 9 changes only `W8-parity-release`. W0-W6 retain their existing
PASS authority. The approved W8 contract binds Baton `v1.0.0-rc.9`, the
12-case autonomous conformance manifest, Coach-loop parity, every configured
driver surface, and the full release checks.

Authority:

- plan commit `8867ed1ee881086962cbc585d9188f70c53fda46`;
- approval `c588caca645155acee69a7c90b37e96ca8398973`;
- design `90d26b52eb9a55a5372daa2d2c3e4961040b5d14`;
- Captain `81d1f26eb558923a85a01e42de00a55f372abd64`.

## Reuse, not regeneration

The prior unsealed W8 implementation is preserved at
`archive/sworn-v0.3.0-w8-attempt1-pre-revision9-20260729`. Its four product
commits replayed cleanly after fresh design and Captain review. Before any RC9
edit, the replay tree was exactly the archived tree
`456773ab1669bea8545ce5e2c013c5eaa2400662`.

## Minimal RC9 delta

Across Sworn's 24 embedded Baton assets, RC9 changes only:

- `VERSION`;
- `baton/README.md`; and
- `conformance/manifest.json`.

The protocol, five operations, plan template, receipt schema, board and record
references, Coach baseline, and twelve case identities remain byte-identical.
Sworn does not vendor RC9 installers, skill directories, wrappers, or client
paths.

The public `support_package_sha256` field retains its wire name and now binds
RC9's published skills-payload digest. The old local subset catalogue and
duplicate digest calculation are deleted because Sworn does not embed that
payload.

## Driver and evidence boundary

All profiles use the existing role-neutral contract and explicit models.
Bedrock Mantle stays on the common Chat Completions codec with
`openai.gpt-oss-120b`; adding a second Responses loop solely for GPT-5.6 Sol
would duplicate orchestration without improving this gate.

One canonical secret-free host config runs `inspect --all` and `doctor --all`
before a single `certify --all`. Provider-specific retries are used only for a
surface named by that aggregate gate. Claude login is an authentication
prerequisite, not a Baton verdict or a reason to reset the slice.
