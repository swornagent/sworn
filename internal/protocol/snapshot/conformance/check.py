#!/usr/bin/env python3
"""Run Baton's portable strict-JSON, schema, and cross-record checks.

The real-boundary engine cases in manifest.json intentionally require an engine
adapter and are reported as NOT RUN here. This script never turns declared
scenarios into a false conformance claim.
"""

from __future__ import annotations

import copy
import hashlib
import json
import math
import re
import sys
from datetime import datetime, timezone
from decimal import Decimal
from pathlib import Path
from typing import Any

from jsonschema import Draft202012Validator, FormatChecker


ROOT = Path(__file__).resolve().parent.parent
MAX_SAFE_INTEGER = 9_007_199_254_740_991
ZERO_DIGEST = "sha256:" + "0" * 64
BAD_OID = "d" * 40
BOUNDARY_RANK = {"component": 0, "assembled": 1, "live": 2}
FORMAT_CHECKER = FormatChecker()
RFC3339 = re.compile(
    r"^(\d{4}-\d{2}-\d{2})[Tt](\d{2}):(\d{2}):(\d{2})(\.\d+)?([Zz]|[+-]\d{2}:\d{2})$"
)


@FORMAT_CHECKER.checks("date-time", raises=(TypeError, ValueError))
def _is_date_time(value: object) -> bool:
    if not isinstance(value, str):
        return True
    match = RFC3339.fullmatch(value)
    if match is None:
        return False
    date, hour, minute, second, _fraction, zone = match.groups()
    if int(hour) > 23 or int(minute) > 59 or int(second) > 59:
        return False
    if zone not in {"Z", "z"}:
        zone_hour, zone_minute = zone[1:].split(":")
        if int(zone_hour) > 23 or int(zone_minute) > 59:
            return False
    safe_zone = "+00:00" if zone in {"Z", "z"} else zone
    datetime.fromisoformat(f"{date}T{hour}:{minute}:{second}{safe_zone}")
    return True


class StrictJSONError(ValueError):
    def __init__(self, code: str, detail: str) -> None:
        super().__init__(detail)
        self.code = code


def _object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise StrictJSONError("duplicate_key", f"duplicate object name {key!r}")
        result[key] = value
    return result


def _integer(value: str) -> int:
    parsed = int(value)
    if abs(parsed) > MAX_SAFE_INTEGER:
        raise StrictJSONError("unsafe_integer", f"integer outside interoperable range: {value}")
    return parsed


def _number(value: str) -> float:
    parsed = float(value)
    if not math.isfinite(parsed):
        raise StrictJSONError("nonfinite_number", f"non-finite number: {value}")
    if parsed.is_integer() and abs(parsed) > MAX_SAFE_INTEGER:
        raise StrictJSONError("unsafe_integer", f"integer-valued number outside interoperable range: {value}")
    return parsed


def _constant(value: str) -> None:
    raise StrictJSONError("nonfinite_number", f"non-finite number: {value}")


def _check_unicode(value: Any) -> None:
    if isinstance(value, str):
        if any(0xD800 <= ord(character) <= 0xDFFF for character in value):
            raise StrictJSONError("invalid_unicode", "lone UTF-16 surrogate")
    elif isinstance(value, list):
        for item in value:
            _check_unicode(item)
    elif isinstance(value, dict):
        for key, item in value.items():
            _check_unicode(key)
            _check_unicode(item)


def strict_load_bytes(data: bytes) -> Any:
    try:
        text = data.decode("utf-8", errors="strict")
    except UnicodeDecodeError as error:
        raise StrictJSONError("invalid_utf8", str(error)) from error
    try:
        value = json.loads(
            text,
            object_pairs_hook=_object,
            parse_int=_integer,
            parse_float=_number,
            parse_constant=_constant,
        )
    except StrictJSONError:
        raise
    except json.JSONDecodeError as error:
        raise StrictJSONError("invalid_json", str(error)) from error
    _check_unicode(value)
    return value


def strict_load(relative_path: str) -> Any:
    return strict_load_bytes((ROOT / relative_path).read_bytes())


def _jcs_number(value: int | float) -> str:
    if isinstance(value, int):
        if abs(value) > MAX_SAFE_INTEGER:
            raise StrictJSONError("unsafe_integer", str(value))
        return str(value)
    if not math.isfinite(value):
        raise StrictJSONError("nonfinite_number", repr(value))
    if value == 0:
        return "0"

    sign = "-" if value < 0 else ""
    rendered = repr(abs(value)).lower()
    mantissa, marker, exponent_text = rendered.partition("e")
    exponent = int(exponent_text) if marker else 0
    integer, dot, fraction = mantissa.partition(".")
    digits = (integer + fraction).lstrip("0") or "0"
    scale = exponent - (len(fraction) if dot else 0)
    while len(digits) > 1 and digits.endswith("0"):
        digits = digits[:-1]
        scale += 1

    decimal_position = len(digits) + scale
    if -6 < decimal_position <= 0:
        return sign + "0." + "0" * (-decimal_position) + digits
    if 0 < decimal_position <= 21:
        if decimal_position < len(digits):
            body = digits[:decimal_position] + "." + digits[decimal_position:]
        else:
            body = digits + "0" * (decimal_position - len(digits))
        return sign + body

    exponent = decimal_position - 1
    coefficient = digits[0]
    if len(digits) > 1:
        coefficient += "." + digits[1:]
    return sign + coefficient + "e" + ("+" if exponent >= 0 else "") + str(exponent)


def jcs(value: Any) -> bytes:
    def encode(item: Any) -> str:
        if item is None:
            return "null"
        if item is True:
            return "true"
        if item is False:
            return "false"
        if isinstance(item, (int, float)) and not isinstance(item, bool):
            return _jcs_number(item)
        if isinstance(item, str):
            _check_unicode(item)
            return json.dumps(item, ensure_ascii=False, allow_nan=False)
        if isinstance(item, list):
            return "[" + ",".join(encode(value) for value in item) + "]"
        if isinstance(item, dict):
            ordered = sorted(item, key=lambda key: key.encode("utf-16-be"))
            return "{" + ",".join(encode(key) + ":" + encode(item[key]) for key in ordered) + "}"
        raise TypeError(f"unsupported JSON value: {type(item).__name__}")

    return encode(value).encode("utf-8")


def canonical_digest(value: Any) -> str:
    return "sha256:" + hashlib.sha256(jcs(value)).hexdigest()


def raw_digest(value: bytes) -> str:
    return "sha256:" + hashlib.sha256(value).hexdigest()


def schema_mutation(name: str | None, value: Any) -> Any:
    mutated = copy.deepcopy(value)
    if not name:
        return mutated
    if name == "invalid_plan_date":
        mutated["created_at"] = "not-a-date"
    elif name == "plan_id_trailing_newline":
        mutated["delivery_id"] += "\n"
    elif name == "lowercase_plan_date":
        mutated["created_at"] = "2026-07-19t00:00:00z"
    elif name == "leap_second_plan_date":
        mutated["created_at"] = "1990-12-31T23:59:60Z"
    elif name == "invalid_plan_path":
        mutated["work"][0]["scope"]["include"] = ["src/./internal"]
    elif name == "plan_path_trailing_newline":
        mutated["work"][0]["scope"]["include"] = ["src\n"]
    elif name == "invalid_candidate_oid":
        mutated["candidate"]["commit"] = "b" * 41
    elif name == "candidate_oid_trailing_newline":
        mutated["candidate"]["commit"] += "\n"
    elif name == "pass_empty_evidence":
        mutated["acceptance_results"][0]["evidence_ids"] = []
    elif name == "fail_authority_only":
        mutated["outcome"] = "FAIL"
        mutated["findings"] = [{
            "id": "F-authority",
            "kind": "authority",
            "principle": "B1",
            "severity": "blocking",
            "summary": "Authority is insufficient.",
            "acceptance_ids": [],
            "evidence_ids": [],
        }]
    elif name == "inconclusive_with_authority":
        mutated["outcome"] = "INCONCLUSIVE"
        mutated["findings"] = [
            {
                "id": "F-environment",
                "kind": "environment",
                "principle": "B4",
                "severity": "blocking",
                "summary": "The verifier environment failed.",
                "acceptance_ids": [],
                "evidence_ids": [],
            },
            {
                "id": "F-authority",
                "kind": "authority",
                "principle": "B1",
                "severity": "blocking",
                "summary": "Authority is insufficient.",
                "acceptance_ids": [],
                "evidence_ids": [],
            },
        ]
    elif name == "board_integrated_waiting":
        mutated["work"][0]["state"] = "waiting"
        mutated["work"][0]["next_action"] = "wait"
    elif name == "board_attention":
        mutated["state"] = "attention"
        mutated["work"] = [{
            "id": mutated["work"][0]["id"],
            "state": "attention",
            "attempt": 0,
            "next_action": "replan",
            "attention": "The approved authority is insufficient.",
        }]
    elif name == "board_reviewable":
        row = mutated["work"][0]
        mutated["state"] = "active"
        row["state"] = "reviewable"
        row["next_action"] = "verify"
        for key in ("verdict_id", "verdict_digest", "verdict"):
            row.pop(key)
    elif name == "board_retry_without_verdict":
        row = mutated["work"][0]
        mutated["state"] = "attention"
        row["state"] = "retry"
        row["next_action"] = "retry_verification"
        for key in ("verdict_id", "verdict_digest", "verdict"):
            row.pop(key)
    elif name == "whitespace_summary":
        mutated["summary"] = " \t "
    elif name == "invalid_branch_ref":
        mutated["target"]["ref"] = "refs/heads/a..b"
    elif name == "branch_ref_newline":
        mutated["base"]["ref"] = "refs/heads/main\n"
    elif name == "whitespace_authority_ref":
        mutated["authority"]["ref"] = " \t "
    elif name == "nul_changed_path":
        mutated["changed_paths"][0] = "src/\u0000server.go"
    elif name == "observed_git_metachar_path":
        mutated["changed_paths"] = ["src/[id]?\\name.ts"]
    elif name == "empty_changed_paths":
        mutated["changed_paths"] = []
    elif name == "missing_evidence_media_type":
        mutated["evidence"][0]["artifact"].pop("media_type")
    elif name == "missing_dispatch_media_type":
        mutated["review"]["dispatch_receipt"].pop("media_type")
    elif name == "missing_checks":
        mutated["checks"] = []
    elif name == "uppercase_evidence_media_type":
        mutated["evidence"][0]["artifact"]["media_type"] = "Application/JSON"
    elif name == "invalid_policy_pack_id":
        mutated["packs"] = [{
            "id": "security",
            "definition": {
                "ref": "examples/authority-source.json",
                "media_type": "application/json",
                "digest": ZERO_DIGEST,
            },
        }]
    elif name == "policy_pack_trailing_newline":
        mutated["packs"][0]["id"] += "\n"
    elif name == "verdict_digest_trailing_newline":
        mutated["submission_digest"] += "\n"
    elif name == "board_id_trailing_newline":
        mutated["delivery_id"] += "\n"
    elif name == "control_id_trailing_newline":
        mutated["effect_id"] += "\n"
    elif name == "invalid_policy_media_type":
        mutated["packs"] = [{
            "id": "security@1",
            "definition": {
                "ref": "examples/authority-source.json",
                "media_type": "text/plain",
                "digest": ZERO_DIGEST,
            },
        }]
    elif name == "missing_policy_checks":
        mutated.pop("checks")
    elif name == "wrong_control_receipt_kind":
        mutated["kind"] = "unknown"
    else:
        raise KeyError(f"unknown schema mutation: {name}")
    return mutated


def _artifact_refs(bundle: dict[str, Any]) -> list[str]:
    submission = bundle["submission"]
    verdict = bundle["verdict"]
    refs = [submission["authority_receipt"]["ref"], verdict["review"]["dispatch_receipt"]["ref"]]
    refs.extend(check["receipt"]["ref"] for check in submission["checks"])
    refs.extend(evidence["artifact"]["ref"] for evidence in submission["evidence"])
    if "integration_receipt" in bundle:
        refs.append(bundle["integration_receipt"]["ref"])
    return refs


def _resolve_local_bytes(reference: str) -> bytes:
    if not isinstance(reference, str):
        raise StrictJSONError("ref_unresolved", repr(reference))
    path = (ROOT / reference).resolve()
    if ROOT.resolve() not in path.parents:
        raise StrictJSONError("ref_outside_root", reference)
    try:
        return path.read_bytes()
    except OSError as error:
        raise StrictJSONError("ref_unresolved", reference) from error


def _resolve_local_ref(reference: str) -> Any:
    return strict_load_bytes(_resolve_local_bytes(reference))


