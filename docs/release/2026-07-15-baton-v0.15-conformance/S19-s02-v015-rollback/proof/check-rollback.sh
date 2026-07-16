#!/usr/bin/env bash
# Proves S19's ordinary rollback boundary directly from committed Git objects.
# It deliberately derives paths from history; the count checks are controls, not
# an allow-list. Run from any directory inside this worktree.
set -euo pipefail

readonly BASE_COMMIT='e61cb190736ee7483fb4ed1a993442b26ce3574c'
readonly BASE_TREE='c57285e3f652e5f49aa8bb15e3ba65249b4a3db8'
readonly FROZEN_HEAD='2a17443d67d39cf681dba117a57673714a916d7f'
readonly S19_START='640396fa8cc319229d6f96dedfdbef65dbe317fe'
readonly RELEASE='2026-07-15-baton-v0.15-conformance'
readonly RELEASE_ROOT="docs/release/${RELEASE}/"
readonly S19_ID='S19-s02-v015-rollback'
readonly S02_ID='S02-v015-parity-and-installs'
readonly S20_ID='S20-v015-parity-portable-fixture'
readonly S19_ROOT="${RELEASE_ROOT}${S19_ID}/"
readonly S02_ROOT="${RELEASE_ROOT}${S02_ID}/"
readonly S19_STATUS="${S19_ROOT}status.json"
readonly S02_STATUS="${S02_ROOT}status.json"
readonly S20_STATUS="${RELEASE_ROOT}${S20_ID}/status.json"
readonly EXPECTED_CONTROL_PATHS=45
readonly EXPECTED_CONTROL_ABSENCES=8

usage() {
  printf 'usage: %s --head <implementation-commit> [--require-maintainability] [--require-proof-bundle] [--require-fresh-verifier]\n' "$0" >&2
  exit 64
}

fail() {
  printf 'ROLLBACK_CHECK FAIL: %s\n' "$*" >&2
  exit 1
}

is_release_record_path() {
  [[ "$1" == "${RELEASE_ROOT}"* ]]
}

is_allowed_s19_record() {
  case "$1" in
    "${S19_ROOT}status.json"|"${S19_ROOT}journal.md"|"${S19_ROOT}proof.json"|"${S19_ROOT}proof.md"|"${S19_ROOT}proof/"*|"${S19_ROOT}reports/maintainability/"*)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

tree_tuple() {
  local tree=$1
  local file_path=$2
  local entry mode kind object_id _name
  entry=$(git ls-tree "$tree" -- "$file_path")
  if [[ -z "$entry" ]]; then
    printf '%s\t%s\n' '-' '-'
    return
  fi
  IFS=$' \t' read -r mode kind object_id _name <<< "$entry"
  [[ "$kind" == 'blob' ]] || fail "non-blob envelope object at ${file_path}: ${kind}"
  printf '%s\t%s\n' "$mode" "$object_id"
}

head_arg=''
require_maintainability=0
require_proof_bundle=0
require_fresh_verifier=0
while (( $# > 0 )); do
  case "$1" in
    --head)
      (( $# >= 2 )) || usage
      head_arg=$2
      shift 2
      ;;
    --require-maintainability)
      require_maintainability=1
      shift
      ;;
    --require-proof-bundle)
      require_proof_bundle=1
      shift
      ;;
    --require-fresh-verifier)
      require_fresh_verifier=1
      require_maintainability=1
      require_proof_bundle=1
      shift
      ;;
    *)
      usage
      ;;
  esac
done
[[ -n "$head_arg" ]] || usage

repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"
head=$(git rev-parse --verify "${head_arg}^{commit}")
current_track_head=$(git rev-parse HEAD)

