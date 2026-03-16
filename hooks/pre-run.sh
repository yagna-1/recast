#!/usr/bin/env bash
set -euo pipefail

if [[ ! -d testdata ]]; then
  echo "expected testdata/ directory missing" >&2
  exit 1
fi

echo "pre-run checks passed"