def load_bundle() -> dict[str, Any]:
    plan = strict_load("examples/standard-plan.json")
    policy = _resolve_local_ref(plan["assurance_policy"]["ref"])
    authority_source = _resolve_local_ref(plan["authority"]["ref"])
    bundle = {
        "plan": plan,
        "submission": strict_load("examples/standard-submission.json"),
        "verdict": strict_load("examples/pass-verdict.json"),
        "board": strict_load("examples/delivery-board.json"),
        "policy": policy,
        "authority_source": authority_source,
        "authority_source_snapshot": copy.deepcopy(authority_source),
        "resolved_refs": {
            plan["assurance_policy"]["ref"]: policy,
            plan["authority"]["ref"]: authority_source,
        },
        "artifacts": {},
        "record_history": {"submission": {}, "verdict": {}},
        "integration_receipt": {
            "ref": "examples/artifacts/integration/target-cas.json",
            "media_type": "application/json",
            "digest": "sha256:1dd439e16d2e382bc3e9b8054ff9f5611f3cacde29fa60f405df43c34f21e1c1",
        },
    }
    for reference in _artifact_refs(bundle):
        path = (ROOT / reference).resolve()
        if ROOT.resolve() not in path.parents:
            raise StrictJSONError("artifact_ref_outside_root", reference)
        bundle["artifacts"][reference] = path.read_bytes()
    bundle["submissions"] = [bundle["submission"]]
    bundle["verdicts"] = [bundle["verdict"]]
    bundle["integration_receipts"] = {
        bundle["submission"]["work_id"]: bundle["integration_receipt"]
    }
    bundle["observed_target"] = bundle["submission"]["candidate"]["commit"]
    bundle["target_ancestors"] = {
        bundle["submission"]["base"]["commit"],
        bundle["submission"]["candidate"]["commit"],
    }
    return bundle


def _pretty_json(value: Any) -> bytes:
    return (json.dumps(value, ensure_ascii=False, indent=2) + "\n").encode("utf-8")


def _update_envelopes(value: Any, reference: str, digest: str) -> None:
    if isinstance(value, dict):
        if value.get("ref") == reference and "digest" in value:
            value["digest"] = digest
        for item in value.values():
            _update_envelopes(item, reference, digest)
    elif isinstance(value, list):
        for item in value:
            _update_envelopes(item, reference, digest)


def replace_artifact(bundle: dict[str, Any], reference: str, value: Any) -> None:
    encoded = _pretty_json(value)
    bundle["artifacts"][reference] = encoded
    digest = raw_digest(encoded)
    for record in (
        bundle["plan"], bundle["submission"], bundle["verdict"], bundle["board"],
        bundle.get("integration_receipt", {}),
    ):
        _update_envelopes(record, reference, digest)


def rebind_integration_receipt(bundle: dict[str, Any]) -> None:
    envelope = bundle.get("integration_receipt")
    if envelope is None:
        return
    receipt = strict_load_bytes(bundle["artifacts"][envelope["ref"]])
    approval = strict_load_bytes(
        bundle["artifacts"][bundle["submission"]["authority_receipt"]["ref"]]
    )
    receipt["repository"] = bundle["plan"]["target"]["repository"]
    receipt["target_ref"] = bundle["plan"]["target"]["ref"]
    receipt["expected_target"] = bundle["submission"]["base"]["commit"]
    receipt["candidate_commit"] = bundle["submission"]["candidate"]["commit"]
    receipt["submission_digest"] = canonical_digest(bundle["submission"])
    receipt["verdict_digest"] = canonical_digest(bundle["verdict"])
    receipt["authority_receipt_digest"] = bundle["submission"]["authority_receipt"]["digest"]
    receipt["authority_source_ref"] = approval["source_ref"]
    receipt["authority_source_digest"] = approval["source_digest"]
    receipt["observed_target"] = bundle["submission"]["candidate"]["commit"]
    replace_artifact(bundle, envelope["ref"], receipt)


def rebind_verdict_board(bundle: dict[str, Any]) -> None:
    verdict_digest = canonical_digest(bundle["verdict"])
    for row in bundle["board"]["work"]:
        if row["id"] == bundle["verdict"]["work_id"]:
            row["verdict_digest"] = verdict_digest
    rebind_integration_receipt(bundle)


def rebind_submission_chain(bundle: dict[str, Any]) -> None:
    submission_digest = canonical_digest(bundle["submission"])
    bundle["verdict"]["submission_digest"] = submission_digest
    for row in bundle["board"]["work"]:
        if row["id"] == bundle["submission"]["work_id"]:
            row["submission_digest"] = submission_digest
    dispatch_ref = bundle["verdict"]["review"]["dispatch_receipt"]["ref"]
    dispatch = strict_load_bytes(bundle["artifacts"][dispatch_ref])
    dispatch["submission_digest"] = submission_digest
    replace_artifact(bundle, dispatch_ref, dispatch)
    rebind_verdict_board(bundle)


def rebind_plan_chain(bundle: dict[str, Any], *, update_receipt_source_ref: bool = False) -> None:
    plan = bundle["plan"]
    submission = bundle["submission"]
    receipt_ref = submission["authority_receipt"]["ref"]
    receipt = strict_load_bytes(bundle["artifacts"][receipt_ref])
    receipt["plan_digest"] = canonical_digest(plan)
    receipt["authority_digest"] = canonical_digest(plan["authority"])
    receipt["grants"] = copy.deepcopy(plan["authority"]["grants"])
    if update_receipt_source_ref:
        receipt["source_ref"] = plan["authority"]["ref"]
    replace_artifact(bundle, receipt_ref, receipt)
    submission["plan_digest"] = canonical_digest(plan)
    submission["assurance"]["policy_ref"] = plan["assurance_policy"]["ref"]
    submission["assurance"]["policy_digest"] = plan["assurance_policy"]["digest"]
    board = bundle["board"]
    board["plan_digest"] = canonical_digest(plan)
    rebind_submission_chain(bundle)


def replace_policy(bundle: dict[str, Any], policy: dict[str, Any]) -> None:
    reference = bundle["plan"]["assurance_policy"]["ref"]
    bundle["policy"] = policy
    bundle["resolved_refs"][reference] = policy
    bundle["plan"]["assurance_policy"]["digest"] = canonical_digest(policy)
    rebind_plan_chain(bundle)


def rebind_authority_source(bundle: dict[str, Any]) -> None:
    source = bundle["authority_source"]
    bundle["authority_source_snapshot"] = copy.deepcopy(source)
    reference = bundle["submission"]["authority_receipt"]["ref"]
    receipt = strict_load_bytes(bundle["artifacts"][reference])
    receipt["source_digest"] = canonical_digest(source)
    replace_artifact(bundle, reference, receipt)
    rebind_submission_chain(bundle)


def remove_integration(bundle: dict[str, Any], work_id: str) -> None:
    bundle.get("integration_receipts", {}).pop(work_id, None)
    if bundle.get("submission", {}).get("work_id") == work_id:
        bundle.pop("integration_receipt", None)


def make_assured(bundle: dict[str, Any]) -> None:
    plan = bundle["plan"]
    submission = bundle["submission"]
    verdict = bundle["verdict"]
    contract = plan["work"][0]
    contract["assurance"] = {"profile": "assured", "packs": ["security@1"]}
    submission["contract_digest"] = canonical_digest(contract)
    submission["assurance"]["profile"] = "assured"
    submission["assurance"]["packs"] = ["security@1"]
    submission["evidence"][0]["pack_ids"] = ["security@1"]
    verdict["assurance_results"] = [{
        "pack": "security@1",
        "outcome": "pass",
        "evidence_ids": [submission["evidence"][0]["id"]],
        "summary": "The selected security pack is supported by the bound evidence.",
    }]
    rebind_plan_chain(bundle)


def make_two_work_serial(bundle: dict[str, Any]) -> None:
    plan = bundle["plan"]
    first_submission = bundle["submission"]
    first_verdict = bundle["verdict"]
    board = bundle["board"]

    second_contract = copy.deepcopy(plan["work"][0])
    second_contract["id"] = "health-header"
    second_contract["outcome"] = "The health response includes its declared media type."
    second_contract["acceptance"][0]["id"] = "AC2"
    second_contract["acceptance"][0]["criterion"] = (
        "GET /health returns Content-Type application/json."
    )
    second_contract["depends_on"] = [plan["work"][0]["id"]]
    plan["work"].append(second_contract)
    rebind_plan_chain(bundle)

    second_submission = copy.deepcopy(first_submission)
    second_submission["submission_id"] = "example-release.health-header.1"
    second_submission["work_id"] = second_contract["id"]
    second_submission["created_at"] = "2026-07-19T00:10:00Z"
    second_submission["contract_digest"] = canonical_digest(second_contract)
    second_submission["builder"] = {
        "run_id": "builder-run-2",
        "agent": "codex",
        "started_at": "2026-07-19T00:08:00Z",
        "completed_at": "2026-07-19T00:09:00Z",
    }
    second_submission["base"]["commit"] = first_submission["candidate"]["commit"]
    second_submission["candidate"]["commit"] = "e" * 40
    second_submission["candidate"]["tree"] = "f" * 40
    second_submission["checks"][0]["run_id"] = "check-run-2"
    second_submission["checks"][0]["candidate_tree"] = "f" * 40
    second_submission["checks"][0]["started_at"] = "2026-07-19T00:09:05Z"
    second_submission["checks"][0]["completed_at"] = "2026-07-19T00:09:20Z"
    evidence = second_submission["evidence"][0]
    evidence["id"] = "health-header-smoke"
    evidence["acceptance_ids"] = ["AC2"]
    evidence["producer_run_id"] = "check-run-2"
    evidence["candidate_tree"] = "f" * 40
    evidence["captured_at"] = "2026-07-19T00:09:20Z"
    evidence["observed"] = "The assembled service returned the declared media type."

    second_submission_digest = canonical_digest(second_submission)
    second_dispatch = copy.deepcopy(
        strict_load_bytes(
            bundle["artifacts"][first_verdict["review"]["dispatch_receipt"]["ref"]]
        )
    )
    second_dispatch["dispatch_id"] = "verifier-run-2"
    second_dispatch["submission_digest"] = second_submission_digest
    second_dispatch["candidate"] = copy.deepcopy(second_submission["candidate"])
    second_dispatch["workspace"] = "fresh-read-only-materialization-2"
    second_dispatch["created_at"] = "2026-07-19T00:10:55Z"
    second_dispatch_ref = "examples/artifacts/dispatch/verifier-run-2.json"
    second_dispatch_raw = _pretty_json(second_dispatch)
    bundle["artifacts"][second_dispatch_ref] = second_dispatch_raw

    second_verdict = copy.deepcopy(first_verdict)
    second_verdict["verdict_id"] = "verdict-example-release-header-1"
    second_verdict["submission_id"] = second_submission["submission_id"]
    second_verdict["submission_digest"] = second_submission_digest
    second_verdict["work_id"] = second_contract["id"]
    second_verdict["review"]["run_id"] = "verifier-run-2"
    second_verdict["review"]["dispatch_receipt"] = {
        "ref": second_dispatch_ref,
        "media_type": "application/json",
        "digest": raw_digest(second_dispatch_raw),
    }
    second_verdict["review"]["started_at"] = "2026-07-19T00:11:00Z"
    second_verdict["review"]["completed_at"] = "2026-07-19T00:12:00Z"
    second_verdict["acceptance_results"][0]["acceptance_id"] = "AC2"
    second_verdict["acceptance_results"][0]["evidence_ids"] = [evidence["id"]]
    second_verdict_digest = canonical_digest(second_verdict)

    approval = strict_load_bytes(
        bundle["artifacts"][second_submission["authority_receipt"]["ref"]]
    )
    second_integration = {
        "schema_version": "control-receipt-v1",
        "kind": "integration",
        "effect_id": "integrate-example-release-header-1",
        "repository": plan["target"]["repository"],
        "target_ref": plan["target"]["ref"],
        "expected_target": second_submission["base"]["commit"],
        "candidate_commit": second_submission["candidate"]["commit"],
        "submission_digest": second_submission_digest,
        "verdict_digest": second_verdict_digest,
        "authority_receipt_digest": second_submission["authority_receipt"]["digest"],
        "authority_source_ref": approval["source_ref"],
        "authority_source_digest": approval["source_digest"],
        "authorized_at": "2026-07-19T00:12:10Z",
        "started_at": "2026-07-19T00:12:11Z",
        "completed_at": "2026-07-19T00:12:12Z",
        "result": "updated",
        "observed_target": second_submission["candidate"]["commit"],
    }
    second_integration_ref = "examples/artifacts/integration/target-cas-2.json"
    second_integration_raw = _pretty_json(second_integration)
    bundle["artifacts"][second_integration_ref] = second_integration_raw
    second_integration_envelope = {
        "ref": second_integration_ref,
        "media_type": "application/json",
        "digest": raw_digest(second_integration_raw),
    }

    bundle["submissions"].append(second_submission)
    bundle["verdicts"].append(second_verdict)
    bundle["integration_receipts"][second_contract["id"]] = second_integration_envelope
    bundle["observed_target"] = second_submission["candidate"]["commit"]
    bundle["target_ancestors"] = {
        first_submission["base"]["commit"],
        first_submission["candidate"]["commit"],
        second_submission["candidate"]["commit"],
    }
    board["work"].append({
        "id": second_contract["id"],
        "state": "integrated",
        "attempt": second_submission["attempt"],
        "submission_id": second_submission["submission_id"],
        "submission_digest": second_submission_digest,
        "candidate_commit": second_submission["candidate"]["commit"],
        "verdict_id": second_verdict["verdict_id"],
        "verdict_digest": second_verdict_digest,
        "verdict": "PASS",
        "next_action": "none",
    })