[[ $(git rev-parse "${BASE_COMMIT}^{tree}") == "$BASE_TREE" ]] || fail 'immutable S02 start tree does not resolve'
git merge-base --is-ancestor "$BASE_COMMIT" "$head" || fail 'S02 start_commit is not an ancestor of implementation head'
git merge-base --is-ancestor "$FROZEN_HEAD" "$head" || fail 'frozen S02 semantic head is not an ancestor of implementation head'
git merge-base --is-ancestor "$S19_START" "$head" || fail 'S19 start checkpoint is not an ancestor of implementation head'

declare -A ordinary_paths=()
declare -A merge_paths=()
declare -A later_semantic_commits=()
ordinary_commit_count=0
merge_commit_count=0

while IFS= read -r commit; do
  ((ordinary_commit_count += 1))
  while IFS= read -r -d '' file_path; do
    is_release_record_path "$file_path" && continue
    ordinary_paths["$file_path"]=1
    if [[ "$commit" != "$FROZEN_HEAD" ]] && git merge-base --is-ancestor "$FROZEN_HEAD" "$commit"; then
      git merge-base --is-ancestor "$S19_START" "$commit" || fail "unexpected later ordinary authority ${commit} before S19 start"
      [[ "$commit" == "$head" ]] || fail "unexpected later ordinary authority ${commit}; only the pinned implementation head may restore semantics"
      later_semantic_commits["$commit"]=1
    fi
  done < <(git diff-tree --no-commit-id --name-only -r -z --no-renames "${commit}^1" "$commit")
done < <(git rev-list --first-parent --reverse --no-merges "${BASE_COMMIT}..${head}")

while IFS= read -r merge; do
  ((merge_commit_count += 1))
  parent_one=''
  parent_two=''
  extra_parent=''
  read -r parent_one parent_two extra_parent < <(git show -s --format='%P' "$merge")
  [[ -n "$parent_one" && -n "$parent_two" && -z "$extra_parent" ]] || fail "unrecognized merge ${merge}: expected exactly two parents"
  git merge-base --is-ancestor "$parent_two" "release-wt/${RELEASE}" || fail "unrecognized merge ${merge}: parent two is not release-wt ancestry"
  while IFS= read -r -d '' file_path; do
    is_release_record_path "$file_path" && continue
    IFS=$'\t' read -r parent_two_mode parent_two_oid < <(tree_tuple "$parent_two" "$file_path")
    IFS=$'\t' read -r merge_mode merge_oid < <(tree_tuple "$merge" "$file_path")
    [[ "$parent_two_mode" == "$merge_mode" && "$parent_two_oid" == "$merge_oid" ]] || fail "unrecognized semantic merge ${merge} at ${file_path}: merge result is not parent-two exact"
    merge_paths["$file_path"]=1
  done < <(git diff-tree --no-commit-id --name-only -r -z --no-renames "${merge}^1" "$merge")
done < <(git rev-list --first-parent --reverse --merges "${BASE_COMMIT}..${head}")

for file_path in "${!merge_paths[@]}"; do
  [[ -z ${ordinary_paths["$file_path"]+present} ]] || fail "authored/merge overlap at ${file_path}"
done

