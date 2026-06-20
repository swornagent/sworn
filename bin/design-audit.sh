#!/usr/bin/env bash
#
# bin/design-audit.sh — deterministic design-conformance first-pass
#
# Thin wrapper around `sworn designaudit <project-dir>`. Runs the machine-
# detectable drift check (hardcoded hex colours, off-scale spacing/borders,
# recreated components). Does NOT require a human cohesion verdict — the
# deterministic pass is the CI-appropriate gate; the cohesion verdict is a
# human-owned step run alongside.
#
# Usage: bin/design-audit.sh <project-dir> [--cohesion on-brand|off-brand]
#
# Exits 0 when the deterministic pass is clean (and cohesion is supplied if
# --cohesion is provided). Exits 1 on any machine-detectable violation.
# Exits 64 on usage error.

set -euo pipefail

if [ $# -lt 1 ]; then
	echo "Usage: $0 <project-dir> [--cohesion on-brand|off-brand]" >&2
	exit 64
fi

exec sworn designaudit "$@"