def rebind_historical_pair(
    bundle: dict[str, Any], submission: dict[str, Any], verdict: dict[str, Any]
) -> None:
    submission_digest = canonical_digest(submission)
    dispatch_envelope = verdict["review"]["dispatch_receipt"]
    dispatch = strict_load_bytes(bundle["artifacts"][dispatch_envelope["ref"]])
    dispatch["dispatch_id"] = verdict["review"]["run_id"]
    dispatch["submission_digest"] = submission_digest
    dispatch["candidate"] = copy.deepcopy(submission["candidate"])
    dispatch_raw = _pretty_json(dispatch)
    bundle["artifacts"][dispatch_envelope["ref"]] = dispatch_raw
    dispatch_envelope["digest"] = raw_digest(dispatch_raw)

    verdict["submission_digest"] = submission_digest
    verdict_digest = canonical_digest(verdict)
    integration_envelope = bundle.get("integration_receipts", {}).get(
        submission["work_id"]
    )
    if integration_envelope is not None:
        integration = strict_load_bytes(
            bundle["artifacts"][integration_envelope["ref"]]
        )
        integration["expected_target"] = submission["base"]["commit"]
        integration["candidate_commit"] = submission["candidate"]["commit"]
        integration["submission_digest"] = submission_digest
        integration["verdict_digest"] = verdict_digest
        integration["authority_receipt_digest"] = submission["authority_receipt"][
            "digest"
        ]
        integration["observed_target"] = submission["candidate"]["commit"]
        integration_raw = _pretty_json(integration)
        bundle["artifacts"][integration_envelope["ref"]] = integration_raw
        integration_envelope["digest"] = raw_digest(integration_raw)

    for row in bundle["board"]["work"]:
        if row["id"] != submission["work_id"]:
            continue
        row["attempt"] = submission["attempt"]
        row["submission_id"] = submission["submission_id"]
        row["submission_digest"] = submission_digest
        row["candidate_commit"] = submission["candidate"]["commit"]
        row["verdict_id"] = verdict["verdict_id"]
        row["verdict_digest"] = verdict_digest
        row["verdict"] = verdict["outcome"]


