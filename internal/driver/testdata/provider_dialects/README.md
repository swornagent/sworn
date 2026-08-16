# Recorded provider-dialect fixtures (2026-08-15/16)

These are the actual recorded wire exchanges the slice contract
`S1-provider-dialect-extensions` cites as acceptance evidence. They were
captured live by the operator session against the real endpoints on
2026-08-15/16:

- Google `generativelanguage.googleapis.com` OpenAI-compatible
  chat/completions (gemini-3.7-flash)
- xAI `api.x.ai/v1/responses` (grok-4.6)

The extension points are defined by these recorded exchanges and nothing else;
the allowlists in `internal/driver` align exactly to this recorded field set.

## Files

- `gemini_chat_response_thought_signature.json` — one chat-completions
  response carrying `message.extra_content.google.thought_signature` (the
  model's opaque reasoning-continuity signature). This is the assistant
  message that must be retained and replayed byte-exact on every subsequent
  turn.
- `gemini_chat_request_replayed_signature.json` — the second-turn request that
  replays that assistant message **with** the signature. The model correctly
  recalled its own prior turn ("The secret number was 4").
- `gemini_chat_request_without_signature.json` — the same second-turn request
  **without** the signature. The model answered confidently and wrongly ("is
  3") with no error: the silent degradation that makes replay mandatory and
  fail-closed (A2).
- `grok_responses_response_usage.json` — one /v1/responses response whose
  usage block carries xAI's vendor accounting decorations
  (`num_sources_used`, `num_server_side_tools_used`, `cost_in_usd_ticks`,
  `context_details`) beside the standard token fields (A3).

## Provenance note

These bytes are the recorded exchanges, not shapes derived from documented
formats. Tests may reduce the A6 certification-regression fixtures from
`gemini_chat_response_thought_signature.json` and
`grok_responses_response_usage.json`, but the extension-point allowlists must
always match the recorded field set above.
