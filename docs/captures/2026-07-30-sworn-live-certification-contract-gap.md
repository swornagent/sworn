# Sworn live driver certification repair

## Outcome

The final candidate reaches every configured provider with the exact configured
model. The identity-bound evidence bundle has a current PASS for seven
profiles:

- Bedrock Mantle, `openai.gpt-oss-120b`;
- Bedrock Runtime Converse, `amazon.nova-pro-v1:0`;
- Claude Code CLI, `claude-sonnet-4-6`;
- Codex CLI, `gpt-5.6-sol`;
- DeepSeek, `deepseek-v4-flash`;
- Gemini GenerateContent, `gemini-3.5-flash`; and
- native OpenAI Responses, `gpt-5.6-sol`.

The earlier aggregate run named a malformed DeepSeek submission and an OpenAI
quota limit. Both first failures remain in the evidence. DeepSeek passed its
unchanged targeted recheck; after API credit was restored, native OpenAI moved
to the recommended Responses surface and passed its targeted final-candidate
check. No adapter retries internally, and no driver, model, credential, or
provider fallback substitutes for either profile.

This was a forward repair of the existing `W8-parity-release` slice. It did not
replace the plan, reset completed slices, create replacement work IDs, or
discard prior evidence.

## Bound candidate and configuration

- stripped binary: `/tmp/sworn-responses-final.Kz6aNX/sworn-a`;
- binary SHA-256:
  `7a72bb6bb25c15147bcd185f8dd28172470a1ba2a1813989ff5f6a39f77d4f28`;
- binary size: 22,237,346 bytes;
- driver configuration:
  `/home/brad/.config/sworn/v1-rc-driver-config.json`;
- configuration SHA-256:
  `12ab8326666c5c942db23d125c21e29c2165dcbd59d4d849356cf2443c0a35af`;
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

The historical aggregate evidence binds its then-current configuration digest
`sha256:cbce7163485c5b9cdcd99ffc6ccba440db661d312f217804ac953902945c164e`.
The final secret-free configuration used by current inspect and targeted
OpenAI certification binds
`sha256:12ab8326666c5c942db23d125c21e29c2165dcbd59d4d849356cf2443c0a35af`.
Fresh final-binary `driver inspect --all` and `driver doctor --all` evidence is
`/tmp/sworn-v0.3.0-inspect-frozen.iVPbAJ.json`
(`6a30bdc7ebb9d68a3ff0ad53f6b71079d4e6e86e5da2fa2524c081c10721007c`)
and `/tmp/sworn-v0.3.0-doctor-frozen.IECA0e.json`
(`6eb6d7cb53ea0f4906517d6b7b6c0e09eda6ee1ca1cc2f7de78f6bea7c59610f`);
each returns seven PASS reports.

## Native OpenAI uses Responses

After API credit was restored, the OpenAI profile exposed a protocol mismatch
rather than a model-capability failure: GPT-5.6 Sol rejected function tools
combined with reasoning effort on Chat Completions. Chat works with reasoning
explicitly disabled, but that would discard useful reasoning in the four
model-facing Baton responsibilities.

OpenAI recommends Responses for new projects and specifically for GPT-5.6
reasoning, tools, and multi-turn work:

- <https://developers.openai.com/api/docs/guides/migrate-to-responses>
- <https://developers.openai.com/api/docs/guides/latest-model>

Sworn therefore treats Responses as the native OpenAI surface. The existing
role-neutral loop remains the only scheduler and tool authority. A small wire
codec sends `store:false`, explicit `reasoning.effort`, and the existing
function tools; it retains each bounded output Item exactly, then correlates
`function_call_output` by `call_id`. Encrypted reasoning stays memory-only and
is replayed to OpenAI, never exposed as a Sworn result. Chat Completions remains
an explicit compatibility surface for providers that speak that dialect.
There is no dialect guessing, fallback, provider SDK, hosted tool, or second
orchestration loop.

The exact certification prompt and tool set first proved the wire contract
directly:

| Evidence | SHA-256 | Result |
| --- | --- | --- |
| `/tmp/sworn-responses-request.mAl9FN.json` | `5d5d862de945383780bec301a5bbfd37f8f1e1bc8c79fbbe1274fa572d8ea6ba` | first Responses request |
| `/tmp/sworn-responses-body.njXpaa.json` | `0100eacf605ee5fc5802d4a5b087e3841160f74801e60e685a22d5094fbfac54` | encrypted reasoning plus `Read` call |
| `/tmp/sworn-responses-request2.BR1pZX.json` | `efc8a4133e844d33b0156886b0e8ee3e24c99d9b3ca1721ab0dc490f280e81fc` | exact Item replay plus correlated tool output |
| `/tmp/sworn-responses-body2.tSEQBS.json` | `162df51ea65a684d3f7f855cfb4bdb0f4dacc82ccd25632d442bdf9385094124` | encrypted reasoning plus terminating `sworn_submit` |

The first integrated targeted certification also passed as
`openai_responses` with `gpt-5.6-sol`; final-candidate evidence below supersedes
that development binary:

| Evidence | SHA-256 | Result |
| --- | --- | --- |
| `/tmp/sworn-responses-final.Kz6aNX/sworn-a` | `7a72bb6bb25c15147bcd185f8dd28172470a1ba2a1813989ff5f6a39f77d4f28` | reproducible stripped final binary |
| `/tmp/sworn-responses-final.Kz6aNX/driver-inspect-all.json` | `06c7947457ad25253f9649e9b53e0ee0a1f107c381472861eb93372dc5eeba4e` | seven configured profiles PASS offline inspection |
| `/tmp/sworn-responses-final.Kz6aNX/driver-certify-openai-sourced.json` | `c471080180ff9047ed2f7ecfccdd0bf89aedd9f4fef623b966e2158bf38172b6` | final OpenAI Responses live certification PASS |

The first final-binary invocation had not loaded the owner environment and
failed at `certification_credential_failed` before contacting OpenAI. Loading
the existing owner-only environment resolved that procedural seam; no adapter
or model retry hid it.

DeepSeek, Mantle, Gemini, Claude, Codex, and Bedrock were not live-rerun merely
because the native OpenAI dialect changed. Their bound profile, model, surface,
and configuration facts remain current, and the final shared corpus re-proves
their relevant wire behavior.

## Gate lesson

A monolithic all-green live-model sample amplifies independent provider and
model variance: one miss makes every already-green profile run again. The
lean evidence shape is one aggregate discovery run followed only by targeted
rechecks of profiles that it names. Each first result remains visible, every
recheck uses the same binary, configuration, profile, and model, and adapters
still perform one attempt. This keeps failures fail-closed without paying for
unrelated successful providers again.

## Release consequence

All offline release gates pass, and the identity-bound live bundle covers every
required profile and surface. Revision 10 now installs this lean evidence rule
without changing a slice identity or resetting prior PASS work. The repository
owner approved and installed the exact revision; fresh W8 Captain, candidate,
and Verifier decisions bind it before composition.