def mutate_bundle(name: str) -> dict[str, Any]:
    bundle = load_bundle()
    if name == "none":
        return bundle
    plan = bundle["plan"]
    submission = bundle["submission"]
    verdict = bundle["verdict"]
    board = bundle["board"]

    if name == "duplicate_work":
        plan["work"].append(copy.deepcopy(plan["work"][0]))
    elif name == "duplicate_acceptance":
        plan["work"][0]["acceptance"].append(copy.deepcopy(plan["work"][0]["acceptance"][0]))
    elif name == "unknown_dependency":
        plan["work"][0]["depends_on"].append("unknown-work")
    elif name == "dependency_cycle":
        plan["work"][0]["depends_on"].append(plan["work"][0]["id"])
    elif name == "plan_policy_digest":
        plan["assurance_policy"]["digest"] = ZERO_DIGEST
    elif name == "plan_policy_ref":
        plan["assurance_policy"]["ref"] = "examples/does-not-exist.json"
        rebind_plan_chain(bundle)
    elif name == "policy_wrong_shape":
        reference = plan["authority"]["ref"]
        plan["assurance_policy"] = {
            "ref": reference,
            "digest": canonical_digest(bundle["resolved_refs"][reference]),
        }
        rebind_plan_chain(bundle)
    elif name == "assured_valid":
        make_assured(bundle)
    elif name == "assured_missing_result":
        make_assured(bundle)
        verdict["assurance_results"] = []
        rebind_verdict_board(bundle)
    elif name == "unselected_policy_definition_unavailable":
        policy = copy.deepcopy(bundle["policy"])
        policy["packs"][0]["definition"]["ref"] = "examples/does-not-exist.json"
        policy["packs"][0]["definition"]["digest"] = ZERO_DIGEST
        replace_policy(bundle, policy)
    elif name == "unknown_policy_pack":
        contract = plan["work"][0]
        contract["assurance"] = {"profile": "assured", "packs": ["privacy@1"]}
        submission["contract_digest"] = canonical_digest(contract)
        submission["assurance"]["profile"] = "assured"
        submission["assurance"]["packs"] = ["privacy@1"]
        submission["evidence"][0]["pack_ids"] = ["privacy@1"]
        verdict["assurance_results"] = [{
            "pack": "privacy@1",
            "outcome": "pass",
            "evidence_ids": [submission["evidence"][0]["id"]],
            "summary": "The requested pack appears satisfied.",
        }]
        rebind_plan_chain(bundle)
    elif name == "duplicate_policy_pack":
        policy = copy.deepcopy(bundle["policy"])
        policy["packs"] = [
            {
                "id": "security@1",
                "definition": {
                    "ref": "examples/authority-source.json",
                    "media_type": "application/json",
                    "digest": raw_digest(_resolve_local_bytes("examples/authority-source.json")),
                },
            },
            {
                "id": "security@1",
                "definition": {
                    "ref": "examples/assurance-policy.json",
                    "media_type": "application/json",
                    "digest": raw_digest(_resolve_local_bytes("examples/assurance-policy.json")),
                },
            },
        ]
        replace_policy(bundle, policy)
    elif name in {"policy_definition_digest", "policy_definition_invalid_json"}:
        reference = (
            "examples/authority-source.json"
            if name == "policy_definition_digest"
            else "conformance/fixtures/raw-invalid-duplicate-key.json"
        )
        digest = ZERO_DIGEST if name == "policy_definition_digest" else raw_digest(_resolve_local_bytes(reference))
        policy = copy.deepcopy(bundle["policy"])
        policy["checks"][0]["definition"] = {
            "ref": reference,
            "media_type": "application/json",
            "digest": digest,
        }
        replace_policy(bundle, policy)
    elif name == "authority_artifact_bytes":
        reference = submission["authority_receipt"]["ref"]
        bundle["artifacts"][reference] += b"\n"
    elif name == "authority_media_type":
        submission["authority_receipt"]["media_type"] = "text/plain"
        rebind_submission_chain(bundle)
    elif name in {
        "authority_plan_digest", "authority_object_digest", "authority_source_digest",
        "authority_grants", "authority_target",
    }:
        reference = submission["authority_receipt"]["ref"]
        receipt = strict_load_bytes(bundle["artifacts"][reference])
        if name == "authority_plan_digest":
            receipt["plan_digest"] = ZERO_DIGEST
        elif name == "authority_object_digest":
            receipt["authority_digest"] = ZERO_DIGEST
        elif name == "authority_source_digest":
            receipt["source_digest"] = ZERO_DIGEST
        elif name == "authority_grants":
            receipt["grants"] = receipt["grants"][:-1]
        else:
            receipt["target_ref"] = "refs/heads/other"
        replace_artifact(bundle, reference, receipt)
        rebind_submission_chain(bundle)
    elif name == "authority_revoked":
        bundle["authority_source"]["status"] = "revoked"
    elif name == "authority_revoked_before_integration":
        bundle["authority_source"]["status"] = "revoked"
        remove_integration(bundle, submission["work_id"])
        board["state"] = "attention"
        board["attention"] = ["Authority was revoked before integration."]
        board["work"][0]["state"] = "ready_to_integrate"
        board["work"][0]["next_action"] = "integrate"
        bundle["observed_target"] = submission["base"]["commit"]
        bundle["target_ancestors"] = {submission["base"]["commit"]}
    elif name == "authority_expired_after_integration":
        bundle["authority_source"]["valid_until"] = "2026-07-19T00:07:30Z"
        rebind_authority_source(bundle)
    elif name in {
        "authority_expired_before_builder",
        "authority_expired_before_pass",
        "authority_expired_before_integration",
    }:
        bundle["authority_source"]["valid_until"] = {
            "authority_expired_before_builder": "2026-07-19T00:00:45Z",
            "authority_expired_before_pass": "2026-07-19T00:06:30Z",
            "authority_expired_before_integration": "2026-07-19T00:07:05Z",
        }[name]
        rebind_authority_source(bundle)
    elif name == "authority_source_ref":
        plan["authority"]["ref"] = "examples/does-not-exist.json"
        rebind_plan_chain(bundle, update_receipt_source_ref=True)
        bundle["integration_receipts"].pop(submission["work_id"], None)
        bundle.pop("integration_receipt", None)
        board["state"] = "ready_to_integrate"
        board["work"][0]["state"] = "ready_to_integrate"
        board["work"][0]["next_action"] = "integrate"
        bundle["observed_target"] = submission["base"]["commit"]
        bundle["target_ancestors"] = {submission["base"]["commit"]}
    elif name == "plan_integration_target":
        for grant in plan["authority"]["grants"]:
            if grant["action"] == "integrate":
                grant["target"]["ref"] = "refs/heads/other"
    elif name == "submission_plan_digest":
        submission["plan_digest"] = ZERO_DIGEST
    elif name == "contract_digest":
        submission["contract_digest"] = ZERO_DIGEST
    elif name == "repository":
        submission["candidate"]["repository"] = "local:other"
    elif name == "base_ref":
        submission["base"]["ref"] = "refs/heads/other"
    elif name == "out_of_scope_path":
        submission["changed_paths"].append("README.md")
    elif name == "duplicate_check":
        submission["checks"].append(copy.deepcopy(submission["checks"][0]))
    elif name == "check_tree":
        submission["checks"][0]["candidate_tree"] = BAD_OID
    elif name == "builder_stamped_check":
        submission["checks"][0]["run_id"] = submission["builder"]["run_id"]
        submission["evidence"][0]["producer_run_id"] = submission["builder"]["run_id"]
        rebind_submission_chain(bundle)
    elif name == "check_artifact_bytes":
        reference = submission["checks"][0]["receipt"]["ref"]
        bundle["artifacts"][reference] += b"drift"
    elif name == "missing_policy_check":
        submission["checks"] = []
        rebind_submission_chain(bundle)
    elif name == "duplicate_evidence":
        submission["evidence"].append(copy.deepcopy(submission["evidence"][0]))
    elif name == "evidence_tree":
        submission["evidence"][0]["candidate_tree"] = BAD_OID
    elif name == "evidence_artifact_bytes":
        reference = submission["evidence"][0]["artifact"]["ref"]
        bundle["artifacts"][reference] += b"drift"
    elif name == "invalid_structured_evidence":
        reference = submission["evidence"][0]["artifact"]["ref"]
        encoded = b'{"duplicate":1,"duplicate":2}\n'
        bundle["artifacts"][reference] = encoded
        for record in (plan, submission, verdict, board):
            _update_envelopes(record, reference, raw_digest(encoded))
        rebind_submission_chain(bundle)
    elif name == "invalid_json_suffix_evidence":
        reference = submission["evidence"][0]["artifact"]["ref"]
        submission["evidence"][0]["artifact"]["media_type"] = "application/problem+json"
        encoded = b'{"duplicate":1,"duplicate":2}\n'
        bundle["artifacts"][reference] = encoded
        for record in (plan, submission, verdict, board):
            _update_envelopes(record, reference, raw_digest(encoded))
        rebind_submission_chain(bundle)
    elif name == "evidence_boundary":
        submission["evidence"][0]["boundary"] = "component"
    elif name == "evidence_mocks":
        submission["evidence"][0]["uses_mocks"] = True
    elif name == "evidence_acceptance":
        submission["evidence"][0]["acceptance_ids"] = ["unknown-acceptance"]
    elif name == "same_runner":
        verdict["review"]["run_id"] = submission["builder"]["run_id"]
    elif name == "dispatch_artifact_bytes":
        reference = verdict["review"]["dispatch_receipt"]["ref"]
        bundle["artifacts"][reference] += b"\n"
    elif name == "dispatch_media_type":
        verdict["review"]["dispatch_receipt"]["media_type"] = "text/plain"
        rebind_verdict_board(bundle)
    elif name in {"dispatch_submission_digest", "dispatch_remote"}:
        reference = verdict["review"]["dispatch_receipt"]["ref"]
        dispatch = strict_load_bytes(bundle["artifacts"][reference])
        if name == "dispatch_submission_digest":
            dispatch["submission_digest"] = ZERO_DIGEST
        else:
            dispatch["remotes_present"] = True
        replace_artifact(bundle, reference, dispatch)
        rebind_verdict_board(bundle)
    elif name == "verdict_submission_digest":
        verdict["submission_digest"] = ZERO_DIGEST
    elif name == "duplicate_acceptance_result":
        verdict["acceptance_results"].append(copy.deepcopy(verdict["acceptance_results"][0]))
    elif name == "missing_acceptance_result":
        verdict["acceptance_results"] = []
    elif name == "verdict_evidence":
        verdict["acceptance_results"][0]["evidence_ids"] = ["unknown-evidence"]
    elif name == "duplicate_finding":
        finding = {
            "id": "F-duplicate",
            "kind": "implementation",
            "principle": "B3",
            "severity": "non_blocking",
            "summary": "A duplicated advisory finding.",
            "acceptance_ids": ["AC1"],
            "evidence_ids": ["health-smoke"],
        }
        verdict["findings"] = [finding, copy.deepcopy(finding)]
    elif name == "board_duplicate_work":
        board["work"].append(copy.deepcopy(board["work"][0]))
    elif name == "board_missing_work":
        board["work"] = []
    elif name == "board_submission_digest":
        board["work"][0]["submission_digest"] = ZERO_DIGEST
    elif name == "board_verdict_digest":
        board["work"][0]["verdict_digest"] = ZERO_DIGEST
    elif name == "board_verified_with_grant":
        board["state"] = "verified"
        board["work"][0]["state"] = "verified"
        board["work"][0]["next_action"] = "replan"
        bundle["observed_target"] = submission["base"]["commit"]
    elif name == "board_ready_without_grant":
        plan["authority"]["grants"] = [
            grant for grant in plan["authority"]["grants"] if grant["action"] != "integrate"
        ]
        board["state"] = "ready_to_integrate"
        board["work"][0]["state"] = "ready_to_integrate"
        board["work"][0]["next_action"] = "integrate"
        rebind_plan_chain(bundle)
        bundle["integration_receipts"].pop(submission["work_id"], None)
        bundle.pop("integration_receipt", None)
        bundle["observed_target"] = submission["base"]["commit"]
        bundle["target_ancestors"] = {submission["base"]["commit"]}
    elif name == "board_not_integrated":
        board["state"] = "verified"
        board["work"][0]["state"] = "verified"
        board["work"][0]["next_action"] = "replan"
    elif name == "board_integrated_target_base":
        bundle["observed_target"] = submission["base"]["commit"]
        bundle["target_ancestors"] = {submission["base"]["commit"]}
    elif name == "board_planned_ready":
        board["state"] = "planned"
        board["work"][0]["state"] = "ready"
        board["work"][0]["next_action"] = "build"
        bundle["observed_target"] = submission["base"]["commit"]
        bundle["target_ancestors"] = {submission["base"]["commit"]}
    elif name == "timestamp_order":
        submission["builder"]["completed_at"] = "2026-07-19T00:00:59Z"
    elif name == "builder_before_approval":
        submission["builder"]["started_at"] = "2026-07-19T00:00:10Z"
        rebind_submission_chain(bundle)
    elif name == "check_before_builder":
        submission["checks"][0]["started_at"] = "2026-07-19T00:04:00Z"
        submission["checks"][0]["completed_at"] = "2026-07-19T00:04:20Z"
        submission["evidence"][0]["captured_at"] = "2026-07-19T00:04:20Z"
        rebind_submission_chain(bundle)
    elif name == "evidence_before_producer":
        submission["evidence"][0]["captured_at"] = "2026-07-19T00:04:00Z"
        rebind_submission_chain(bundle)
    elif name == "evidence_builder_producer":
        submission["evidence"][0]["producer_run_id"] = submission["builder"]["run_id"]
        submission["evidence"][0]["captured_at"] = submission["builder"]["completed_at"]
        rebind_submission_chain(bundle)
    elif name == "fractional_timestamp_order":
        submission["builder"]["started_at"] = "2026-07-19T00:01:00.9Z"
        submission["builder"]["completed_at"] = "2026-07-19T00:01:00.1Z"
        rebind_submission_chain(bundle)
    elif name == "submission_id_reuse":
        bundle["record_history"]["submission"][submission["submission_id"]] = ZERO_DIGEST
    elif name == "verdict_id_reuse":
        bundle["record_history"]["verdict"][verdict["verdict_id"]] = ZERO_DIGEST
    elif name == "duplicate_verifier_run":
        duplicate_verifier = copy.deepcopy(verdict)
        duplicate_verifier["verdict_id"] = "verdict-example-release-health-duplicate-run"
        bundle["verdicts"].append(duplicate_verifier)
    elif name == "inconclusive_then_pass":
        inconclusive = copy.deepcopy(verdict)
        inconclusive["verdict_id"] = "verdict-example-release-health-inconclusive"
        inconclusive["outcome"] = "INCONCLUSIVE"
        inconclusive["review"]["run_id"] = "verifier-run-0"
        inconclusive["review"]["started_at"] = "2026-07-19T00:05:15Z"
        inconclusive["review"]["completed_at"] = "2026-07-19T00:05:30Z"
        inconclusive["acceptance_results"][0]["outcome"] = "inconclusive"
        inconclusive["findings"] = [{
            "id": "F-verifier-environment",
            "kind": "environment",
            "principle": "B4",
            "severity": "blocking",
            "summary": "The first verifier environment could not establish truth.",
            "acceptance_ids": ["AC1"],
            "evidence_ids": [submission["evidence"][0]["id"]],
        }]
        dispatch = copy.deepcopy(
            strict_load_bytes(
                bundle["artifacts"][verdict["review"]["dispatch_receipt"]["ref"]]
            )
        )
        dispatch["dispatch_id"] = "verifier-run-0"
        dispatch["workspace"] = "fresh-read-only-materialization-0"
        dispatch["created_at"] = "2026-07-19T00:05:10Z"
        dispatch_ref = "examples/artifacts/dispatch/verifier-run-0.json"
        dispatch_raw = _pretty_json(dispatch)
        bundle["artifacts"][dispatch_ref] = dispatch_raw
        inconclusive["review"]["dispatch_receipt"] = {
            "ref": dispatch_ref,
            "media_type": "application/json",
            "digest": raw_digest(dispatch_raw),
        }
        bundle["verdicts"].insert(0, inconclusive)
    elif name == "repair_then_pass":
        failed_submission = copy.deepcopy(submission)
        failed_verdict = copy.deepcopy(verdict)
        failed_verdict["outcome"] = "FAIL"
        failed_verdict["acceptance_results"][0]["outcome"] = "fail"
        failed_verdict["findings"] = [{
            "id": "F-first-attempt",
            "kind": "implementation",
            "principle": "B3",
            "severity": "blocking",
            "summary": "The first attempt did not satisfy acceptance.",
            "acceptance_ids": ["AC1"],
            "evidence_ids": [failed_submission["evidence"][0]["id"]],
        }]

        submission["submission_id"] = "example-release.health-endpoint.2"
        submission["attempt"] = 2
        submission["created_at"] = "2026-07-19T00:10:00Z"
        submission["builder"] = {
            "run_id": "builder-run-2",
            "agent": "codex",
            "started_at": "2026-07-19T00:08:00Z",
            "completed_at": "2026-07-19T00:09:00Z",
        }
        submission["candidate"]["commit"] = "e" * 40
        submission["candidate"]["tree"] = "f" * 40
        submission["checks"][0]["run_id"] = "check-run-2"
        submission["checks"][0]["candidate_tree"] = "f" * 40
        submission["checks"][0]["started_at"] = "2026-07-19T00:09:05Z"
        submission["checks"][0]["completed_at"] = "2026-07-19T00:09:20Z"
        submission["evidence"][0]["producer_run_id"] = "check-run-2"
        submission["evidence"][0]["candidate_tree"] = "f" * 40
        submission["evidence"][0]["captured_at"] = "2026-07-19T00:09:20Z"

        verdict["verdict_id"] = "verdict-example-release-health-2"
        verdict["submission_id"] = submission["submission_id"]
        verdict["review"]["run_id"] = "verifier-run-2"
        verdict["review"]["started_at"] = "2026-07-19T00:11:00Z"
        verdict["review"]["completed_at"] = "2026-07-19T00:12:00Z"
        current_dispatch = copy.deepcopy(
            strict_load_bytes(
                bundle["artifacts"][failed_verdict["review"]["dispatch_receipt"]["ref"]]
            )
        )
        current_dispatch["dispatch_id"] = "verifier-run-2"
        current_dispatch["workspace"] = "fresh-read-only-materialization-2"
        current_dispatch["created_at"] = "2026-07-19T00:10:55Z"
        current_dispatch_ref = "examples/artifacts/dispatch/verifier-run-2.json"
        current_dispatch_raw = _pretty_json(current_dispatch)
        bundle["artifacts"][current_dispatch_ref] = current_dispatch_raw
        verdict["review"]["dispatch_receipt"] = {
            "ref": current_dispatch_ref,
            "media_type": "application/json",
            "digest": raw_digest(current_dispatch_raw),
        }

        bundle["submissions"] = [failed_submission, submission]
        bundle["verdicts"] = [failed_verdict, verdict]
        rebind_historical_pair(bundle, submission, verdict)
        integration_envelope = bundle["integration_receipts"][submission["work_id"]]
        integration = strict_load_bytes(
            bundle["artifacts"][integration_envelope["ref"]]
        )
        integration["authorized_at"] = "2026-07-19T00:12:10Z"
        integration["started_at"] = "2026-07-19T00:12:11Z"
        integration["completed_at"] = "2026-07-19T00:12:12Z"
        integration_raw = _pretty_json(integration)
        bundle["artifacts"][integration_envelope["ref"]] = integration_raw
        integration_envelope["digest"] = raw_digest(integration_raw)
        bundle["observed_target"] = submission["candidate"]["commit"]
        bundle["target_ancestors"] = {
            submission["base"]["commit"],
            submission["candidate"]["commit"],
        }
    elif name == "duplicate_work_attempt":
        duplicate_attempt = copy.deepcopy(submission)
        duplicate_attempt["submission_id"] = "example-release.health-endpoint.duplicate"
        bundle["submissions"].append(duplicate_attempt)
    elif name == "integration_stale_verdict":
        later_verdict = copy.deepcopy(verdict)
        later_verdict["verdict_id"] = "verdict-example-release-health-2"
        later_verdict["summary"] = "A later fresh review also passed the submission."
        later_verdict["review"]["completed_at"] = "2026-07-19T00:06:45Z"
        later_verdict_digest = canonical_digest(later_verdict)
        bundle["verdicts"].append(later_verdict)
        board["work"][0]["verdict_id"] = later_verdict["verdict_id"]
        board["work"][0]["verdict_digest"] = later_verdict_digest
    elif name == "board_stale_current_verdict":
        later_verdict = copy.deepcopy(verdict)
        later_verdict["verdict_id"] = "verdict-example-release-health-current"
        later_verdict["review"]["run_id"] = "verifier-run-current"
        later_verdict["review"]["started_at"] = "2026-07-19T00:07:01Z"
        later_verdict["review"]["completed_at"] = "2026-07-19T00:07:05Z"
        dispatch = copy.deepcopy(
            strict_load_bytes(
                bundle["artifacts"][verdict["review"]["dispatch_receipt"]["ref"]]
            )
        )
        dispatch["dispatch_id"] = "verifier-run-current"
        dispatch["workspace"] = "fresh-read-only-materialization-current"
        dispatch["created_at"] = "2026-07-19T00:07:00Z"
        dispatch_ref = "examples/artifacts/dispatch/verifier-run-current.json"
        dispatch_raw = _pretty_json(dispatch)
        bundle["artifacts"][dispatch_ref] = dispatch_raw
        later_verdict["review"]["dispatch_receipt"] = {
            "ref": dispatch_ref,
            "media_type": "application/json",
            "digest": raw_digest(dispatch_raw),
        }
        bundle["verdicts"].append(later_verdict)
    elif name == "integration_base_not_reachable":
        submission["base"]["commit"] = "d" * 40
        rebind_submission_chain(bundle)
    elif name == "integration_bound_to_fail":
        verdict["outcome"] = "FAIL"
        verdict["acceptance_results"][0]["outcome"] = "fail"
        verdict["findings"] = [{
            "id": "F-failed-delivery",
            "kind": "implementation",
            "principle": "B3",
            "severity": "blocking",
            "summary": "The implementation does not satisfy acceptance.",
            "acceptance_ids": ["AC1"],
            "evidence_ids": [submission["evidence"][0]["id"]],
        }]
        board["state"] = "active"
        board["work"][0]["state"] = "repair"
        board["work"][0]["verdict"] = "FAIL"
        board["work"][0]["next_action"] = "repair"
        rebind_verdict_board(bundle)
    elif name == "no_change_candidate":
        submission["candidate"]["commit"] = submission["base"]["commit"]
        submission["changed_paths"] = []
        rebind_historical_pair(bundle, submission, verdict)
        reference = bundle["integration_receipt"]["ref"]
        integration = strict_load_bytes(bundle["artifacts"][reference])
        integration["result"] = "already_observed"
        replace_artifact(bundle, reference, integration)
        bundle["observed_target"] = submission["candidate"]["commit"]
        bundle["target_ancestors"] = {submission["candidate"]["commit"]}
    elif name == "integrated_without_grant":
        plan["authority"]["grants"] = [
            grant for grant in plan["authority"]["grants"] if grant["action"] != "integrate"
        ]
        rebind_plan_chain(bundle)
    elif name == "integration_artifact_bytes":
        reference = bundle["integration_receipt"]["ref"]
        bundle["artifacts"][reference] += b"\n"
    elif name in {"integration_authority_digest", "integration_before_verdict"}:
        reference = bundle["integration_receipt"]["ref"]
        integration = strict_load_bytes(bundle["artifacts"][reference])
        if name == "integration_authority_digest":
            integration["authority_source_digest"] = ZERO_DIGEST
        else:
            integration["authorized_at"] = "2026-07-19T00:06:50Z"
            integration["started_at"] = "2026-07-19T00:06:51Z"
        replace_artifact(bundle, reference, integration)
    elif name == "board_pass_as_ready":
        board["state"] = "active"
        row = board["work"][0]
        row["state"] = "ready"
        row["next_action"] = "build"
        for key in (
            "submission_id", "submission_digest", "candidate_commit",
            "verdict_id", "verdict_digest", "verdict",
        ):
            row.pop(key, None)
    elif name == "board_pass_as_reviewable":
        board["state"] = "active"
        row = board["work"][0]
        row["state"] = "reviewable"
        row["next_action"] = "verify"
        for key in ("verdict_id", "verdict_digest", "verdict"):
            row.pop(key, None)
    elif name == "phantom_second_integrated":
        second = copy.deepcopy(plan["work"][0])
        second["id"] = "phantom-work"
        second["acceptance"][0]["id"] = "AC2"
        second["depends_on"] = [plan["work"][0]["id"]]
        plan["work"].append(second)
        rebind_plan_chain(bundle)
        board["work"].append({
            "id": "phantom-work",
            "state": "integrated",
            "attempt": 1,
            "submission_id": "phantom-submission",
            "submission_digest": ZERO_DIGEST,
            "candidate_commit": BAD_OID,
            "verdict_id": "phantom-verdict",
            "verdict_digest": ZERO_DIGEST,
            "verdict": "PASS",
            "next_action": "none",
        })
    elif name == "dependent_waiting_after_pass":
        second = copy.deepcopy(plan["work"][0])
        second["id"] = "health-header"
        second["acceptance"][0]["id"] = "AC2"
        second["depends_on"] = [plan["work"][0]["id"]]
        plan["work"].append(second)
        rebind_plan_chain(bundle)
        board["state"] = "active"
        board["work"].append({
            "id": second["id"],
            "state": "waiting",
            "attempt": 0,
            "next_action": "wait",
        })
    elif name == "mixed_integrated_revoked_attention":
        second = copy.deepcopy(plan["work"][0])
        second["id"] = "health-header"
        second["acceptance"][0]["id"] = "AC2"
        second["depends_on"] = [plan["work"][0]["id"]]
        plan["work"].append(second)
        rebind_plan_chain(bundle)
        board["state"] = "attention"
        board["attention"] = ["Authority was revoked before pending work began."]
        board["work"].append({
            "id": second["id"],
            "state": "attention",
            "attempt": 0,
            "next_action": "replan",
            "attention": "Current authority is revoked.",
        })
        bundle["work_controls"] = {second["id"]: "attention"}
        bundle["authority_source"]["status"] = "revoked"
    elif name == "dependent_active_after_fail":
        verdict["outcome"] = "FAIL"
        verdict["acceptance_results"][0]["outcome"] = "fail"
        verdict["findings"] = [{
            "id": "F-dependency-failed",
            "kind": "implementation",
            "principle": "B3",
            "severity": "blocking",
            "summary": "The dependency has not passed.",
            "acceptance_ids": ["AC1"],
            "evidence_ids": [submission["evidence"][0]["id"]],
        }]
        board["state"] = "active"
        board["work"][0]["state"] = "repair"
        board["work"][0]["verdict"] = "FAIL"
        board["work"][0]["next_action"] = "repair"
        rebind_verdict_board(bundle)
        remove_integration(bundle, submission["work_id"])
        bundle["observed_target"] = submission["base"]["commit"]
        bundle["target_ancestors"] = {submission["base"]["commit"]}
        second = copy.deepcopy(plan["work"][0])
        second["id"] = "health-header"
        second["acceptance"][0]["id"] = "AC2"
        second["depends_on"] = [plan["work"][0]["id"]]
        plan["work"].append(second)
        rebind_plan_chain(bundle)
        board["work"].append({
            "id": second["id"],
            "state": "active",
            "attempt": 0,
            "next_action": "wait",
        })
        bundle["work_controls"] = {second["id"]: "active"}
    elif name == "dependency_builder_before_pass":
        make_two_work_serial(bundle)
        second_submission = bundle["submissions"][1]
        second_verdict = bundle["verdicts"][1]
        second_submission["builder"]["started_at"] = "2026-07-19T00:06:00Z"
        rebind_historical_pair(bundle, second_submission, second_verdict)
    elif name == "dependency_pass_superseded":
        make_two_work_serial(bundle)
        first_submission = bundle["submissions"][0]
        first_verdict = bundle["verdicts"][0]
        inconclusive = copy.deepcopy(first_verdict)
        inconclusive["verdict_id"] = "verdict-example-release-health-recheck"
        inconclusive["outcome"] = "INCONCLUSIVE"
        inconclusive["review"]["run_id"] = "verifier-run-recheck"
        inconclusive["review"]["started_at"] = "2026-07-19T00:05:35Z"
        inconclusive["review"]["completed_at"] = "2026-07-19T00:05:50Z"
        inconclusive["acceptance_results"][0]["outcome"] = "inconclusive"
        inconclusive["findings"] = [{
            "id": "F-dependency-recheck",
            "kind": "environment",
            "principle": "B4",
            "severity": "blocking",
            "summary": "The current dependency review is inconclusive.",
            "acceptance_ids": ["AC1"],
            "evidence_ids": [first_submission["evidence"][0]["id"]],
        }]
        dispatch = copy.deepcopy(
            strict_load_bytes(
                bundle["artifacts"][first_verdict["review"]["dispatch_receipt"]["ref"]]
            )
        )
        dispatch["dispatch_id"] = "verifier-run-recheck"
        dispatch["workspace"] = "fresh-read-only-dependency-recheck"
        dispatch["created_at"] = "2026-07-19T00:05:30Z"
        dispatch_ref = "examples/artifacts/dispatch/verifier-run-recheck.json"
        dispatch_raw = _pretty_json(dispatch)
        bundle["artifacts"][dispatch_ref] = dispatch_raw
        inconclusive["review"]["dispatch_receipt"] = {
            "ref": dispatch_ref,
            "media_type": "application/json",
            "digest": raw_digest(dispatch_raw),
        }
        bundle["verdicts"].insert(1, inconclusive)
        remove_integration(bundle, first_submission["work_id"])
        first_row = board["work"][0]
        first_row["state"] = "retry"
        first_row["verdict_id"] = inconclusive["verdict_id"]
        first_row["verdict_digest"] = canonical_digest(inconclusive)
        first_row["verdict"] = "INCONCLUSIVE"
        first_row["next_action"] = "retry_verification"
        board["state"] = "active"
    elif name == "duplicate_integration_effect":
        make_two_work_serial(bundle)
        first_envelope = bundle["integration_receipts"][plan["work"][0]["id"]]
        second_envelope = bundle["integration_receipts"][plan["work"][1]["id"]]
        first_integration = strict_load_bytes(
            bundle["artifacts"][first_envelope["ref"]]
        )
        second_integration = strict_load_bytes(
            bundle["artifacts"][second_envelope["ref"]]
        )
        second_integration["effect_id"] = first_integration["effect_id"]
        second_raw = _pretty_json(second_integration)
        bundle["artifacts"][second_envelope["ref"]] = second_raw
        second_envelope["digest"] = raw_digest(second_raw)
    elif name == "authority_receipt_id_reuse":
        make_two_work_serial(bundle)
        second_submission = bundle["submissions"][1]
        second_verdict = bundle["verdicts"][1]
        original_ref = second_submission["authority_receipt"]["ref"]
        reused_approval = copy.deepcopy(
            strict_load_bytes(bundle["artifacts"][original_ref])
        )
        reused_approval["approved_at"] = "2026-07-19T00:00:31Z"
        reused_ref = "examples/artifacts/authority/plan-approval-reused-id.json"
        reused_raw = _pretty_json(reused_approval)
        bundle["artifacts"][reused_ref] = reused_raw
        second_submission["authority_receipt"] = {
            "ref": reused_ref,
            "media_type": "application/json",
            "digest": raw_digest(reused_raw),
        }
        rebind_historical_pair(bundle, second_submission, second_verdict)
    elif name == "two_work_serial":
        make_two_work_serial(bundle)
    else:
        raise KeyError(f"unknown model mutation: {name}")
    return bundle


