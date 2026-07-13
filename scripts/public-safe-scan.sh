#!/usr/bin/env bash
#
# public-safe-scan.sh — fail-closed guard that the public repo carries no
# private-repo, personal, dogfood-target, OR commercial-strategy leaks.
#
# WHY THIS EXISTS (Rule 12 — Guard Fidelity): the earlier S27 scrub shipped a
# grep guard scoped to internal/ + cmd/ only, so docs/ silently re-accumulated
# leaks. This guard's search domain is the WHOLE tracked tree (git grep over
# git ls-files) so the domain it checks equals the claim it backs.
#
# Two token classes:
#   1. IDENTITY leaks (private repo / personal / dogfood / home paths) — assembled
#      from fragments (e.g. 'get'.'fired') so this script's own source contains no
#      literal token and never self-matches. Excluded path: .gitignore, which must
#      literally name the private symlink path it blocks, so it cannot be
#      fragment-encoded the way the pattern definitions below are.
#   2. COMMERCIAL-strategy leaks (competitor analysis / pricing / monetisation).
#      These are proper nouns and phrases, written plainly for maintainability;
#      the ONE excluded path is this script itself, which necessarily enumerates
#      the patterns it searches for. Case-sensitive to avoid substring false hits.
#
# Exit 0 = clean (PASS). Exit 1 = any banned token found (FAIL).

set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

fail=0

# ---- class 1: identity leaks (fragment-assembled; scans everything incl. self) ----
t_dogfood='get''fired'
t_dogfood2='fired''au'
t_email='brad@''sawyer''\.net\.au'
t_priv1='sworn''-internal'
t_priv2='internal''-docs'
t_home1='/home/''brad'
t_home2='/Users/''brad'
ID_PAT="${t_dogfood}|${t_dogfood2}|${t_email}|${t_priv1}|${t_priv2}|${t_home1}|${t_home2}"

if hits=$(git grep -inE "$ID_PAT" -- ':!.gitignore' 2>/dev/null); then
  echo "PUBLIC-SAFE SCAN: FAIL — identity leaks in tracked files:" >&2
  echo "$hits" >&2; echo "" >&2
  fail=1
fi

# ---- class 2: commercial-strategy leaks (plain patterns; excludes only self) ----
# Case-sensitive (grep -nE, no -i): proper nouns + strategy phrases only.
COMM_PAT='OpenCode Zen|\bARR\b|pricing precedent|billing precedent|revenue surface|moneti[sz]ation|\bStripe\b|\bdunning\b|price point|protect margin|managed proxy model|\bSpaceX\b|Agent Compute Unit|outcome billing|take rate'

if hits=$(git grep -nE "$COMM_PAT" -- ':!scripts/public-safe-scan.sh' 2>/dev/null); then
  echo "PUBLIC-SAFE SCAN: FAIL — commercial-strategy content in tracked files:" >&2
  echo "$hits" >&2; echo "" >&2
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  echo "Resolve each (genericise / remove / move to the private repo) before publishing." >&2
  exit 1
fi

echo "PUBLIC-SAFE SCAN: PASS — no banned tokens in $(git ls-files | wc -l | tr -d ' ') tracked files."
