# Sworn live driver certification repair

## Outcome

The final candidate reaches every configured provider with the exact configured
model. The evidence bundle has a current PASS for six profiles:

- Bedrock Mantle, `openai.gpt-oss-120b`;
- Bedrock Runtime Converse, `amazon.nova-pro-v1:0`;
- Claude Code CLI, `claude-sonnet-4-6`;
- Codex CLI, `gpt-5.6-sol`;
- DeepSeek, `deepseek-v4-flash`; and
- Gemini GenerateContent, `gemini-3.5-flash`.

The final aggregate run named one model-output failure: a malformed DeepSeek
submission. Its targeted recheck passed without a code or configuration
change. The first failure remains in the evidence; no provider adapter retries
internally.

The remaining OpenAI-compatible HTTP failure is the provider's HTTP 429
`insufficient_quota` response for the configured API key. Codex uses ChatGPT
CLI authentication and passes independently. No driver, model, credential, or
provider fallback substitutes for the failed OpenAI API profile.

This was a forward repair of the existing `W8-parity-release` slice. It did not
replace the plan, reset completed slices, create replacement work IDs, or
discard prior evidence.

## Bound candidate and configuration

- stripped binary: `/tmp/sworn-v0.3.0`;
- binary SHA-256:
  `c6e06d38b1fb650ad02b5ef8de6a4d3c33ffd77ac109058d3b638ff5ea11bc9b`;
- binary size: 22,212,770 bytes;
- driver configuration:
  `/home/brad/.config/sworn/v1-rc-driver-config.json`;
- configuration SHA-256:
  `cbce7163485c5b9cdcd99ffc6ccba440db661d312f217804ac953902945c164e`;
- stable native resolver source:
  `/home/brad/.config/sworn/resolv.conf`;
- resolver SHA-256:
  `7fe98ecaceba71c6efa231e232667ee130d8b8b592c614b2c85f4d1c5c53cd3e`.

The owner-controlled resolver file replaces a volatile
`/run/systemd/resolve/stub-resolv.conf` source. Tailscale had legitimately
rewritten the systemd-generated file between readiness commands, changing its
digest and causing both native closures to fail closed. No digest check was
relaxed or cached across commands.

## What the live gate found

The first live candidate exposed one common gap: the advertised
`sworn_submit` tool did not describe the complete closed submission envelope.
Provider-specific retries then exposed only dialect and native-client seams:

| Surface | Exact live mismatch | Smallest correction |
| --- | --- | --- |
| Common tool loop | incomplete submission schema and ambiguous role-owned optional fields | advertise the existing closed submission fields; name only the current invocation's `result_fields`; keep strict permission validation |
| Mantle | output-only `message.reasoning` | admit bounded inert OpenAI-compatible reasoning metadata, validate it, discard it, and never replay it |
| Gemini | JSON Schema sent through `parameters`; later output-only `finishMessage` | use `parametersJsonSchema`; admit and ignore `finishMessage` |
| Bedrock Runtime | SigV4 canonical model path needed double escaping; usage added `serverToolUsage` | canonicalize the escaped URI; admit and ignore response-bounded server-tool metadata |
| Claude Code | final-output schema competed with `sworn_submit`; Sworn MCP was deferred; MCP calls carried standard `_meta` | remove the competing schema, set only the Sworn server `alwaysLoad:true`, accept object-valued `_meta`, and discard it before execution |
| Codex CLI | invalid `--output-schema` created a competing terminal path | delete the argument and its now-unused mounted schema file |
| Readiness output | provider and model-output failures collapsed to generic tokens | retain a small secret-free stage vocabulary, including provider, response-contract, submission, tool, usage, and resource stages |

Structurally valid fields outside the invocation's responsibility are discarded
at the common tool boundary before strict permission validation. Those fields
carry no authority and are not sealed or persisted. Required fields, exact
bytes, decisions, invocation identity, responsibility, unknown fields, and the
canonical sealed handoff remain fail-closed.

## Evidence progression

All named evidence files are mode `0600`.

| Evidence | SHA-256 | Result |
| --- | --- | --- |
| `/tmp/sworn-v0.3.0-certify.GycxsT.json` | `b50d0f18f187a5bf4c41d05b8e874f97eab8a9f3aae0fc3eb8c716c00f25fb9f` | original generic seven-profile failure |
| `/tmp/sworn-v0.3.0-certify-fixed.tw3DoC.json` | `e2e9177a8b9649eb0cfb87757550e3ab9d9c1ff987a4e3772e1ed51dae2164cd` | common schema repaired; exact dialect gaps exposed |
| `/tmp/sworn-v0.3.0-certify-adapters.auL5Ko.json` | `5b899ed4a106983096083e3ce7635cce7657708e5d232baa99218af76152455f` | Mantle and DeepSeek pass; remaining stages separated |
| `/tmp/sworn-v0.3.0-certify-frozen.4dsQbq.json` | `7621a12b8c1690252f29e23e10510716e280a049a83d04b88ebf2012036a1c8f` | final aggregate passes five profiles and names DeepSeek submission failure plus OpenAI provider limit |
| `/tmp/sworn-deepseek-frozen.u64Dbm.json` | `0e84334b3e5c6848161495a2ab6e9fa129e31487ff567e77136f84efe6c80a9c` | named DeepSeek recheck passes |
| `/tmp/sworn-openai-frozen.pfmY3P.json` | `786b2fb53b1a4b92e5cd3ebba4758fa3afaeb695ddc2bff7d73e7bc2566a710d` | named OpenAI recheck remains provider-limited |
| `/tmp/openai-quota-check.dzWQtm.json` | `7d89b876eb0352487c10e4a9364e450ab3ca59a67a99d6381865aace71ba362a` | direct provider response identifies HTTP 429 `insufficient_quota` |

The aggregate evidence binds configuration digest
`sha256:cbce7163485c5b9cdcd99ffc6ccba440db661d312f217804ac953902945c164e`.
Fresh final-binary `driver inspect --all` and `driver doctor --all` evidence is
`/tmp/sworn-v0.3.0-inspect-frozen.iVPbAJ.json`
(`6a30bdc7ebb9d68a3ff0ad53f6b71079d4e6e86e5da2fa2524c081c10721007c`)
and `/tmp/sworn-v0.3.0-doctor-frozen.IECA0e.json`
(`6eb6d7cb53ea0f4906517d6b7b6c0e09eda6ee1ca1cc2f7de78f6bea7c59610f`);
each returns seven PASS reports.

## Gate lesson

A monolithic all-green live-model sample amplifies independent provider and
model variance: one miss makes every already-green profile run again. The
lean evidence shape is one aggregate discovery run followed only by targeted
rechecks of profiles that it names. Each first result remains visible, every
recheck uses the same binary, configuration, profile, and model, and adapters
still perform one attempt. This keeps failures fail-closed without paying for
unrelated successful providers again.

## Release consequence

The code and six reachable provider surfaces are ready for the normal offline
release gates. The current all-seven gate remains fail-closed until the
configured OpenAI API account has usable quota and one aggregate run passes,
or an approved plan revision adopts the targeted evidence bundle or names an
exact deferral.