def _duplicates(values: list[str]) -> set[str]:
    seen: set[str] = set()
    return {value for value in values if value in seen or seen.add(value)}


def _timestamp(value: str) -> Decimal:
    match = RFC3339.fullmatch(value)
    if match is None or not _is_date_time(value):
        raise ValueError(f"invalid RFC 3339 date-time: {value}")
    date, hour, minute, second, fraction, zone = match.groups()
    safe_zone = "+00:00" if zone in {"Z", "z"} else zone
    minute_start = datetime.fromisoformat(
        f"{date}T{hour}:{minute}:00{safe_zone}"
    ).astimezone(timezone.utc)
    epoch = datetime(1970, 1, 1, tzinfo=timezone.utc)
    delta = minute_start - epoch
    whole_seconds = delta.days * 86_400 + delta.seconds
    second_value = Decimal(second)
    if fraction:
        second_value += Decimal("0" + fraction)
    return Decimal(whole_seconds) + second_value


def _is_json_media_type(value: object) -> bool:
    if not isinstance(value, str):
        return False
    base = value.split(";", 1)[0].strip().lower()
    return base == "application/json" or base.endswith("+json")


def _derive_delivery_state(board: dict[str, Any]) -> str:
    states = [row["state"] for row in board["work"]]
    if board.get("attention") or any(state in {"attention", "blocked"} for state in states):
        return "attention"
    if all(state == "waiting" for state in states):
        return "planned"
    if all(state == "integrated" for state in states):
        return "integrated"
    for projected in ("integrating", "ready_to_integrate", "verified"):
        if projected in states and all(state in {projected, "integrated"} for state in states):
            return projected
    return "active"


def _path_matches(path: str, prefix: str) -> bool:
    return prefix == "." or path == prefix or path.startswith(prefix + "/")


