# Baton examples

These files form one coherent Standard delivery. Baton records and the assurance
policy use RFC 8785 canonical JSON digests. Artifact pointers declare their media
type and use SHA-256 over the exact stored bytes, including the final newline.

## Canonical JSON digests

| Value | Digest |
|---|---|
| `assurance-policy.json` | `sha256:7a97154ed556cf6821be212cc8a8b97268e4bb74e5d6286e0337f157e00c2a23` |
| `authority-source.json` | `sha256:d21cef146645aef455a3ce750c19c705ae2d75c3a74ef22baae3bfb94132c839` |
| `standard-plan.json` | `sha256:5f44521823b466b350b572813c7aa8677a5e487e4eadfc8f35fde23580f5422f` |
| plan authority object | `sha256:20d9d443a98f0a43d64e4eaffdb29bf111c1a00f7c42847094a5a57e81d8da4b` |
| extracted `health-endpoint` contract | `sha256:3636fadbe95f88831a30d05044113459932e813fb36d4b30cee623663e219a94` |
| `standard-submission.json` | `sha256:51532765e47ad1d3414a7753e025ac17646cbbae70cd8ec63f5f3487b125a2f6` |
| `pass-verdict.json` | `sha256:4f1e638be19a8fa258aed350a10006a9eca169bf98952d4bbed8e4e3edf5dc0d` |
| `delivery-board.json` | `sha256:7db639182bf0c70c7fac03fa460f3ce912ce6135ee5c5acd091e5c5d8be7bc24` |

## Raw artifact digests

| Artifact | Media type | Digest |
|---|---|---|
| `policy/checks/test.json` | `application/json` | `sha256:6151de5a31da2453883460a952adf269417da47c06c67227dbe5b4db100ff782` |
| `policy/packs/security-v1.json` | `application/json` | `sha256:38b36ce18742242fd613a9c807ef46e29897b94b4eec94553876f15e184ff501` |
| `artifacts/authority/plan-approval.json` | `application/json` | `sha256:27d399f256917b52d6e0cdc845a34f9281d30cf3637003bd4ba9a32c5fe59492` |
| `artifacts/checks/test.log` | `text/plain` | `sha256:7764922a6c0dd026058180a2dd1f52daf8465e128d9e2f100869ed8574ca0d06` |
| `artifacts/evidence/health-smoke.json` | `application/json` | `sha256:5d73c8862750dd0d151e6955facb950ad8a34e40f082b4aacd87d96e9e1d80ed` |
| `artifacts/dispatch/verifier-run-1.json` | `application/json` | `sha256:25d5e84ec61e8c72c25b257e62d1397cd313cebd97d2038fa783f9926b22bf22` |
| `artifacts/integration/target-cas.json` | `application/json` | `sha256:1dd439e16d2e382bc3e9b8054ff9f5611f3cacde29fa60f405df43c34f21e1c1` |

The authority source and schema-valid assurance-policy registry are engine or
project policy inputs. Approval, verifier-dispatch, and integration artifacts
use `control-receipt-v1`; check and evidence bytes remain producer artifacts.
None is an additional Baton delivery record type.
`authority-source.json` is a resolver fixture, not authentication proof. A real
engine must authenticate its source and approval against a protected trust root
or capability as required by B1; the portable checker cannot prove that boundary.