[[ ${#later_semantic_commits[@]} -eq 1 ]] || fail 'rollback must contain exactly one post-frozen ordinary semantic restoration commit'
[[ -n ${later_semantic_commits["$head"]+present} ]] || fail 'pinned implementation head has no restoration authority'

mapfile -d '' -t envelope_paths < <(printf '%s\0' "${!ordinary_paths[@]}" | LC_ALL=C sort -z)
[[ ${#envelope_paths[@]} -eq "$EXPECTED_CONTROL_PATHS" ]] || fail "envelope control count ${#envelope_paths[@]} does not equal ${EXPECTED_CONTROL_PATHS}"

baseline_present=0
baseline_absent=0
for file_path in "${envelope_paths[@]}"; do
  IFS=$'\t' read -r base_mode base_oid < <(tree_tuple "$BASE_COMMIT" "$file_path")
  IFS=$'\t' read -r head_mode head_oid < <(tree_tuple "$head" "$file_path")
  [[ "$base_mode" == "$head_mode" && "$base_oid" == "$head_oid" ]] || fail "mode/blob/absence mismatch at ${file_path}: expected ${base_mode}/${base_oid}, got ${head_mode}/${head_oid}"
  if [[ "$base_mode" == '-' ]]; then
    ((baseline_absent += 1))
  else
    ((baseline_present += 1))
  fi
  printf 'ENVELOPE %s baseline=%s/%s head=%s/%s\n' "$file_path" "$base_mode" "$base_oid" "$head_mode" "$head_oid"
done

[[ "$baseline_absent" -eq "$EXPECTED_CONTROL_ABSENCES" ]] || fail "absence control count ${baseline_absent} does not equal ${EXPECTED_CONTROL_ABSENCES}"
[[ "$baseline_present" -eq $((EXPECTED_CONTROL_PATHS - EXPECTED_CONTROL_ABSENCES)) ]] || fail 'baseline-present control count is wrong'

git diff --exit-code "$BASE_COMMIT" "$head" -- . ":(exclude)${RELEASE_ROOT}**" >/dev/null || fail 'whole-tree non-release backstop found a semantic difference'
git diff --quiet "$S19_START" "$current_track_head" -- "$S02_ROOT" || fail 'S02 release evidence changed after S19 start'

changed_release_records=0
while IFS= read -r -d '' file_path; do
  ((changed_release_records += 1))
  is_allowed_s19_record "$file_path" || fail "non-S19 or non-lifecycle release record changed after S19 start: ${file_path}"
done < <(git diff --name-only -z "$S19_START" "$current_track_head" -- "$RELEASE_ROOT")

status_start=$(git show "${current_track_head}:${S19_STATUS}" | jq -r '.start_commit')
[[ "$status_start" == "$S19_START" ]] || fail 'S19 status start_commit changed or does not match its immutable start checkpoint'

git show "${current_track_head}:${S02_STATUS}" | jq -e --arg rollback "$S19_ID" '
  .state == "deferred" and
  .maintainability.state == "re_slice_required" and
  .maintainability.rollback_slice_id == $rollback
' >/dev/null || fail 'S02 is not retained as the rollback-backed terminal deferral'

s19_status_json=$(git show "${current_track_head}:${S19_STATUS}")
s20_status_json=$(git show "${current_track_head}:${S20_STATUS}")
status_json=$s19_status_json
if ! printf '%s' "$s20_status_json" | jq -e '
  .state == "planned" and
  .start_commit == null and
  .maintainability.state == "pending"
' >/dev/null; then
  # A successor may leave planned/pending only after the whole AC-05
  # conjunction is established: exact-head Implementer PASS, a complete proof
  # bundle, and independent fresh verifier evidence. Turning on the existing
  # strict checks here makes that a default transition gate, not caller choice.
  require_maintainability=1
  require_proof_bundle=1
  require_fresh_verifier=1
fi

if (( require_maintainability == 1 )); then
  report_count=$(printf '%s' "$status_json" | jq -r --arg h "$head" '
    [.maintainability.reports[] |
      select(.role == "implementer" and .phase == "preflight" and .cycle == 0 and .verdict == "PASS" and .review_scope_head == $h)
    ] | length
  ')
  [[ "$report_count" == '1' ]] || fail 'missing one final Implementer maintainability PASS bound to the implementation head'
  printf '%s' "$status_json" | jq -e --arg h "$head" '
    .maintainability.state == "passed" and
    .maintainability.cycle == 0 and
    .maintainability.implementation_head == $h
  ' >/dev/null || fail 'status does not bind maintainability PASS to the implementation head'
  report_path=$(printf '%s' "$status_json" | jq -r --arg h "$head" '
    .maintainability.reports[] |
    select(.role == "implementer" and .phase == "preflight" and .cycle == 0 and .verdict == "PASS" and .review_scope_head == $h) |
    .report_path
  ')
  report_blob=$(printf '%s' "$status_json" | jq -r --arg h "$head" '
    .maintainability.reports[] |
    select(.role == "implementer" and .phase == "preflight" and .cycle == 0 and .verdict == "PASS" and .review_scope_head == $h) |
    .report_blob_oid
  ')
  [[ $(git rev-parse "${current_track_head}:${report_path}") == "$report_blob" ]] || fail 'maintainability report blob does not match its status ledger entry'
  git show "${current_track_head}:${report_path}" | jq -e --arg h "$head" '
    .check == "maintainability-review" and
    .role == "implementer" and
    .phase == "preflight" and
    .cycle == 0 and
    .verdict == "PASS" and
    .review_scope.head == $h
  ' >/dev/null || fail 'maintainability report does not bind its review scope to the implementation head'
fi

if (( require_proof_bundle == 1 )); then
  proof_path="${S19_ROOT}proof.json"
  proof_markdown_path="${S19_ROOT}proof.md"
  git cat-file -e "${current_track_head}:${proof_path}" || fail 'proof.json is missing from the committed release record'
  git cat-file -e "${current_track_head}:${proof_markdown_path}" || fail 'proof.md is missing from the committed release record'
  proof_json=$(git show "${current_track_head}:${proof_path}")
  printf '%s' "$proof_json" | jq -e --arg slice "$S19_ID" --arg release "$RELEASE" '
    .slice_id == $slice and
    .release == $release and
    (.files_changed | type == "array") and
    (.reachability.type | type == "string") and
    (.reachability.evidence | type == "string" and length > 0) and
    (.delivered | type == "array" and length > 0) and
    (.not_delivered | type == "array") and
    (.divergence | type == "array")
  ' >/dev/null || fail 'proof bundle does not satisfy the required S19 Rule-6 shape'
  while IFS= read -r test_command; do
    printf '%s' "$proof_json" | jq -e --arg command "$test_command" '
      [.test_results[] | select(.command == $command and .passed == true)] | length > 0
    ' >/dev/null || fail "proof bundle lacks a passing required test: ${test_command}"
  done < <(printf '%s' "$status_json" | jq -r '.test_commands[]')
fi

if (( require_fresh_verifier == 1 )); then
  printf '%s' "$status_json" | jq -e --arg h "$head" '
    .state == "verified" and
    .maintainability.implementation_head == $h and
    .verification.result == "pass" and
    .verification.verifier_was_fresh_context == true and
    (.verification.verifier_verdict_at | type == "string" and length > 0)
  ' >/dev/null || fail 'fresh verifier evidence is absent or not bound to the implementation head'
fi

printf 'ROLLBACK_CHECK PASS\n'
printf 'BASE %s tree=%s\n' "$BASE_COMMIT" "$BASE_TREE"
printf 'IMPLEMENTATION_HEAD %s\n' "$head"
printf 'ORDINARY_COMMITS %s\n' "$ordinary_commit_count"
printf 'RECORD_ONLY_MERGES %s\n' "$merge_commit_count"
printf 'ENVELOPE_PATHS %s baseline-present=%s baseline-absent=%s\n' "${#envelope_paths[@]}" "$baseline_present" "$baseline_absent"
printf 'RELEASE_RECORD_CHANGES_AFTER_S19_START %s\n' "$changed_release_records"
if (( require_maintainability == 1 )); then
  printf 'MAINTAINABILITY_BINDING PASS head=%s report=%s\n' "$head" "$report_path"
fi
if (( require_proof_bundle == 1 )); then
  printf 'PROOF_BUNDLE_BINDING PASS path=%s\n' "$proof_path"
fi
if (( require_fresh_verifier == 1 )); then
  printf 'FRESH_VERIFIER_GATE PASS head=%s\n' "$head"
fi