def validate_chain(bundle: dict[str, Any]) -> list[str]:
    errors: list[str] = []

    def reject(condition: bool, code: str) -> None:
        if condition:
            errors.append(code)

    def resolve_json_ref(
        reference: str, error_code: str, *, required: bool = True
    ) -> Any | None:
        try:
            physical = _resolve_local_ref(reference)
        except (StrictJSONError, OSError, TypeError, ValueError):
            if required:
                errors.append(error_code)
            return None
        return bundle.get("resolved_refs", {}).get(reference, physical)

    def resolve_raw_ref(reference: str, error_code: str) -> bytes | None:
        try:
            physical = _resolve_local_bytes(reference)
        except (StrictJSONError, OSError, TypeError, ValueError):
            errors.append(error_code)
            return None
        return bundle.get("artifacts", {}).get(reference, physical)

    plan = bundle["plan"]
    submission = bundle["submission"]
    verdict = bundle["verdict"]
    board = bundle["board"]

    for label, schema_path, value in (
        ("plan", "schemas/delivery-plan-v1.json", plan),
        ("submission", "schemas/submission-v1.json", submission),
        ("verdict", "schemas/delivery-verdict-v1.json", verdict),
        ("board", "schemas/delivery-board-v1.json", board),
    ):
        schema = strict_load(schema_path)
        validator = Draft202012Validator(schema, format_checker=FORMAT_CHECKER)
        if next(validator.iter_errors(value), None) is not None:
            errors.append(f"schema_{label}")

    submissions = bundle.get("submissions", [submission])
    verdicts = bundle.get("verdicts", [verdict])
    submission_schema = strict_load("schemas/submission-v1.json")
    verdict_schema = strict_load("schemas/delivery-verdict-v1.json")
    submission_validator = Draft202012Validator(
        submission_schema, format_checker=FORMAT_CHECKER
    )
    verdict_validator = Draft202012Validator(
        verdict_schema, format_checker=FORMAT_CHECKER
    )
    for historical_submission in submissions:
        if historical_submission is submission:
            continue
        if next(submission_validator.iter_errors(historical_submission), None) is not None:
            errors.append("schema_submission_history")
    for historical_verdict in verdicts:
        if historical_verdict is verdict:
            continue
        if next(verdict_validator.iter_errors(historical_verdict), None) is not None:
            errors.append("schema_verdict_history")

    submission_ids = [item["submission_id"] for item in submissions]
    verdict_ids = [item["verdict_id"] for item in verdicts]
    reject(bool(_duplicates(submission_ids)), "duplicate_submission_id")
    reject(bool(_duplicates(verdict_ids)), "duplicate_verdict_id")
    builder_run_ids = [item["builder"]["run_id"] for item in submissions]
    check_run_ids = [
        check["run_id"] for item in submissions for check in item["checks"]
    ]
    verifier_run_ids = [item["review"]["run_id"] for item in verdicts]
    reject(bool(_duplicates(builder_run_ids)), "duplicate_builder_run_id")
    reject(bool(_duplicates(check_run_ids)), "duplicate_check_run_id")
    reject(bool(_duplicates(verifier_run_ids)), "duplicate_verifier_run_id")
    approval_receipt_digests: dict[str, str] = {}
    for historical_submission in submissions:
        approval_envelope = historical_submission["authority_receipt"]
        approval_raw = bundle["artifacts"].get(approval_envelope["ref"])
        if approval_raw is None:
            continue
        try:
            approval_record = strict_load_bytes(approval_raw)
        except StrictJSONError:
            continue
        approval_id = approval_record.get("receipt_id")
        if not isinstance(approval_id, str):
            continue
        approval_digest = raw_digest(approval_raw)
        prior_approval_digest = approval_receipt_digests.get(approval_id)
        reject(
            prior_approval_digest is not None
            and prior_approval_digest != approval_digest,
            "authority_receipt_id_reused",
        )
        approval_receipt_digests[approval_id] = approval_digest
    work_attempts = [
        f"{item['work_id']}\x00{item['attempt']}" for item in submissions
    ]
    reject(bool(_duplicates(work_attempts)), "duplicate_work_attempt")
    known_submission_ids = set(submission_ids)
    reject(
        any(item["submission_id"] not in known_submission_ids for item in verdicts),
        "orphan_verdict",
    )

    work_ids = [work["id"] for work in plan["work"]]
    reject(bool(_duplicates(work_ids)), "duplicate_work_id")
    work_by_id = {work["id"]: work for work in plan["work"]}
    acceptance_ids = [item["id"] for work in plan["work"] for item in work["acceptance"]]
    reject(bool(_duplicates(acceptance_ids)), "duplicate_acceptance_id")
    for work in plan["work"]:
        for dependency in work["depends_on"]:
            reject(dependency not in work_by_id, "unknown_dependency")

    visiting: set[str] = set()
    visited: set[str] = set()

    def visit(work_id: str) -> bool:
        if work_id in visiting:
            return True
        if work_id in visited or work_id not in work_by_id:
            return False
        visiting.add(work_id)
        cyclic = any(visit(dependency) for dependency in work_by_id[work_id]["depends_on"])
        visiting.remove(work_id)
        visited.add(work_id)
        return cyclic

    reject(any(visit(work_id) for work_id in work_ids), "dependency_cycle")

    policy = resolve_json_ref(plan["assurance_policy"]["ref"], "plan_policy_ref_unresolved")
    policy_pack_ids: set[str] = set()
    policy_check_ids: set[str] = set()
    if policy is not None:
        policy_schema = strict_load("schemas/assurance-policy-v1.json")
        policy_validator = Draft202012Validator(policy_schema, format_checker=FORMAT_CHECKER)
        reject(next(policy_validator.iter_errors(policy), None) is not None, "policy_schema_invalid")
        try:
            reject(
                plan["assurance_policy"]["digest"] != canonical_digest(policy),
                "plan_policy_digest_mismatch",
            )
        except (StrictJSONError, TypeError, ValueError):
            errors.append("plan_policy_digest_invalid")
        if isinstance(policy, dict):
            check_entries = policy.get("checks", [])
            check_ids = [
                check.get("id") for check in check_entries
                if isinstance(check, dict) and isinstance(check.get("id"), str)
            ] if isinstance(check_entries, list) else []
            reject(bool(_duplicates(check_ids)), "duplicate_policy_check_id")
            policy_check_ids = set(check_ids)

        if isinstance(policy, dict) and isinstance(policy.get("packs"), list):
            pack_ids = [
                pack.get("id") for pack in policy["packs"]
                if isinstance(pack, dict) and isinstance(pack.get("id"), str)
            ]
            reject(bool(_duplicates(pack_ids)), "duplicate_policy_pack_id")
            policy_pack_ids = set(pack_ids)

        selected_pack_ids = {
            pack_id
            for work in plan["work"]
            for pack_id in work.get("assurance", {}).get("packs", [])
        }
        entries: list[tuple[str, dict[str, Any]]] = []
        if isinstance(policy, dict):
            entries.extend(
                ("check", entry) for entry in policy.get("checks", [])
                if isinstance(entry, dict)
            )
            entries.extend(
                ("pack", entry) for entry in policy.get("packs", [])
                if isinstance(entry, dict) and entry.get("id") in selected_pack_ids
            )
        for entry_kind, entry in entries:
            if not isinstance(entry.get("definition"), dict):
                continue
            definition = entry["definition"]
            prefix = "policy_check" if entry_kind == "check" else "policy_definition"
            reject(
                definition.get("media_type") != "application/json",
                f"{prefix}_media_type",
            )
            reference = definition.get("ref")
            if not isinstance(reference, str):
                errors.append(f"{prefix}_ref_unresolved")
                continue
            raw = resolve_raw_ref(reference, f"{prefix}_ref_unresolved")
            if raw is None:
                continue
            reject(
                raw_digest(raw) != definition.get("digest"),
                f"{prefix}_digest_mismatch",
            )
            try:
                strict_load_bytes(raw)
            except StrictJSONError:
                errors.append(f"{prefix}_invalid_json")

    integrated_claim = any(
        row.get("state") == "integrated"
        and row.get("submission_id") == submission["submission_id"]
        for row in board.get("work", [])
    )
    post_submission_attention = (
        board.get("state") == "attention" and bool(board.get("attention"))
    )
    authority_source = resolve_json_ref(
        plan["authority"]["ref"],
        "authority_source_ref_unresolved",
        required=not integrated_claim and not post_submission_attention,
    )
    authority_snapshot = bundle.get("authority_source_snapshot", authority_source)
    integrate_grants = [grant for grant in plan["authority"]["grants"] if grant["action"] == "integrate"]
    for grant in integrate_grants:
        reject(grant["target"] != plan["target"], "integration_grant_target_mismatch")

    authority_envelope = submission["authority_receipt"]
    reject(authority_envelope.get("media_type") != "application/json", "authority_receipt_media_type")
    authority_raw = bundle["artifacts"].get(authority_envelope["ref"])
    receipt: dict[str, Any] | None = None
    if authority_raw is None:
        errors.append("authority_receipt_artifact_missing")
    else:
        reject(raw_digest(authority_raw) != authority_envelope["digest"], "authority_receipt_artifact_digest_mismatch")
        try:
            receipt = strict_load_bytes(authority_raw)
            control_schema = strict_load("schemas/control-receipt-v1.json")
            control_validator = Draft202012Validator(
                control_schema, format_checker=FORMAT_CHECKER
            )
            reject(
                next(control_validator.iter_errors(receipt), None) is not None,
                "authority_receipt_schema_invalid",
            )
        except StrictJSONError:
            errors.append("authority_receipt_invalid_json")

    approved_time: Decimal | None = None
    source_valid_from: Decimal | None = None
    source_valid_until: Decimal | None = None
    if receipt is not None:
        required_receipt = {
            "schema_version", "kind", "receipt_id", "plan_digest", "authority_digest", "source_ref", "source_digest",
            "grants", "repository", "target_ref", "authorizer_ref", "approved_at",
        }
        reject(not required_receipt.issubset(receipt), "authority_receipt_missing_field")
        if required_receipt.issubset(receipt):
            reject(
                receipt["schema_version"] != "control-receipt-v1"
                or receipt["kind"] != "authority_approval",
                "authority_receipt_version_mismatch",
            )
            reject(receipt["plan_digest"] != canonical_digest(plan), "authority_plan_digest_mismatch")
            reject(receipt["authority_digest"] != canonical_digest(plan["authority"]), "authority_object_digest_mismatch")
            reject(receipt["source_ref"] != plan["authority"]["ref"], "authority_source_ref_mismatch")
            if authority_snapshot is not None:
                reject(
                    receipt["source_digest"] != canonical_digest(authority_snapshot),
                    "authority_source_digest_mismatch",
                )
            reject(receipt["grants"] != plan["authority"]["grants"], "authority_grants_mismatch")
            reject(
                receipt["repository"] != plan["target"]["repository"]
                or receipt["target_ref"] != plan["target"]["ref"],
                "authority_target_mismatch",
            )
            if isinstance(authority_snapshot, dict):
                source = authority_snapshot
                reject(source.get("status") != "active", "authority_source_not_active")
                reject(source.get("authorizer_ref") != receipt["authorizer_ref"], "authority_authorizer_mismatch")
                reject(
                    source.get("repository") != plan["target"]["repository"]
                    or source.get("target_ref") != plan["target"]["ref"],
                    "authority_source_target_mismatch",
                )
                maximum = {jcs(grant) for grant in source.get("maximum_grants", [])}
                reject(any(jcs(grant) not in maximum for grant in receipt["grants"]), "authority_grant_exceeds_source")
                try:
                    approved_time = _timestamp(receipt["approved_at"])
                    source_valid_from = _timestamp(source["valid_from"])
                    source_valid_until = _timestamp(source["valid_until"])
                    reject(_timestamp(plan["created_at"]) > approved_time, "authority_approval_precedes_plan")
                    reject(source_valid_from > approved_time, "authority_not_yet_valid")
                    reject(source_valid_until < approved_time, "authority_expired_at_approval")
                except (KeyError, TypeError, ValueError):
                    errors.append("authority_timestamp_invalid")
            if not integrated_claim and not post_submission_attention:
                if not isinstance(authority_source, dict):
                    errors.append("authority_source_unavailable")
                else:
                    reject(authority_source.get("status") != "active", "authority_source_not_active")
                    reject(
                        canonical_digest(authority_source) != receipt["source_digest"],
                        "authority_source_digest_mismatch",
                    )

    reject(submission["plan_digest"] != canonical_digest(plan), "submission_plan_digest_mismatch")
    reject(submission["delivery_id"] != plan["delivery_id"], "submission_delivery_id_mismatch")
    contract = work_by_id.get(submission["work_id"])
    if contract is None:
        errors.append("submission_work_id_unknown")
        contract_acceptance: dict[str, Any] = {}
        contract_packs: set[str] = set()
    else:
        reject(submission["contract_digest"] != canonical_digest(contract), "contract_digest_mismatch")
        contract_acceptance = {item["id"]: item for item in contract["acceptance"]}
        contract_packs = set(contract["assurance"]["packs"])
        reject(submission["assurance"]["profile"] != contract["assurance"]["profile"], "submission_profile_mismatch")
        reject(set(submission["assurance"]["packs"]) != contract_packs, "submission_pack_set_mismatch")
        reject(any(pack not in policy_pack_ids for pack in contract_packs), "unknown_policy_pack")

    reject(submission["assurance"]["policy_ref"] != plan["assurance_policy"]["ref"], "submission_policy_ref_mismatch")
    reject(submission["assurance"]["policy_digest"] != plan["assurance_policy"]["digest"], "submission_policy_digest_mismatch")
    reject(submission["base"]["repository"] != plan["target"]["repository"], "base_repository_mismatch")
    reject(submission["candidate"]["repository"] != plan["target"]["repository"], "candidate_repository_mismatch")
    reject(submission["base"]["ref"] != plan["target"]["ref"], "base_ref_mismatch")

    if contract is not None:
        for path in submission["changed_paths"]:
            included = any(_path_matches(path, prefix) for prefix in contract["scope"]["include"])
            excluded = any(_path_matches(path, prefix) for prefix in contract["scope"]["exclude"])
            reject(not included or excluded, "changed_path_out_of_scope")

    candidate_tree = submission["candidate"]["tree"]
    check_ids = [check["id"] for check in submission["checks"]]
    evidence_ids = [evidence["id"] for evidence in submission["evidence"]]
    reject(bool(_duplicates(check_ids)), "duplicate_check_id")
    reject(not policy_check_ids.issubset(check_ids), "missing_policy_check")
    reject(bool(_duplicates(evidence_ids)), "duplicate_evidence_id")
    builder_start: Decimal | None = None
    builder_end: Decimal | None = None
    try:
        builder_start = _timestamp(submission["builder"]["started_at"])
        builder_end = _timestamp(submission["builder"]["completed_at"])
        reject(builder_start > builder_end, "builder_timestamp_order")
        reject(builder_end > _timestamp(submission["created_at"]), "builder_after_submission")
        if approved_time is not None:
            reject(builder_start < approved_time, "builder_precedes_authority")
        if source_valid_from is not None:
            reject(builder_start < source_valid_from, "authority_not_valid_at_builder")
        if source_valid_until is not None:
            reject(builder_start > source_valid_until, "authority_expired_before_builder")
    except (TypeError, ValueError):
        errors.append("builder_timestamp_invalid")

    if contract is not None and builder_start is not None:
        submission_by_id_for_dependencies = {
            item["submission_id"]: item for item in submissions
        }
        for dependency_id in contract["depends_on"]:
            dependency_verdicts_before_builder: list[
                tuple[Decimal, int, str]
            ] = []
            for verdict_order, historical_verdict in enumerate(verdicts):
                historical_submission = submission_by_id_for_dependencies.get(
                    historical_verdict["submission_id"]
                )
                if (
                    historical_submission is None
                    or historical_submission["work_id"] != dependency_id
                ):
                    continue
                try:
                    completed_at = _timestamp(
                        historical_verdict["review"]["completed_at"]
                    )
                except (KeyError, TypeError, ValueError):
                    continue
                if completed_at <= builder_start:
                    dependency_verdicts_before_builder.append(
                        (completed_at, verdict_order, historical_verdict["outcome"])
                    )
            current_dependency_outcome = (
                max(
                    dependency_verdicts_before_builder,
                    key=lambda item: item[1],
                )[2]
                if dependency_verdicts_before_builder
                else None
            )
            reject(
                current_dependency_outcome != "PASS",
                "dependency_not_passed_before_builder",
            )

    known_runs: set[str] = set()
    producer_windows: dict[str, tuple[Decimal, Decimal]] = {}
    for check in submission["checks"]:
        known_runs.add(check["run_id"])
        reject(
            check["run_id"] == submission["builder"]["run_id"],
            "builder_stamped_check",
        )
        reject(check["candidate_tree"] != candidate_tree, "check_candidate_tree_mismatch")
        raw = bundle["artifacts"].get(check["receipt"]["ref"])
        reject(raw is None, "check_artifact_missing")
        if raw is not None:
            reject(raw_digest(raw) != check["receipt"]["digest"], "check_artifact_digest_mismatch")
            if _is_json_media_type(check["receipt"].get("media_type")):
                try:
                    strict_load_bytes(raw)
                except StrictJSONError:
                    errors.append("check_artifact_invalid_json")
        try:
            check_start = _timestamp(check["started_at"])
            check_end = _timestamp(check["completed_at"])
            producer_windows[check["run_id"]] = (check_start, check_end)
            reject(check_start > check_end, "check_timestamp_order")
            reject(check_end > _timestamp(submission["created_at"]), "check_after_submission")
            if builder_end is not None:
                reject(check_start < builder_end, "check_precedes_builder")
        except (TypeError, ValueError):
            errors.append("check_timestamp_invalid")

    evidence_by_id = {evidence["id"]: evidence for evidence in submission["evidence"]}
    for evidence in submission["evidence"]:
        reject(evidence["candidate_tree"] != candidate_tree, "evidence_candidate_tree_mismatch")
        reject(evidence["producer_run_id"] not in known_runs, "unknown_evidence_producer")
        raw = bundle["artifacts"].get(evidence["artifact"]["ref"])
        reject(raw is None, "evidence_artifact_missing")
        if raw is not None:
            reject(raw_digest(raw) != evidence["artifact"]["digest"], "evidence_artifact_digest_mismatch")
            if _is_json_media_type(evidence["artifact"].get("media_type")):
                try:
                    strict_load_bytes(raw)
                except StrictJSONError:
                    errors.append("evidence_artifact_invalid_json")
        for acceptance_id in evidence.get("acceptance_ids", []):
            reject(acceptance_id not in contract_acceptance, "unknown_evidence_acceptance")
            if acceptance_id in contract_acceptance:
                required = contract_acceptance[acceptance_id]["evidence_level"]
                reject(BOUNDARY_RANK[evidence["boundary"]] < BOUNDARY_RANK[required], "evidence_boundary_too_weak")
        for pack_id in evidence.get("pack_ids", []):
            reject(pack_id not in contract_packs, "unknown_evidence_pack")
        reject(evidence["uses_mocks"] and evidence["boundary"] != "component", "mocked_high_boundary_evidence")
        try:
            captured = _timestamp(evidence["captured_at"])
            reject(captured > _timestamp(submission["created_at"]), "evidence_after_submission")
            window = producer_windows.get(evidence["producer_run_id"])
            if window is not None:
                reject(captured < window[0], "evidence_precedes_producer")
                reject(captured > window[1], "evidence_after_producer")
        except (TypeError, ValueError):
            errors.append("evidence_timestamp_invalid")

    submission_digest = canonical_digest(submission)
    if bundle.get("_submission_only"):
        return errors

    dispatch_envelope = verdict["review"]["dispatch_receipt"]
    reject(dispatch_envelope.get("media_type") != "application/json", "dispatch_media_type")
    dispatch_raw = bundle["artifacts"].get(dispatch_envelope["ref"])
    dispatch: dict[str, Any] | None = None
    if dispatch_raw is None:
        errors.append("dispatch_artifact_missing")
    else:
        reject(raw_digest(dispatch_raw) != dispatch_envelope["digest"], "dispatch_artifact_digest_mismatch")
        try:
            dispatch = strict_load_bytes(dispatch_raw)
            control_schema = strict_load("schemas/control-receipt-v1.json")
            control_validator = Draft202012Validator(
                control_schema, format_checker=FORMAT_CHECKER
            )
            reject(
                next(control_validator.iter_errors(dispatch), None) is not None,
                "dispatch_schema_invalid",
            )
        except StrictJSONError:
            errors.append("dispatch_invalid_json")
    if dispatch is not None:
        reject(
            dispatch.get("schema_version") != "control-receipt-v1"
            or dispatch.get("kind") != "verifier_dispatch",
            "dispatch_version_mismatch",
        )
        reject(dispatch.get("dispatch_id") != verdict["review"]["run_id"], "dispatch_run_id_mismatch")
        reject(dispatch.get("role") != "verifier", "dispatch_role_mismatch")
        reject(dispatch.get("submission_digest") != submission_digest, "dispatch_submission_digest_mismatch")
        reject(dispatch.get("candidate") != submission["candidate"], "dispatch_candidate_mismatch")
        isolated = (
            dispatch.get("fresh_context") is True
            and dispatch.get("builder_transcript_included") is False
            and dispatch.get("target_ref_writable") is False
            and dispatch.get("remotes_present") is False
            and dispatch.get("write_credentials_present") is False
        )
        reject(not isolated, "verifier_dispatch_not_isolated")
        try:
            dispatch_time = _timestamp(dispatch["created_at"])
            reject(dispatch_time < _timestamp(submission["created_at"]), "dispatch_precedes_submission")
            reject(dispatch_time > _timestamp(verdict["review"]["started_at"]), "review_precedes_dispatch")
            if source_valid_from is not None:
                reject(dispatch_time < source_valid_from, "authority_not_valid_at_verifier_dispatch")
            if source_valid_until is not None:
                reject(dispatch_time > source_valid_until, "authority_expired_before_verifier_dispatch")
        except (KeyError, TypeError, ValueError):
            errors.append("dispatch_timestamp_invalid")

    reject(verdict["submission_id"] != submission["submission_id"], "verdict_submission_id_mismatch")
    reject(verdict["submission_digest"] != submission_digest, "verdict_submission_digest_mismatch")
    reject(verdict["delivery_id"] != submission["delivery_id"], "verdict_delivery_id_mismatch")
    reject(verdict["work_id"] != submission["work_id"], "verdict_work_id_mismatch")
    reject(verdict["review"]["run_id"] == submission["builder"]["run_id"], "verifier_not_independent")
    review_completed: Decimal | None = None
    try:
        review_started = _timestamp(verdict["review"]["started_at"])
        review_completed = _timestamp(verdict["review"]["completed_at"])
        reject(review_started > review_completed, "review_timestamp_order")
    except (TypeError, ValueError):
        errors.append("review_timestamp_invalid")

    result_ids = [result["acceptance_id"] for result in verdict["acceptance_results"]]
    reject(bool(_duplicates(result_ids)), "duplicate_acceptance_result")
    reject(set(result_ids) != set(contract_acceptance), "acceptance_result_set_mismatch")
    pack_ids = [result["pack"] for result in verdict["assurance_results"]]
    reject(bool(_duplicates(pack_ids)), "duplicate_assurance_result")
    reject(set(pack_ids) != contract_packs, "assurance_result_set_mismatch")
    finding_ids = [finding["id"] for finding in verdict["findings"]]
    reject(bool(_duplicates(finding_ids)), "duplicate_finding_id")

    for result in verdict["acceptance_results"]:
        for evidence_id in result["evidence_ids"]:
            reject(evidence_id not in evidence_by_id, "unknown_verdict_evidence")
            if evidence_id in evidence_by_id:
                reject(result["acceptance_id"] not in evidence_by_id[evidence_id].get("acceptance_ids", []), "verdict_evidence_link_mismatch")
    for result in verdict["assurance_results"]:
        for evidence_id in result["evidence_ids"]:
            reject(evidence_id not in evidence_by_id, "unknown_verdict_evidence")
            if evidence_id in evidence_by_id:
                reject(result["pack"] not in evidence_by_id[evidence_id].get("pack_ids", []), "verdict_pack_evidence_link_mismatch")
    for finding in verdict["findings"]:
        reject(any(item not in contract_acceptance for item in finding["acceptance_ids"]), "unknown_finding_acceptance")
        reject(any(item not in evidence_by_id for item in finding["evidence_ids"]), "unknown_finding_evidence")

    if verdict["outcome"] == "PASS":
        if review_completed is not None and source_valid_from is not None:
            reject(review_completed < source_valid_from, "authority_not_valid_at_pass")
        if review_completed is not None and source_valid_until is not None:
            reject(review_completed > source_valid_until, "authority_expired_before_pass")
        reject(any(check["outcome"] != "pass" for check in submission["checks"]), "pass_with_nonpassing_check")
        reject(any(result["outcome"] != "pass" or not result["evidence_ids"] for result in verdict["acceptance_results"]), "pass_acceptance_not_proven")
        reject(any(result["outcome"] != "pass" or not result["evidence_ids"] for result in verdict["assurance_results"]), "pass_assurance_not_proven")
        reject(any(finding["severity"] == "blocking" for finding in verdict["findings"]), "pass_with_blocking_finding")

    history = bundle.get("record_history", {})
    for historical_submission in submissions:
        historical_digest = canonical_digest(historical_submission)
        prior_submission = history.get("submission", {}).get(
            historical_submission["submission_id"]
        )
        reject(
            prior_submission is not None and prior_submission != historical_digest,
            "submission_id_reused",
        )
    verdict_digest = canonical_digest(verdict)
    for historical_verdict in verdicts:
        historical_digest = canonical_digest(historical_verdict)
        prior_verdict = history.get("verdict", {}).get(
            historical_verdict["verdict_id"]
        )
        reject(
            prior_verdict is not None and prior_verdict != historical_digest,
            "verdict_id_reused",
        )

    candidate_integration_envelope = bundle.get("integration_receipts", {}).get(
        submission["work_id"]
    )
    integration_envelope: dict[str, Any] | None = None
    if candidate_integration_envelope is not None:
        candidate_integration_raw = bundle["artifacts"].get(
            candidate_integration_envelope.get("ref")
        )
        if candidate_integration_raw is not None:
            try:
                candidate_integration = strict_load_bytes(candidate_integration_raw)
            except StrictJSONError:
                candidate_integration = None
            if (
                isinstance(candidate_integration, dict)
                and candidate_integration.get("submission_digest") == submission_digest
                and candidate_integration.get("verdict_digest") == verdict_digest
            ):
                integration_envelope = candidate_integration_envelope
    integration_valid = False
    integration_errors_before = len(errors)
    if integration_envelope is not None:
        reject(
            integration_envelope.get("media_type") != "application/json",
            "integration_receipt_media_type",
        )
        integration_raw = bundle["artifacts"].get(integration_envelope.get("ref"))
        integration: dict[str, Any] | None = None
        if integration_raw is None:
            errors.append("integration_receipt_artifact_missing")
        else:
            reject(
                raw_digest(integration_raw) != integration_envelope.get("digest"),
                "integration_receipt_artifact_digest_mismatch",
            )
            try:
                parsed_integration = strict_load_bytes(integration_raw)
                if isinstance(parsed_integration, dict):
                    integration = parsed_integration
                    control_schema = strict_load("schemas/control-receipt-v1.json")
                    control_validator = Draft202012Validator(
                        control_schema, format_checker=FORMAT_CHECKER
                    )
                    reject(
                        next(control_validator.iter_errors(integration), None) is not None,
                        "integration_receipt_schema_invalid",
                    )
                else:
                    errors.append("integration_receipt_invalid_json")
            except StrictJSONError:
                errors.append("integration_receipt_invalid_json")
        if integration is not None:
            required_integration = {
                "schema_version", "kind", "effect_id", "repository", "target_ref",
                "expected_target", "candidate_commit", "submission_digest",
                "verdict_digest", "authority_receipt_digest",
                "authority_source_ref", "authority_source_digest", "authorized_at",
                "started_at", "completed_at", "result", "observed_target",
            }
            reject(
                not required_integration.issubset(integration),
                "integration_receipt_missing_field",
            )
            if required_integration.issubset(integration):
                reject(
                    integration["schema_version"] != "control-receipt-v1"
                    or integration.get("kind") != "integration",
                    "integration_receipt_version_mismatch",
                )
                reject(
                    integration["repository"] != plan["target"]["repository"]
                    or integration["target_ref"] != plan["target"]["ref"],
                    "integration_target_mismatch",
                )
                reject(
                    integration["expected_target"] != submission["base"]["commit"],
                    "integration_expected_target_mismatch",
                )
                reject(
                    integration["candidate_commit"] != submission["candidate"]["commit"],
                    "integration_candidate_mismatch",
                )
                reject(
                    integration["submission_digest"] != submission_digest,
                    "integration_submission_digest_mismatch",
                )
                reject(
                    integration["verdict_digest"] != verdict_digest,
                    "integration_verdict_digest_mismatch",
                )
                reject(
                    integration["authority_receipt_digest"] != authority_envelope["digest"],
                    "integration_authority_receipt_mismatch",
                )
                if receipt is not None:
                    reject(
                        integration["authority_source_ref"] != receipt.get("source_ref")
                        or integration["authority_source_digest"] != receipt.get("source_digest"),
                        "integration_authority_source_mismatch",
                    )
                reject(integration["result"] not in {"updated", "already_observed"}, "integration_result_invalid")
                reject(
                    integration["observed_target"] != submission["candidate"]["commit"],
                    "integration_observation_mismatch",
                )
                reject(verdict["outcome"] != "PASS", "integration_without_pass")
                reject(not integrate_grants, "integration_without_grant")
                reject(
                    submission["candidate"]["commit"] not in bundle.get("target_ancestors", set()),
                    "integration_candidate_not_reachable",
                )
                try:
                    authorized_at = _timestamp(integration["authorized_at"])
                    integration_started = _timestamp(integration["started_at"])
                    integration_completed = _timestamp(integration["completed_at"])
                    reject(authorized_at > integration_started, "integration_precedes_authorization")
                    reject(integration_started > integration_completed, "integration_timestamp_order")
                    if review_completed is not None:
                        reject(authorized_at < review_completed, "integration_precedes_verdict")
                    if source_valid_from is not None:
                        reject(authorized_at < source_valid_from, "authority_not_valid_at_integration")
                    if source_valid_until is not None:
                        reject(authorized_at > source_valid_until, "authority_expired_before_integration")
                except (TypeError, ValueError):
                    errors.append("integration_timestamp_invalid")
        integration_valid = len(errors) == integration_errors_before

    board_work_ids = [row["id"] for row in board["work"]]
    reject(bool(_duplicates(board_work_ids)), "duplicate_board_work_id")
    reject(set(board_work_ids) != set(work_ids), "board_work_set_mismatch")
    reject(board["delivery_id"] != plan["delivery_id"], "board_delivery_id_mismatch")
    reject(board["plan_digest"] != canonical_digest(plan), "board_plan_digest_mismatch")
    target_ancestors = bundle.get("target_ancestors", set())
    reject(
        bundle.get("observed_target") not in target_ancestors,
        "observed_target_not_reachable",
    )
    integration_effect_ids: list[str] = []
    for historical_envelope in bundle.get("integration_receipts", {}).values():
        historical_raw = bundle["artifacts"].get(historical_envelope.get("ref"))
        if historical_raw is None:
            continue
        try:
            historical_integration = strict_load_bytes(historical_raw)
        except StrictJSONError:
            continue
        effect_id = historical_integration.get("effect_id")
        if isinstance(effect_id, str):
            integration_effect_ids.append(effect_id)
    reject(
        bool(_duplicates(integration_effect_ids)),
        "duplicate_integration_effect_id",
    )
    submission_index = {item["submission_id"]: item for item in submissions}
    verdict_index = {item["verdict_id"]: item for item in verdicts}
    latest_submission_by_work: dict[str, dict[str, Any]] = {}
    for item in submissions:
        previous = latest_submission_by_work.get(item["work_id"])
        if previous is None or item["attempt"] > previous["attempt"]:
            latest_submission_by_work[item["work_id"]] = item
    verdict_by_submission = {item["submission_id"]: item for item in verdicts}
    submission_states = {
        "reviewable", "repair", "blocked", "retry", "verified",
        "ready_to_integrate", "integrating", "integrated",
    }
    verdict_states = {
        "repair", "blocked", "retry", "verified", "ready_to_integrate",
        "integrating", "integrated",
    }
    submission_fields = {"submission_id", "submission_digest", "candidate_commit"}
    verdict_fields = {"verdict_id", "verdict_digest", "verdict"}
    for row in board["work"]:
        state = row["state"]
        actual_submission = latest_submission_by_work.get(row["id"])
        actual_verdict = (
            verdict_by_submission.get(actual_submission["submission_id"])
            if actual_submission is not None else None
        )
        expected_state: str | None = None
        if actual_submission is None:
            control_state = bundle.get("work_controls", {}).get(row["id"])
            dependencies_satisfied = True
            for dependency_id in work_by_id[row["id"]]["depends_on"]:
                dependency_submission = latest_submission_by_work.get(
                    dependency_id
                )
                dependency_verdict = (
                    verdict_by_submission.get(
                        dependency_submission["submission_id"]
                    )
                    if dependency_submission is not None
                    else None
                )
                if (
                    dependency_verdict is None
                    or dependency_verdict["outcome"] != "PASS"
                ):
                    dependencies_satisfied = False
                    break
            if control_state == "attention":
                expected_state = "attention"
            elif control_state == "active" and dependencies_satisfied:
                expected_state = "active"
            else:
                expected_state = "ready" if dependencies_satisfied else "waiting"
        elif actual_verdict is None:
            expected_state = "reviewable"
        elif actual_submission is not None and actual_verdict is not None:
            if actual_verdict["outcome"] == "PASS":
                actual_integration = bundle.get("integration_receipts", {}).get(row["id"])
                actual_reachable = (
                    actual_submission["candidate"]["commit"]
                    in bundle.get("target_ancestors", set())
                )
                if not integrate_grants:
                    expected_state = "verified"
                elif actual_integration is not None and actual_reachable:
                    expected_state = "integrated"
                elif bundle.get("integration_in_progress", {}).get(row["id"]) is True:
                    expected_state = "integrating"
                else:
                    expected_state = "ready_to_integrate"
            else:
                expected_state = {
                    "FAIL": "repair",
                    "SPEC_BLOCK": "blocked",
                    "INCONCLUSIVE": "retry",
                }[actual_verdict["outcome"]]
        if expected_state is not None:
            reject(state != expected_state, "board_lifecycle_state_mismatch")

        if state not in submission_states:
            reject(any(field in row for field in submission_fields), "board_stale_submission_fields")
            reject(any(field in row for field in verdict_fields), "board_stale_verdict_fields")
            continue
        row_submission = submission_index.get(row.get("submission_id"))
        if row_submission is None:
            errors.append("board_submission_unresolved")
            continue
        row_submission_digest = canonical_digest(row_submission)
        reject(row_submission.get("work_id") != row["id"], "board_submission_work_mismatch")
        reject(row.get("attempt") != row_submission["attempt"], "board_attempt_mismatch")
        reject(row.get("submission_digest") != row_submission_digest, "board_submission_digest_mismatch")
        reject(row.get("candidate_commit") != row_submission["candidate"]["commit"], "board_candidate_mismatch")

        if state == "reviewable":
            reject(any(field in row for field in verdict_fields), "board_stale_verdict_fields")
            continue

        row_verdict = verdict_index.get(row.get("verdict_id"))
        if row_verdict is None:
            errors.append("board_verdict_unresolved")
            continue
        row_verdict_digest = canonical_digest(row_verdict)
        reject(row_verdict.get("work_id") != row["id"], "board_verdict_work_mismatch")
        reject(row_verdict.get("submission_id") != row_submission["submission_id"], "board_verdict_submission_mismatch")
        reject(row_verdict.get("submission_digest") != row_submission_digest, "board_verdict_submission_digest_mismatch")
        reject(row.get("verdict_digest") != row_verdict_digest, "board_verdict_digest_mismatch")
        reject(row.get("verdict") != row_verdict["outcome"], "board_verdict_outcome_mismatch")
        if actual_verdict is not None:
            reject(
                row_verdict["verdict_id"] != actual_verdict["verdict_id"]
                or row_verdict_digest != canonical_digest(actual_verdict),
                "board_current_verdict_mismatch",
            )

        if state == "integrated":
            reject(
                row_submission["base"]["commit"] not in target_ancestors,
                "integration_base_not_reachable",
            )
            row_integration_envelope = bundle.get("integration_receipts", {}).get(
                row["id"]
            )
            if row_integration_envelope is None:
                errors.append("board_integration_receipt_missing")
            else:
                row_integration_raw = bundle["artifacts"].get(
                    row_integration_envelope.get("ref")
                )
                if row_integration_raw is None:
                    errors.append("board_integration_receipt_missing")
                else:
                    reject(
                        raw_digest(row_integration_raw)
                        != row_integration_envelope.get("digest"),
                        "board_integration_receipt_digest_mismatch",
                    )
                    try:
                        row_integration = strict_load_bytes(row_integration_raw)
                    except StrictJSONError:
                        errors.append("board_integration_receipt_invalid_json")
                    else:
                        reject(
                            row_integration.get("candidate_commit")
                            != row_submission["candidate"]["commit"],
                            "board_integration_candidate_mismatch",
                        )
                        reject(
                            row_integration.get("submission_digest")
                            != row_submission_digest,
                            "board_integration_submission_mismatch",
                        )
                        reject(
                            row_integration.get("verdict_digest")
                            != row_verdict_digest,
                            "board_integration_verdict_mismatch",
                        )
                        reject(
                            row_integration.get("authority_receipt_digest")
                            != row_submission["authority_receipt"]["digest"],
                            "board_integration_authority_mismatch",
                        )
                        reject(
                            row_integration.get("repository")
                            != plan["target"]["repository"]
                            or row_integration.get("target_ref")
                            != plan["target"]["ref"],
                            "board_integration_target_mismatch",
                        )
                        reject(
                            row_integration.get("result")
                            not in {"updated", "already_observed"},
                            "board_integration_result_invalid",
                        )
                        reject(
                            row_integration.get("observed_target")
                            != row_submission["candidate"]["commit"],
                            "board_integration_observation_mismatch",
                        )

        if (
            row["id"] == submission["work_id"]
            and row.get("verdict_id") == verdict["verdict_id"]
            and state == "integrated"
        ):
            reject(not integration_valid, "board_integration_receipt_invalid")
    if board["state"] == "integrated":
        reject(any(row["state"] != "integrated" for row in board["work"]), "board_integrated_row_mismatch")
    reject(board["state"] != _derive_delivery_state(board), "board_state_aggregate_mismatch")

    # Validate every additional verdict through the same complete cross-record
    # path as the primary verdict, including repeated verification of one immutable
    # submission. Also validate submission-only records that remain reviewable.
    # The guard prevents recursive replay.
    if not bundle.get("_validating_record_pair"):
        submission_by_id = {item["submission_id"]: item for item in submissions}
        submissions_with_verdicts: set[str] = set()
        for historical_verdict in verdicts:
            submissions_with_verdicts.add(historical_verdict["submission_id"])
            if historical_verdict is verdict:
                continue
            historical_submission = submission_by_id.get(
                historical_verdict["submission_id"]
            )
            if historical_submission is None:
                continue
            pair_bundle = dict(bundle)
            pair_bundle["submission"] = historical_submission
            pair_bundle["verdict"] = historical_verdict
            pair_bundle["integration_receipt"] = bundle.get(
                "integration_receipts", {}
            ).get(historical_submission["work_id"])
            pair_bundle["_validating_record_pair"] = True
            errors.extend(validate_chain(pair_bundle))

        for historical_submission in submissions:
            if (
                historical_submission is submission
                or historical_submission["submission_id"] in submissions_with_verdicts
            ):
                continue
            submission_bundle = dict(bundle)
            submission_bundle["submission"] = historical_submission
            submission_bundle["_validating_record_pair"] = True
            submission_bundle["_submission_only"] = True
            errors.extend(validate_chain(submission_bundle))

    return errors


