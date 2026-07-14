# Brief — local + cloud model providers (Ollama Cloud, local Ollama, llama.cpp, LM Studio, vLLM)

**For a fresh session.** Self-contained. Branch off `main` (everything below is verified against
`main` @ `637d05c`, after sworn#107 merged).

**Companion:** `docs/captures/2026-07-14-outstanding-work-catalogue.md` — read it, and
cross-reference before filing anything as new.

---

## 1. The ask

Run sworn against **Ollama Cloud** (the Coach has a subscription), **local Ollama**, and the
other popular local aggregators — **llama.cpp**, **LM Studio**, **vLLM**, **LocalAI**.

## 2. What already exists (verified, not assumed)

| | state |
|---|---|
| **local Ollama** | A native driver exists (`internal/model/ollama.go`, `NewOllama`, `POST /api/chat`). It is **Verify-only and deliberately NOT registered** in the driver registry — `internal/driver/registry/registry.go:262` says so explicitly. **So `sworn verify` / `llm-check` can use it; THE LOOP CANNOT.** |
| **Ollama Cloud** | does not exist |
| **llama.cpp / LM Studio / vLLM / LocalAI** | do not exist |

## 3. The finding that should shape the whole slice

**Every one of these is OpenAI-compatible.**

- Ollama Cloud exposes an OpenAI-compatible layer at **`https://ollama.com/v1`**, Bearer auth
  via **`OLLAMA_API_KEY`**; cloud models carry a **`:cloud` suffix** (e.g. `gpt-oss:120b-cloud`,
  `glm-5:cloud`). Native API is `https://ollama.com/api/chat`.
  Sources: <https://docs.ollama.com/cloud>, <https://docs.ollama.com/api/openai-compatibility>
- Local Ollama exposes the same shim at **`http://localhost:11434/v1`**.
- llama.cpp (`llama-server`), LM Studio, vLLM and LocalAI all serve
  **`/v1/chat/completions`**.

And `internal/model/provider.go` **already has 16 `case` arms and 8 hardcoded base URLs**, every
one of the OAI-compatible arms building the *same* struct:

```go
case "deepseek":   return &OAI{BaseURL: "https://api.deepseek.com/v1",    APIKey: ..., ...}
case "groq":       return &OAI{BaseURL: "https://api.groq.com/openai/v1", APIKey: ..., ...}
case "mistral":    return &OAI{BaseURL: "https://api.mistral.ai/v1",      APIKey: ..., ...}
```

> **Adding four bespoke drivers would make that switch twelve cases of one thing.**
>
> That is *exactly* the defect class the 2026-07-13/14 session spent itself excising — **"N
> places that should be one → they drift → the divergence is silent"** — found five times
> independently (see `2026-07-14-architecture-review-brief.md` §2). Do not add a sixth
> instance on request. **The provider list is data. Make it data.**

This closes **sworn#15** — *"Architecture: self-registering provider factory to eliminate
`provider.go` touchpoint collision"* — which the codebase has known about for weeks.

## 4. The real unlock is not the new providers

It is that **local Ollama is Verify-only**. Registering the OAI-compatible local providers as
**chat-capable** is what lets the **loop's implementer** run on a local model. That is what a
subscription actually buys. The new endpoints are the easy half.

---

## 5. Design direction (argue with it; do not just implement it)

**A declared endpoint table — provider name → endpoint descriptor — replacing the OAI-compatible
switch arms.** Adding a provider becomes a *data* change.

```
name          base URL (default)              key            notes
------------  ------------------------------  -------------  -----------------------------
openai-*      https://api.openai.com/v1       OPENAI_API_KEY
deepseek      https://api.deepseek.com/v1     DEEPSEEK_API_KEY
groq          https://api.groq.com/openai/v1  GROQ_API_KEY
mistral       https://api.mistral.ai/v1       MISTRAL_API_KEY
openrouter    https://openrouter.ai/api/v1    OPENROUTER_API_KEY
xai           https://api.x.ai/v1             XAI_API_KEY
cloudflare    …/client/v4/ai/v1               CLOUDFLARE_API_KEY
ollama-cloud  https://ollama.com/v1           OLLAMA_API_KEY   models carry a :cloud suffix
ollama        http://localhost:11434/v1       (none)           OAI shim; native driver stays
llamacpp      http://localhost:8080/v1        (none)
lmstudio      http://localhost:1234/v1        (none)
vllm          http://localhost:8000/v1        (none)
localai       http://localhost:8080/v1        (none)
```

Base URLs must be **overridable per provider** in `config.json` — a local server can live
anywhere, and hardcoding a port is the same mistake in a smaller font.

**Credentials are already solved.** The layer landed in #107: `model.ProviderKey(provider)` reads
the **canonical** env var (`OLLAMA_API_KEY` — no `SWORN_` prefix) then `credentials.json` (XDG,
0600). Add `ollama-cloud → OLLAMA_API_KEY` to `canonicalKeyEnv` and it works. **Do not invent a
second key path** — three of them disagreeing is what caused the bug this all came from.

### 5a. THE TRAP — read this before touching the registry

`internal/driver/registry/registry.go:350` — `keyProbe` decides a provider is *available* by
`keyFor(cfg, prefix) != ""`.

**Local providers have no key.** Register them behind `keyProbe` and they are **permanently
unavailable**, silently, forever — and `sworn capabilities` will cheerfully report them as
absent.

Local providers need a **reachability probe** (can I open `http://localhost:11434/v1/models`?),
not a key probe. Get this wrong and you ship a provider that can never dispatch, with a green
build.

### 5b. "OpenAI-compatible" is a spectrum, not a contract — THE HARD PART

The Coach's question, and it is the right one: *"there are differences between OAI-compatible
providers — is there a standardised way they report them, or do you just have to try and fail?"*

**Short answer: there is no universal standard, and sworn has already been bitten twice.**

The scar is in the code. `internal/model/oai.go:88`:

```go
Content string `json:"content"` // EVAL FIX 2026-06-28: omitempty dropped 'content' on
// tool-only assistant turns → OpenAI "content: got null" / DeepSeek "missing field content".
// Always emit (incl "").
```

That was found in production, by failing. So was the nullable-`tool_calls` bug. **Two wire
divergences, both discovered the hard way, neither predicted by any metadata.**

#### The two things people conflate — separate them or this slice will fail

|  | **Capability** | **Dialect quirk** |
|---|---|---|
| asks | *Can this model do X?* | *How does the wire differ, even for X it CAN do?* |
| e.g. | structured output? tools? reasoning? | `content` must be present, not omitted · `tool_calls` may be `null` · `max_tokens` vs `max_completion_tokens` · `system` vs `developer` role · stop-sequence caps · whether `temperature: 0` is honoured |
| reported by | **partially** — see below | **nothing. Ever.** |
| owned by | **S04-provider-registry** (already specced) | **this slice** |

#### What capability reporting actually exists (descending reliability)

1. **OpenRouter `/api/v1/models` → `supported_parameters`.** The closest thing to a
   machine-readable capability report (`response_format`, `structured_outputs`, `tools`,
   `reasoning`, …), and it covers many providers *because OpenRouter proxies them*. **This is
   precisely what `S04-provider-registry` is designed around** — read its spec. The capability
   half of the Coach's question is *already solved and specced*. Do not re-solve it.
2. **Provider-specific metadata.** Mostly useless for this. Ollama's `/api/show` returns model
   family and parameter size, not API capability. OpenAI and Anthropic publish nothing
   machine-readable.
3. **Attempt-and-degrade.** Try strict `json_schema` → on 400, fall back to a forced tool call →
   on 400, fall back to prompt-and-parse. `S04`'s spec already mandates this for sparse-metadata
   providers.

#### But nothing — nothing — reports dialect

`supported_parameters` will never tell you that DeepSeek rejects a *missing* `content` field.
There is no endpoint, no header, no spec that will. **Dialect is discovered by failing.**

So the only honest engineering answer is: **stop discovering it in production, and build the
thing that discovers it for you.**

#### The deliverable that answers the question: a live ENDPOINT-conformance suite

`internal/driver/drivertest/` already exports a behavioural conformance suite (`drivertest.Run`)
— but it asserts the **Driver interface** contract (roles, `ErrKind`). Its missing sibling is an
**endpoint conformance suite**: one probe matrix, run **live** against *every* OAI-compatible base
URL, that reports each endpoint's dialect **from observed behaviour rather than from a guess**.

Probe the axes that have actually diverged in the wild — start with the two that already bit us:

| probe | what it establishes |
|---|---|
| assistant turn with `content: ""` and only `tool_calls` | does it demand `content`? *(the 2026-06-28 bug)* |
| response with `tool_calls: null` | do we parse it? *(the second bug)* |
| `response_format: {json_schema, strict: true}` | strict structured output? |
| a single forced function tool | tool-call structured-output fallback? |
| `max_tokens` **and** `max_completion_tokens` | which name does it take? |
| `system` role vs `developer` role | which does it accept? |
| `temperature: 0` | honoured, or silently ignored? *(every deterministic gate depends on this)* |
| multiple stop sequences, streaming, `reasoning_effort` | accepted / rejected / ignored? |

Each probe is one small live call. Output: a **dialect record per provider**, generated from
observation, feeding the dialect table in §5b. Run it in `.github/workflows/live.yml` — nightly
and on demand, **never in the PR gate**.

> This is the same move as everything else that worked this week: **a guard, not a document.**
> A hand-maintained quirks table drifts the moment a provider ships a change. A suite that
> *re-derives* the table nightly cannot. And when the next provider is added, its dialect is
> **discovered**, not assumed — which is the whole difference between adding a provider in an
> afternoon and finding out in production three weeks later.
>
> The last probe in that list — `temperature: 0` — deserves special attention. **Every
> deterministic gate in sworn assumes it.** If a local aggregator silently ignores it, the LLM
> checks stop being reproducible and nothing anywhere would tell you.

### 5c. Structured output — be conservative, and do not duplicate S04

`OAI.Structured` selects the ADR-0011 structured-output mechanism
(`StructuredResponseFormat` / `StructuredToolCall`). **Local aggregators vary wildly** — some
support strict `json_schema`, some tool-calling only, some neither. A wrong advertisement here is
a *silent* failure: the LLM-check contract (`llm-check-report-v1`) fails closed on a malformed
response, so a model that cannot honour `response_format` makes every check red.

Default to **not advertising** structured output for local providers, and let the reactive
attempt-and-degrade path handle it — **which is exactly what `S04-provider-registry` is being
built for**. Do not build a capability registry here. Declare the endpoint; leave capability
detection to S04. (`S04` is *capabilities + pricing metadata synced from OpenRouter* — a
genuinely different concern from *endpoints*. They meet; they are not the same slice.)

---

## 6. Scope

**In:** the declared endpoint table; `ollama-cloud`; local `ollama`/`llamacpp`/`lmstudio`/`vllm`/
`localai`; per-provider base-URL override in `config.json`; keyless local providers; a
reachability probe; **registering the OAI-compatible local providers as chat-capable so the loop
can use them**; keeping the native Ollama driver (it is genuinely different, and useful); **the
dialect table and the live endpoint-conformance suite that populates it (§5b)**.

**Out:** the capability/pricing registry (**S04**, already specced — do not duplicate);
eval-based routing (S06); any new credential path (#107 landed it).

**The seam with S04, stated once:** S04 answers *"what can this model do?"* from OpenRouter
metadata + attempt-and-degrade. This slice answers *"how does this endpoint's wire differ?"* from
live probes. They meet at the OAI driver and they are not the same question. If you find yourself
building a capability cache here, stop — that is S04.

---

## 7. Definition of done

- [ ] **A live reachability artefact against Ollama Cloud** — the Coach has a subscription, so
      this is testable *for real*. Add it to `.github/workflows/live.yml` (the nightly/on-demand
      workflow, **never the PR gate** — a live call is non-deterministic, and a flaky gate gets
      muted; a muted guard is a decoration).
- [ ] **A live artefact against a local Ollama**, or an explicit skip with a stated reason.
- [ ] **`sworn run` demonstrably dispatches its implementer to a local model.** This is the point
      of the slice; a passing unit test is not this artefact.
- [ ] **The keyless-probe guard has a mutation proof**: register a local provider behind
      `keyProbe`, watch it go permanently unavailable, restore, watch it resolve. Record both.
      (Rule 12 — a guard that has never failed is a decoration.)
- [ ] Adding a *new* OAI-compatible provider is a **one-line data change**, proven by doing it.
- [ ] **The endpoint-conformance suite runs live against every OAI-compatible provider** and
      emits a dialect record per endpoint. It must **re-derive** the dialect table, not merely
      assert a hand-written one — a hand-maintained quirks table drifts the moment a provider
      ships a change.
- [ ] **The suite reproduces both known dialect bugs** as probes: the `content`-omitempty
      divergence (`oai.go:88`) and nullable `tool_calls`. If the suite cannot detect the two
      quirks we have *already been bitten by*, it will not detect the next one — that is a Rule 12
      scope-parity failure, and the whole suite is a decoration.
- [ ] **`temperature: 0` is probed on every endpoint.** Every deterministic gate in sworn assumes
      it is honoured. A local aggregator that silently ignores it makes the LLM checks
      non-reproducible and nothing would say so.
- [ ] `go test ./...` green, `gofmt`/`vet` clean, `bash scripts/public-safe-scan.sh` passes.
- [ ] Cross-referenced against the 81 open issues; **closes #15**; explicitly *not* S04.
- [ ] Proof bundle at `docs/captures/<date>-local-providers-proof.md`, from live repo state.
