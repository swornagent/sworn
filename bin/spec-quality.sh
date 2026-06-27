#!/usr/bin/env bash
#
# bin/spec-quality.sh — first-pass deterministic spec-quality check
#
# Thin wrapper around `sworn specquality <release>`. Invoked by the CI merge
# gate or by an implementer before requesting verification. Exits 0 when every
# slice meets the completeness threshold; exits 1 on any violation.
#
# Usage: bin/spec-quality.sh <release> [--threshold <0.0-1.0>]
#
# The threshold defaults to 0.5 (50%) — a slice whose acceptance examples
# catch fewer than 50% of output mutations is flagged.

set -euo pipefail

if [ $# -lt 1 ]; then
	echo "Usage: $0 <release> [--threshold <0.0-1.0>]" >&2
	exit 64
fi

exec sworn specquality "$@"