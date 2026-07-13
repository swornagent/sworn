#!/usr/bin/env bash
#
# migrate-records.sh — migrate spec-v1-era release records to the strict baton
# v0.10.0 contract. Committed, re-runnable, and idempotent: running it twice is a
# no-op (every transform is a fixpoint). This is the DATA half of sworn#48 (the
# code half — the read-path normalise shim removal + the ears_pattern reader
# repoint — lives in the sworn binary, S12-record-migration).
#
# Consumer repos with incomplete spec-v1-era releases (e.g. ~/projects/<consumer-repo>) run
# the identical tool:  scripts/migrate-records.sh <path-to>/docs/release
#
# SCOPE: only releases that carry at least one spec.json (spec-v1-era) are
# touched. Pre-spec-v1 legacy releases (markdown-era, 0 spec.json) are left
# untouched by construction (Coach decision 2026-07-10: completed releases are
# not repaired). Renders (index.md) are NOT regenerated here — that is a
# sworn-specific step (`sworn render <release>`) run separately by the owning
# repo; this script is stdlib + jq only so it is portable to non-sworn repos.
#
# Per-record transforms:
#   spec.json (strict spec-v1, additionalProperties:false):
#     - drop schema_version ($schema carries the version)
#     - acceptance_criteria: MAP the retired 'type' -> canonical 'ears_pattern'
#       (unwanted->unwanted-behaviour etc.) so EARS classification is PRESERVED,
#       then drop 'type' and 'ears_keyword' (strict AC = {id,text,ears_pattern,
#       test_refs})  [AC-07 / sworn#95]
#     - effort_complexity.quadrant: chore->quick, epic->beast (axes untouched)
#     - ensure in_scope / out_of_scope arrays exist (historical backfill)
#   status.json (slice-status-v1, additionalProperties:true):
#     - effort_complexity.quadrant: chore->quick, epic->beast ONLY.
#       schema_version is KEPT (the schema tolerates it).
#   board.json (strict board-v1, pure plan):
#     - whitelist projection to {$schema, release, tracks:[{id,slices,depends_on}]}
#       — drops schema_version, release_worktree_path/branch, stray activity, and
#       every track state/worktree_path/worktree_branch/worktree. The release
#       object is itself whitelisted to {name,target_version,integration_branch,
#       vertical_trace} (a stray release.worktree is dropped).
#
# Postconditions (fail-closed, exit 1 if violated across the processed releases):
#   - zero '"quadrant": "chore"' / '"epic"' / '"feature"' in any *.json  [AC-01/AC-02]
#   - every spec.json carries in_scope AND out_of_scope                  [AC-03]
#   - no schema_version in any spec.json or board.json                   [AC-03/AC-06]
#
set -euo pipefail

ROOT="${1:-docs/release}"

if ! command -v jq >/dev/null 2>&1; then
  echo "migrate-records: jq is required but not on PATH" >&2
  exit 2
fi
if [ ! -d "$ROOT" ]; then
  echo "migrate-records: release root not found: $ROOT" >&2
  exit 2
fi

# --- jq filters -------------------------------------------------------------

SPEC_FILTER='
  del(.schema_version)
  | (if has("acceptance_criteria") then
       .acceptance_criteria |= map(
         . as $ac
         | ( $ac.ears_pattern
             // ( {
                   "ubiquitous":"ubiquitous",
                   "event-driven":"event-driven",
                   "state-driven":"state-driven",
                   "optional-feature":"optional-feature",
                   "complex":"complex",
                   "unwanted":"unwanted-behaviour",
                   "note":"note"
                 }[$ac.type] )
           ) as $pat
         | del(.type, .ears_keyword)
         | if $pat != null then .ears_pattern = $pat else . end
       )
     else . end)
  | (if (.effort_complexity | type) == "object" then
       .effort_complexity.quadrant |=
         (if . == "chore" then "quick" elif . == "epic" then "beast" else . end)
     else . end)
  | (if has("in_scope") then . else .in_scope = [] end)
  | (if has("out_of_scope") then . else .out_of_scope = [] end)
'

STATUS_FILTER='
  if (.effort_complexity | type) == "object" then
    .effort_complexity.quadrant |=
      (if . == "chore" then "quick" elif . == "epic" then "beast" else . end)
  else . end
'

BOARD_FILTER='
  (if has("$schema") then {"$schema": .["$schema"]} else {} end)
  + {release: (.release | with_entries(select(
       .key == "name" or .key == "target_version"
       or .key == "integration_branch" or .key == "vertical_trace")))}
  + {tracks: ((.tracks // []) | map(
       {id: .id, slices: .slices}
       + (if has("depends_on") then {depends_on: .depends_on} else {} end)
     ))}
