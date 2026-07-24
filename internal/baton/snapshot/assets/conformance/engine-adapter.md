# Baton autonomous-engine conformance adapter

Version: `baton.engine-conformance/v1`

This is the process boundary for running Baton's autonomous cases through a
real engine. It does not turn portable fixtures into engine evidence. Until a
case is run through this boundary, its manifest result is `NOT RUN`.

## Commands

```text
adapter info
adapter run < request.json > result.json
```

`info` writes one strict JSON object with exactly `contract_version`,
`engine_id`, and `engine_version`.

`run` reads one strict JSON request from standard input and writes one strict
JSON result to standard output. Diagnostics go to standard error, are bounded,
and contain no credentials, prompts, model output, or repository contents. A
non-zero exit writes no result.

## Request

The request has exactly:

```json
{
  "schema_version": "baton.engine-conformance-request/v1",
  "invocation_id": "unique stable string",
  "case_id": "manifest autonomous case id",
  "baton_version": "1.0.0-rc.2",
  "repository": "/absolute/path/to/temporary/repository",
  "limits": {
    "timeout_ms": 600000
  }
}
```

The harness creates and owns the temporary repository. The adapter must run the
named case through its normal binary, persistence, scheduler, driver, workspace,
and process boundaries. It must not replace those boundaries with Baton
reference helpers or an in-process simulation.

## Result

The result has exactly:

```json
{
  "schema_version": "baton.engine-conformance-result/v1",
  "invocation_id": "same request identity",
  "case_id": "same manifest case id",
  "engine_id": "stable engine identity",
  "engine_version": "engine version",
  "status": "PASS",
  "evidence": [
    {
      "kind": "engine-owned evidence kind",
      "digest": "sha256:lowercase-hex",
      "summary": "bounded factual description"
    }
  ]
}
```

`status` is exactly `PASS`, `FAIL`, or `NOT RUN`. `PASS` requires evidence from
the real engine boundary named by the case. `FAIL` means the case ran and a
required observation failed. `NOT RUN` means the adapter could not execute the
case; absence, timeout before a case begins, missing credentials, and missing
engine support never become PASS.

Evidence summaries are diagnostic only. The harness independently checks the
temporary repository, refs, durable records, process exits, and engine-owned
evidence digests required by each case. A model response or adapter assertion
is not proof of approval, isolation, freshness, concurrency, recovery, or a Git
effect.
