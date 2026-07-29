#!/usr/bin/env sh

set -eu

profile=${1:-coverage.out}
minimum=${2:-65.0}

if [ ! -f "$profile" ]; then
  printf 'coverage profile not found: %s\n' "$profile" >&2
  exit 2
fi

actual=$(go tool cover -func="$profile" | awk '/^total:/ { gsub(/%/, "", $3); print $3 }')
if [ -z "$actual" ]; then
  printf 'could not read total coverage from: %s\n' "$profile" >&2
  exit 2
fi

if ! awk -v actual="$actual" -v minimum="$minimum" 'BEGIN { exit !(actual + 0 >= minimum + 0) }'; then
  printf 'coverage %.1f%% is below required %.1f%%\n' "$actual" "$minimum" >&2
  exit 1
fi

printf 'coverage %.1f%% meets required %.1f%%\n' "$actual" "$minimum"
