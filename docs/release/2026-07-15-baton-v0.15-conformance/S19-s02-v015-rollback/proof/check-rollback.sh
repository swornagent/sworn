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
readonly S19_SPEC="${S19_ROOT}spec.json"
readonly S02_STATUS="${S02_ROOT}status.json"
readonly S20_STATUS="${RELEASE_ROOT}${S20_ID}/status.json"
readonly INDEX_PATH="${RELEASE_ROOT}index.md"
readonly AMENDMENT_SCHEMA="${S19_ROOT}proof/contract-amendment-v1.schema.json"
readonly AMENDMENT_RECORD="${S19_ROOT}proof/contract-amendment.json"
# These planner-ratified evidence records are immutable input to this repair.
# Pinning their committed objects prevents a later proof-file edit from becoming
# a mutable-spec or rendered-index waiver.
readonly AMENDMENT_SCHEMA_BLOB='b62d48f698059fc0151ea0a3b9da18dfe1e507f5'
readonly AMENDMENT_RECORD_BLOB='9e298676129ee628714ffa80caa8c02bcea244f7'
readonly EXPECTED_CONTROL_PATHS=45
readonly EXPECTED_CONTROL_ABSENCES=8

contract_json=''

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

validate_contract_amendment() {
  local schema_blob record_blob schema_json source_head source_status_path
  local source_status_blob source_checker_blob trigger_head trigger_status_blob

  schema_blob=$(git rev-parse "${current_track_head}:${AMENDMENT_SCHEMA}") || fail 'contract amendment schema is absent from the current track head'
  record_blob=$(git rev-parse "${current_track_head}:${AMENDMENT_RECORD}") || fail 'contract amendment record is absent from the current track head'
  [[ "$schema_blob" == "$AMENDMENT_SCHEMA_BLOB" ]] || fail 'contract amendment schema blob differs from the planner-ratified v1 schema'
  [[ "$record_blob" == "$AMENDMENT_RECORD_BLOB" ]] || fail 'contract amendment record blob differs from the planner-ratified record'

  schema_json=$(git show "${current_track_head}:${AMENDMENT_SCHEMA}") || fail 'cannot read contract amendment schema'
  contract_json=$(git show "${current_track_head}:${AMENDMENT_RECORD}") || fail 'cannot read contract amendment record'

  # Validate the complete record through the committed v1 schema rather than
  # trusting a field-by-field shell allowlist. The schema uses only this compact
  # JSON-Schema subset; unsupported references or constraints fail closed.
  if ! jq -en --argjson schema "$schema_json" --argjson document "$contract_json" '
    def resolve($root; $ref):
      if (($ref | type) != "string") or (($ref | startswith("#/")) | not) then
        error("unsupported schema reference")
      else
        reduce ($ref | ltrimstr("#/") | split("/"))[] as $part ($root; .[$part])
      end;
    def resolved($root; $schema):
      if ($schema | has("$ref")) then resolve($root; $schema["$ref"]) else $schema end;
    def type_ok($schema; $value):
      if ($schema.type? == null) then true
      elif $schema.type == "object" then ($value | type) == "object"
      elif $schema.type == "array" then ($value | type) == "array"
      elif $schema.type == "string" then ($value | type) == "string"
      elif $schema.type == "integer" then (($value | type) == "number" and ($value | floor) == $value)
      elif $schema.type == "boolean" then ($value | type) == "boolean"
      else false
      end;
    def valid($root; $schema; $value):
      resolved($root; $schema) as $s
      | (type_ok($s; $value)
         and (if ($s | has("const")) then $value == $s.const else true end)
         and (if $s.type == "object" then
                (($s.required // []) | all(.[]; . as $key | ($value | has($key))))
                and (($s.properties // {}) | to_entries | all(.[]; . as $entry |
                  if ($value | has($entry.key)) then valid($root; $entry.value; $value[$entry.key]) else true end))
                and (if $s.additionalProperties == false then
                       ((($value | keys) - (($s.properties // {}) | keys)) | length == 0)
                     else true end)
              else true end)
         and (if $s.type == "array" then
                (($s.minItems // 0) as $min | ($value | length) >= $min)
                and (if ($s | has("maxItems")) then ($value | length) <= $s.maxItems else true end)
                and (($s.prefixItems // []) as $prefix |
                     [range(0; ($prefix | length))] | all(.[]; . as $i | valid($root; $prefix[$i]; $value[$i])))
                and (if ($s.items? == false) then ($value | length) == (($s.prefixItems // []) | length) else true end)
              else true end)
         and (if $s.type == "string" and ($s | has("minLength")) then ($value | length) >= $s.minLength else true end)
         and (if $s.type == "string" and ($s | has("pattern")) then ($value | test($s.pattern)) else true end)
         and (if $s.format? == "date-time" then ($value | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$")) else true end));
    ($schema."$schema" == "https://json-schema.org/draft/2020-12/schema")
    and ($schema."$id" == "https://swornagent.dev/schemas/s19-executable-proof-contract-amendment-v1.json")
    and ($schema.type == "object")
    and ($schema.additionalProperties == false)
    and valid($schema; $schema; $document)
  ' >/dev/null; then
    fail 'contract amendment record does not validate against the planner-ratified v1 schema'
  fi

  source_head=$(printf '%s' "$contract_json" | jq -r '.source.track_head')
  source_status_path=$(printf '%s' "$contract_json" | jq -r '.source.status_path')
  source_status_blob=$(printf '%s' "$contract_json" | jq -r '.source.status_blob_oid')
  source_checker_blob=$(printf '%s' "$contract_json" | jq -r '.checker_repair.checker_blob_before_repair')
  trigger_head=$(printf '%s' "$contract_json" | jq -r '.allowed_rendered_index_validation.trigger.blocked_track_head')
  trigger_status_blob=$(printf '%s' "$contract_json" | jq -r '.allowed_rendered_index_validation.trigger.blocked_status_blob_oid')

  git merge-base --is-ancestor "$source_head" "$current_track_head" || fail 'amendment source blocked verdict is not reachable from the current track head'
  [[ $(git rev-parse "${source_head}:${source_status_path}") == "$source_status_blob" ]] || fail 'amendment source status blob does not match its declared provenance'
  git show "${source_head}:${source_status_path}" | jq -e '.verification.result == "blocked"' >/dev/null || fail 'amendment source does not contain the declared blocked verdict'
  [[ $(git rev-parse "${source_head}:${S19_ROOT}proof/check-rollback.sh") == "$source_checker_blob" ]] || fail 'amendment source checker blob does not match its declared pre-repair identity'

  git merge-base --is-ancestor "$trigger_head" "$current_track_head" || fail 'render correction trigger is not reachable from the current track head'
  [[ $(git rev-parse "${trigger_head}:${S19_STATUS}") == "$trigger_status_blob" ]] || fail 'render correction trigger status blob does not match its declared provenance'
  git show "${trigger_head}:${S19_STATUS}" | jq -e '.verification.result == "blocked"' >/dev/null || fail 'render correction trigger does not contain the declared blocked verdict'

  printf 'CONTRACT_AMENDMENT PASS schema=%s record=%s\n' "$schema_blob" "$record_blob"
}

spec_state() {
  local tuple=$1 expected_mode=$2 first_before=$3 first_after=$4 second_after=$5
  local mode object_id
  IFS=$'\t' read -r mode object_id <<< "$tuple"
  [[ "$mode" == "$expected_mode" ]] || fail "S19 spec mode drifted to ${mode}"
  case "$object_id" in
    "$first_before") printf '0\n' ;;
    "$first_after") printf '1\n' ;;
    "$second_after") printf '2\n' ;;
    *) fail "unrecognized post-start S19 spec blob ${object_id}" ;;
  esac
}

validate_s19_spec_history() {
  local first_before first_after first_count first_pre first_subject
  local second_before second_after second_count second_pre second_subject
  local start_tuple spec_mode start_oid commit parent_line subject before_tuple after_tuple
  local -a parents=()
  local parent_one_state parent_two_state result_state
  local first_commit='' second_commit='' first_propagations=0 second_propagations=0

  first_before=$(printf '%s' "$contract_json" | jq -r '.allowed_post_start_spec_transition.before_blob_oid')
  first_after=$(printf '%s' "$contract_json" | jq -r '.allowed_post_start_spec_transition.after_blob_oid')
  first_count=$(printf '%s' "$contract_json" | jq -r '.allowed_post_start_spec_transition.allowed_transition_count')
  first_pre=$(printf '%s' "$contract_json" | jq -r '.ratification.pre_ratification_release_wt_head')
  first_subject=$(printf '%s' "$contract_json" | jq -r '.ratification.required_planner_commit_subject')
  second_before=$(printf '%s' "$contract_json" | jq -r '.allowed_rendered_index_validation.correction_spec_transition.before_blob_oid')
  second_after=$(printf '%s' "$contract_json" | jq -r '.allowed_rendered_index_validation.correction_spec_transition.after_blob_oid')
  second_count=$(printf '%s' "$contract_json" | jq -r '.allowed_rendered_index_validation.correction_spec_transition.allowed_transition_count')
  second_pre=$(printf '%s' "$contract_json" | jq -r '.allowed_rendered_index_validation.correction_spec_transition.pre_ratification_release_wt_head')
  second_subject=$(printf '%s' "$contract_json" | jq -r '.allowed_rendered_index_validation.correction_spec_transition.required_planner_commit_subject')

  [[ "$first_count" == '1' && "$second_count" == '1' ]] || fail 'contract amendment does not declare exactly one transition for each S19 spec correction'
  [[ "$second_before" == "$first_after" ]] || fail 'contract amendment does not form a contiguous S19 spec-transition chain'

  start_tuple=$(tree_tuple "$S19_START" "$S19_SPEC")
  IFS=$'\t' read -r spec_mode start_oid <<< "$start_tuple"
  [[ "$spec_mode" != '-' && "$start_oid" == "$first_before" ]] || fail 'S19 start spec does not match the first declared amendment baseline'

  while IFS= read -r commit; do
    parent_line=$(git show -s --format='%P' "$commit")
    read -r -a parents <<< "$parent_line"
    [[ ${#parents[@]} -ge 1 ]] || fail "S19 spec history contains a parentless post-start commit ${commit}"
    after_tuple=$(tree_tuple "$commit" "$S19_SPEC")
    before_tuple=$(tree_tuple "${parents[0]}" "$S19_SPEC")
    if [[ ${#parents[@]} -eq 1 && "$after_tuple" == "$before_tuple" ]]; then
      continue
    fi
    if [[ ${#parents[@]} -eq 2 ]]; then
      local parent_two_tuple
      parent_two_tuple=$(tree_tuple "${parents[1]}" "$S19_SPEC")
      [[ "$after_tuple" == "$before_tuple" && "$after_tuple" == "$parent_two_tuple" ]] && continue
    fi

    if [[ ${#parents[@]} -eq 1 ]]; then
      subject=$(git show -s --format='%s' "$commit")
      if [[ "$subject" == "$first_subject" ]]; then
        [[ -z "$first_commit" ]] || fail 'duplicate first planner-ratified S19 spec transition'
        [[ "${parents[0]}" == "$first_pre" ]] || fail 'first S19 spec transition has the wrong planner provenance parent'
        [[ "$before_tuple" == "${spec_mode}"$'\t'"${first_before}" && "$after_tuple" == "${spec_mode}"$'\t'"${first_after}" ]] || fail 'first S19 spec transition has the wrong mode or blob identity'
        first_commit=$commit
      elif [[ "$subject" == "$second_subject" ]]; then
        [[ -n "$first_commit" && -z "$second_commit" ]] || fail 'second S19 spec transition is duplicate or precedes the first transition'
        [[ "${parents[0]}" == "$second_pre" && "$second_pre" == "$first_commit" ]] || fail 'second S19 spec transition has the wrong planner provenance parent'
        [[ "$before_tuple" == "${spec_mode}"$'\t'"${second_before}" && "$after_tuple" == "${spec_mode}"$'\t'"${second_after}" ]] || fail 'second S19 spec transition has the wrong mode or blob identity'
        second_commit=$commit
      else
        fail "unrecognized ordinary S19 spec transition ${commit}"
      fi
      continue
    fi

    [[ ${#parents[@]} -eq 2 ]] || fail "S19 spec history contains an unrecognized multi-parent transition ${commit}"
    parent_one_state=$(spec_state "$before_tuple" "$spec_mode" "$first_before" "$first_after" "$second_after")
    parent_two_state=$(spec_state "$(tree_tuple "${parents[1]}" "$S19_SPEC")" "$spec_mode" "$first_before" "$first_after" "$second_after")
    result_state=$(spec_state "$after_tuple" "$spec_mode" "$first_before" "$first_after" "$second_after")
    case "$result_state" in
      1)
        [[ "$parent_one_state" == '0' && "$parent_two_state" == '1' ]] || fail "first S19 spec propagation merge ${commit} has unexpected provenance"
        ((first_propagations += 1))
        ;;
      2)
        [[ "$parent_one_state" == '1' && "$parent_two_state" == '2' ]] || fail "second S19 spec propagation merge ${commit} has unexpected provenance"
        ((second_propagations += 1))
        ;;
      *)
        fail "S19 spec propagation merge ${commit} does not carry a declared amended state"
        ;;
    esac
  done < <(git rev-list --topo-order --reverse "${S19_START}..${current_track_head}")

  [[ -n "$first_commit" && -n "$second_commit" ]] || fail 'S19 spec history is missing a planner-ratified amendment transition'
  [[ "$first_propagations" -eq 1 && "$second_propagations" -eq 1 ]] || fail 'S19 spec history does not contain exactly one provenance-preserving propagation for each amendment'
  printf 'S19_SPEC_HISTORY PASS first=%s second=%s\n' "$first_commit" "$second_commit"
}

validate_s02_record_history() {
  local commit parent changed_paths checked_commits=0

  # S02 records are append-only from S19's start. Compare only each T1
  # first-parent transition: a propagation merge may legitimately differ from
  # parent two while retaining the exact S02 bytes already present on parent one.
  git merge-base --is-ancestor "$S19_START" "$current_track_head" || fail 'S19 start checkpoint is not an ancestor of the current T1 track head'
  while IFS= read -r commit; do
    parent=$(git rev-parse --verify "${commit}^1") || fail "S02 record history contains a parentless post-start commit ${commit}"
    ((checked_commits += 1))
    if ! git diff --quiet --no-ext-diff "$parent" "$commit" -- "$S02_ROOT"; then
      changed_paths=$(git diff --name-status --no-ext-diff "$parent" "$commit" -- "$S02_ROOT" | tr '\n' ';')
      fail "S02 release record transition on T1 first-parent history at ${commit}: ${changed_paths}"
    fi
  done < <(git rev-list --first-parent --reverse "${S19_START}..${current_track_head}")

  printf 'S02_RECORD_HISTORY PASS first-parent-commits=%s\n' "$checked_commits"
}

remove_disposable_worktree() {
  local worktree_path=$1
  [[ -n "$worktree_path" && -d "$worktree_path" ]] || return 0
  git worktree remove --force "$worktree_path" >/dev/null 2>&1 || rm -rf "$worktree_path"
}

validate_rendered_index() {
  local worktree_path='' status_before status_after refs_before refs_after render_output

  git cat-file -e "${current_track_head}:${INDEX_PATH}" || fail 'current track head has no committed rendered release index'
  command -v sworn >/dev/null 2>&1 || fail 'sworn render is unavailable for deterministic rendered-index validation'
  status_before=$(git status --porcelain)
  refs_before=$(git for-each-ref --format='%(refname) %(objectname)' 'refs/heads/release/*' 'refs/heads/release-wt/*')
  worktree_path=$(mktemp -d "${TMPDIR:-/tmp}/s19-render.XXXXXX") || fail 'cannot allocate disposable rendered-index worktree path'
  rmdir "$worktree_path" || fail 'cannot prepare disposable rendered-index worktree path'

  if ! git worktree add --detach "$worktree_path" "$current_track_head" >/dev/null 2>&1; then
    fail 'cannot create disposable detached worktree for rendered-index validation'
  fi
  if ! render_output=$(sworn render "$RELEASE" "$worktree_path" 2>&1); then
    remove_disposable_worktree "$worktree_path"
    fail "deterministic rendered-index command failed: ${render_output}"
  fi
  if ! cmp -s "${worktree_path}/${INDEX_PATH}" <(git show "${current_track_head}:${INDEX_PATH}"); then
    remove_disposable_worktree "$worktree_path"
    fail 'committed release index does not byte-match the disposable current-head sworn render'
  fi
  remove_disposable_worktree "$worktree_path"

  status_after=$(git status --porcelain)
  refs_after=$(git for-each-ref --format='%(refname) %(objectname)' 'refs/heads/release/*' 'refs/heads/release-wt/*')
  [[ "$status_before" == "$status_after" ]] || fail 'deterministic rendered-index validation mutated the validated checkout'
  [[ "$refs_before" == "$refs_after" ]] || fail 'deterministic rendered-index validation mutated a release ref'
  printf 'RENDERED_INDEX PASS head=%s\n' "$current_track_head"
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
validate_s02_record_history

changed_release_records=0
s19_spec_record_changes=0
rendered_index_record_changes=0
while IFS= read -r -d '' file_path; do
  ((changed_release_records += 1))
  case "$file_path" in
    "$S19_SPEC")
      ((s19_spec_record_changes += 1))
      ;;
    "$INDEX_PATH")
      ((rendered_index_record_changes += 1))
      ;;
    *)
      is_allowed_s19_record "$file_path" || fail "non-S19 or non-lifecycle release record changed after S19 start: ${file_path}"
      ;;
  esac
done < <(git diff --name-only -z "$S19_START" "$current_track_head" -- "$RELEASE_ROOT")

[[ "$s19_spec_record_changes" -eq 1 ]] || fail 'S19 spec amendment is absent or appears more than once in the post-start release-record diff'
[[ "$rendered_index_record_changes" -eq 1 ]] || fail 'rendered release index amendment is absent or appears more than once in the post-start release-record diff'
validate_contract_amendment
validate_s19_spec_history
validate_rendered_index

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