'

# transform <file> <jq-filter> : rewrite file in place only when the content
# actually changes (keeps re-runs a true no-op and unchanged files un-churned).
transform() {
  local f="$1" filter="$2" tmp
  tmp="$(mktemp)"
  jq --indent 2 "$filter" "$f" > "$tmp"
  if cmp -s "$tmp" "$f"; then
    rm -f "$tmp"
  else
    mv "$tmp" "$f"
    echo "  migrated $f"
  fi
}

# status.json is only rewritten when its quadrant is a retired name, so the
# grind/puzzle/quick status.json files are never re-serialised (minimal blast).
status_needs_migration() {
  local q
  q="$(jq -r '.effort_complexity.quadrant // ""' "$1" 2>/dev/null || echo "")"
  [ "$q" = "chore" ] || [ "$q" = "epic" ]
}

# --- migrate ----------------------------------------------------------------

processed=0
skipped_legacy=0
processed_dirs=()
for rel in "$ROOT"/*/; do
  [ -d "$rel" ] || continue
  relname="$(basename "$rel")"
  # spec-v1-era gate: at least one spec.json anywhere under the release.
  if [ -z "$(find "$rel" -name spec.json -print -quit 2>/dev/null)" ]; then
    skipped_legacy=$((skipped_legacy + 1))
    continue
  fi
  echo "release $relname"
  processed=$((processed + 1))
  processed_dirs+=("$rel")

  # spec.json — always transform (all need the schema_version/AC reshape).
  while IFS= read -r -d '' f; do
    transform "$f" "$SPEC_FILTER"
  done < <(find "$rel" -name spec.json -print0)

  # status.json — quadrant rename only, and only when a retired name is present.
  while IFS= read -r -d '' f; do
    if status_needs_migration "$f"; then
      transform "$f" "$STATUS_FILTER"
    fi
  done < <(find "$rel" -name status.json -print0)

  # board.json — whitelist projection to pure-plan board-v1.
  if [ -f "$rel/board.json" ]; then
    transform "$rel/board.json" "$BOARD_FILTER"
  fi
done

# --- postconditions (fail-closed) -------------------------------------------
#
# Scoped to the PROCESSED (spec-v1-era) releases only. Legacy releases were
# deliberately skipped (Coach 2026-07-10) and keep their pre-v0.10.0 shape
# (schema_version, worktree fields) — asserting over them would wrongly fail.

fail=0
assert_zero() {
  local desc="$1" ; shift
  local hits
  hits="$("$@" || true)"
  if [ -n "$hits" ]; then
    echo "POSTCONDITION FAIL: $desc" >&2
    echo "$hits" | sed 's/^/  /' >&2
    fail=1
  fi
}

if [ "$processed" -eq 0 ]; then
  echo "---"
  echo "processed 0 spec-v1-era release(s); skipped $skipped_legacy legacy release(s)"
  echo "migrate-records: nothing to migrate (no spec-v1-era release under $ROOT)"
  exit 0
fi

# no retired/invalid quadrant names remain in any JSON record.
assert_zero "retired/invalid quadrant name in a *.json record" \
  grep -rln '"quadrant": *"\(chore\|epic\|feature\)"' "${processed_dirs[@]}" --include='*.json'

# no schema_version survives in spec.json or board.json.
assert_zero "schema_version still present in a spec.json/board.json" \
  grep -rln '"schema_version"' "${processed_dirs[@]}" --include=spec.json --include=board.json

# every spec.json carries in_scope AND out_of_scope.
missing_scope=""
while IFS= read -r -d '' f; do
  if ! jq -e 'has("in_scope") and has("out_of_scope")' "$f" >/dev/null 2>&1; then
    missing_scope="$missing_scope$f"$'\n'
  fi
done < <(find "${processed_dirs[@]}" -name spec.json -print0)
if [ -n "$missing_scope" ]; then
  echo "POSTCONDITION FAIL: spec.json missing in_scope/out_of_scope" >&2
  echo "$missing_scope" | sed 's/^/  /' >&2
  fail=1
fi

echo "---"
echo "processed $processed spec-v1-era release(s); skipped $skipped_legacy legacy release(s)"
if [ "$fail" -ne 0 ]; then
  echo "migrate-records: FAILED postconditions" >&2
  exit 1
fi
echo "migrate-records: OK — all postconditions satisfied"