def run() -> list[str]:
    manifest = strict_load("conformance/manifest.json")
    failures: list[str] = []

    for case in manifest["strict_json_cases"]:
        actual_error: str | None = None
        try:
            strict_load(case["instance"])
        except StrictJSONError as error:
            actual_error = error.code
        actual_valid = actual_error is None
        if actual_valid != case["valid"] or (not case["valid"] and actual_error != case["error"]):
            failures.append(f"{case['id']}: expected valid={case['valid']} error={case.get('error')}, got {actual_error or 'valid'}")

    for case in manifest["canonical_cases"]:
        value = case.get("value")
        if "number_strings" in case:
            value = [float(item) for item in case["number_strings"]]
        actual = jcs(value).decode("utf-8")
        if actual != case["expected"]:
            failures.append(f"{case['id']}: canonical bytes {actual!r}, expected {case['expected']!r}")

    for case in manifest["schema_cases"]:
        schema = strict_load(case["schema"])
        Draft202012Validator.check_schema(schema)
        instance = schema_mutation(case.get("mutation"), strict_load(case["instance"]))
        validator = Draft202012Validator(schema, format_checker=FORMAT_CHECKER)
        errors = sorted(validator.iter_errors(instance), key=lambda error: [str(item) for item in error.path])
        actual = not errors
        if actual != case["valid"]:
            detail = "; ".join(error.message for error in errors[:3]) or "unexpectedly valid"
            failures.append(f"{case['id']}: {detail}")

    for case in manifest["model_cases"]:
        try:
            errors = validate_chain(mutate_bundle(case["mutation"]))
        except (KeyError, StrictJSONError, TypeError, ValueError) as error:
            failures.append(f"{case['id']}: harness error: {error}")
            continue
        actual_valid = not errors
        expected_error = case.get("error")
        if actual_valid != case["valid"] or (expected_error is not None and expected_error not in errors):
            failures.append(
                f"{case['id']}: expected valid={case['valid']} error={expected_error}; "
                f"got {', '.join(errors) or 'valid'}"
            )

    if failures:
        return failures

    print(
        f"PASS {len(manifest['strict_json_cases'])} strict JSON cases, "
        f"{len(manifest['canonical_cases'])} canonicalization vectors, "
        f"{len(manifest['schema_cases'])} schema fixtures, and "
        f"{len(manifest['model_cases'])} executable cross-record cases"
    )
    print(
        f"NOT RUN {len(manifest['engine_cases'])} real-boundary engine cases "
        "(requires a conforming engine adapter)"
    )
    return []


def main() -> int:
    failures = run()
    if not failures:
        return 0
    for failure in failures:
        print(f"FAIL {failure}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
