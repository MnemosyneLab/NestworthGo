#!/bin/bash
# Build the -run regex for one race-shard part.
#
# CI:
#   bash tools/race-shard-select.sh PART PARTS PACKAGE
#     Enumerates with: go test -race -list . PACKAGE
#     Prints only the -run regex on stdout.
#
# The same race build is required so a test that exists only in the race
# binary cannot be dropped. Runnable names are Test, Example, and Fuzz.
# Fuzz seed corpus entries are subtests of the Fuzz target, not separate
# -list lines; anchoring ^(FuzzName)$ still runs those seeds because go test
# splits -run on unbracketed slashes.
#
# Enumeration failure is not hidden. The list command writes to a file and
# its status is checked before any name is read. A non-zero status discards
# that file, including a partial list, and becomes this script's status.
#
# Narrow checks use the same selection path:
#   bash tools/race-shard-select.sh --names PART PARTS < listing.txt
#   bash tools/race-shard-select.sh --enumerate PART PARTS -- COMMAND...
set -euo pipefail

usage() {
  echo "usage: race-shard-select.sh [--names|--enumerate] PART PARTS [PACKAGE|-- COMMAND...]" >&2
  exit 2
}

# Run the list command into outfile. A failing command must not leave a
# usable name list for the caller: discard the file and return its status.
run_enumeration() {
  local outfile=$1
  shift
  local status
  set +e
  "$@" >"$outfile"
  status=$?
  set -e
  if [ "$status" -ne 0 ]; then
    rm -f "$outfile"
    echo "enumeration command failed with status $status; discarding partial list" >&2
    exit "$status"
  fi
}

mode=package
case "${1:-}" in
  --names)
    mode=names
    shift
    ;;
  --enumerate)
    mode=enumerate
    shift
    ;;
esac

part=${1:-}
parts=${2:-}
if [ -z "$part" ] || [ -z "$parts" ]; then
  usage
fi
shift 2

if ! [[ "$part" =~ ^[0-9]+$ && "$parts" =~ ^[0-9]+$ ]]; then
  echo "invalid part/parts: $part / $parts" >&2
  exit 2
fi
if [ "$parts" -lt 1 ] || [ "$part" -ge "$parts" ]; then
  echo "part $part is outside 0..$((parts - 1))" >&2
  exit 2
fi

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

case "$mode" in
  names)
    if [ "$#" -ne 0 ]; then
      usage
    fi
    cat >"$tmp"
    ;;
  enumerate)
    if [ "${1:-}" != "--" ] || [ "$#" -lt 2 ]; then
      usage
    fi
    shift
    run_enumeration "$tmp" "$@"
    ;;
  package)
    if [ "$#" -ne 1 ]; then
      usage
    fi
    run_enumeration "$tmp" go test -race -list . "$1"
    ;;
  *)
    usage
    ;;
esac

names=()
declare -A seen_names=()
while IFS= read -r line || [ -n "$line" ]; do
  case "$line" in
    Test*|Example*|Fuzz*)
      case "$line" in
        *[!A-Za-z0-9_]*)
          echo "refusing runnable name that is not a Go identifier: $line" >&2
          exit 1
          ;;
      esac
      if [[ -n "${seen_names[$line]+present}" ]]; then
        echo "duplicate runnable name in enumeration: $line" >&2
        exit 1
      fi
      seen_names[$line]=1
      names+=("$line")
      ;;
  esac
done <"$tmp"

if [ "${#names[@]}" -eq 0 ]; then
  echo "enumeration produced no runnable Test, Example, or Fuzz names" >&2
  exit 1
fi

weight() {
  # Historical race timings from PR #114. New tests default to one unit and
  # are included automatically. Keep known expensive fixtures apart while
  # balancing the remaining tests deterministically.
  case "$1" in
    TestHistoricalOverviewCancellationReleasesConnection) echo 380 ;;
    TestHistoricalOverviewDenseEntrySingleAndComparison) echo 124 ;;
    *) echo 1 ;;
  esac
}

# Longest-processing-time first, then lowest accumulated estimated cost.
# Name and shard index break ties, so every invocation assigns the same
# discovered test set identically. New names participate with unit weight.
ordered=()
while IFS=$'\t' read -r _ name; do
  ordered+=("$name")
done < <(for name in "${names[@]}"; do printf '%s\t%s\n' "$(weight "$name")" "$name"; done | LC_ALL=C sort -t $'\t' -k1,1nr -k2,2)

loads=()
selected_by_part=()
for ((i=0; i<parts; i++)); do
  loads[i]=0
  selected_by_part[i]=""
done
for name in "${ordered[@]}"; do
  target=0
  for ((i=1; i<parts; i++)); do
    if (( loads[i] < loads[target] )); then
      target=$i
    fi
  done
  loads[target]=$((loads[target] + $(weight "$name")))
  if [ -n "${selected_by_part[target]}" ]; then
    selected_by_part[target]+=$'\n'
  fi
  selected_by_part[target]+="$name"
done

selected=()
while IFS= read -r name; do
  [ -n "$name" ] && selected+=("$name")
done <<< "${selected_by_part[$part]}"

if [ "${#selected[@]}" -eq 0 ]; then
  echo "part $part/$parts selected no names from ${#names[@]} runnable" >&2
  exit 1
fi

regex='^('
for i in "${!selected[@]}"; do
  if [ "$i" -gt 0 ]; then
    regex+='|'
  fi
  regex+="${selected[$i]}"
done
regex+=')$'
printf '%s\n' "$regex"
echo "race-shard-select: part $((part + 1))/$parts selected ${#selected[@]} of ${#names[@]} runnable Test/Example/Fuzz names" >&2
