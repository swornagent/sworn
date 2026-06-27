# SwornAgent API Contract (Stub)

This document defines the expected request/response shapes for the SwornAgent
proxy and account APIs. It is a stub — the backend implementation is not yet
live. The shapes here are the contract the CLI (`sworn`) codes against.

## Proxy endpoint

### Request

```
POST <host>/proxy/v1/<modelID>/chat/completions
Authorization: Bearer <sworn-token>
Content-Type: application/json

{
  "model": "<model-name>",
  "messages": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "..."}
  ],
  "tools": [...]  // optional
}
```

The request body is identical to the OpenAI `/chat/completions` format.
SwornAgent adds provider authentication server-side using the bearer token.

`<modelID>` is the full `provider/model` string (e.g. `openai/gpt-4.1`),
URL-encoded in the path. `<host>` is the compiled-in default
(`https://api.swornagent.com`) unless `SWORN_PROXY_URL` is set (test-only
override).

### Response

```json
{
  "choices": [
    {
      "message": {"content": "...", "tool_calls": [...]},
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 100,
    "completion_tokens": 50,
    "total_tokens": 150
  }
}
```

Same shape as the OpenAI `/chat/completions` response. Unknown fields are
silently ignored (normalisation).

### 402 Payment Required (insufficient credits)

When the user's credit balance is exhausted, the proxy returns:

```
HTTP 402
{"error": "insufficient credits"}
```

The CLI surfaces this as: `out of SwornAgent credits — run \`sworn account buy\` to add more`
and does **not** fall back to a direct provider call.

## Account credits endpoint

### Request

```
GET <host>/account/credits
Authorization: Bearer <sworn-token>
```

### Response

```json
{"credits": 47}
```

`credits` is an **integer count** of credits (Coach ack pin A). The
credit-to-token-to-currency conversion rate is a backend concern and is not
exposed by this API.

## Credit unit

A "credit" is an integer count. `FetchCredits` returns `int`. `sworn account`
displays `Credits: <int>`. `sworn account buy <N>` treats `<N>` as a number of
credits. All three agree on the unit.