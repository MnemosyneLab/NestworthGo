#!/bin/bash
# User-facing installer; works with macOS's system Bash 3.2 and standard tools.
set -euo pipefail

usage() {
  cat <<'EOF'
Install or update the Nestworth skill for local Codex (not the MCP connection).

  bash install-nestworth-skill.sh [options]

  --source PATH        Local skill directory, extracted bundle, or .tar.gz bundle
  --version TAG        Download from a GitHub release tag, or "latest"
  --skills-dir PATH    Codex skills root (default: $CODEX_HOME/skills,
                       or $HOME/.codex/skills when CODEX_HOME is unset)
  --sha256 HEX         Expected archive SHA-256; otherwise require its .sha256 file
  --dry-run            Validate and describe changes without modifying the target
  --status             Show installed version and location; no downloads/writes
  --replace-unmanaged  Back up and replace an existing skill not owned by installer
  --help               Show this help

No arguments uses the skill next to this script in a checkout/extracted bundle.
The same command updates an installation. Old versions/local edits are backed up
outside the skills root. Only the nestworth skill is replaced. No tokens, Codex
config, App data or other skills are changed. Releases must contain the assets
nestworth-skill.tar.gz and nestworth-skill.tar.gz.sha256.
EOF
}

die() { printf 'Error: %s\n' "$*" >&2; exit 1; }
need_value() { [ "$#" -ge 2 ] && [ -n "$2" ] || die "$1 requires a value"; }
hash_file() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  elif command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    die 'SHA-256 requires shasum or sha256sum'
  fi
}

source_path='' release_tag='' expected_hash=''
skills_root="${CODEX_HOME:-$HOME/.codex}/skills"
dry_run=false status=false replace_unmanaged=false
while [ "$#" -gt 0 ]; do
  case "$1" in
    --source) need_value "$@"; source_path=$2; shift 2 ;;
    --version) need_value "$@"; release_tag=$2; shift 2 ;;
    --skills-dir) need_value "$@"; skills_root=$2; shift 2 ;;
    --sha256) need_value "$@"; expected_hash=$2; shift 2 ;;
    --dry-run) dry_run=true; shift ;;
    --status) status=true; shift ;;
    --replace-unmanaged) replace_unmanaged=true; shift ;;
    --help|-h) usage; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done
[ -z "$source_path" ] || [ -z "$release_tag" ] || die 'choose --source or --version'
[ -z "$expected_hash" ] || [[ "$expected_hash" =~ ^[0-9a-fA-F]{64}$ ]] || die 'invalid SHA-256'
[ -n "$skills_root" ] && [ "$skills_root" != / ] || die 'invalid skills root'
case "$skills_root" in /*) ;; *) skills_root="$PWD/$skills_root" ;; esac
skills_root=${skills_root%/}
destination="$skills_root/nestworth"
marker='.nestworth-install-info'

if $status; then
  if [ -f "$destination/VERSION" ] && [ ! -L "$destination" ]; then
    printf 'Nestworth skill %s\nLocation: %s\n' "$(cat "$destination/VERSION")" "$destination"
    [ ! -f "$destination/$marker" ] || cat "$destination/$marker"
  else
    printf 'Nestworth skill is not installed at %s\n' "$destination"
  fi
  exit 0
fi

work='' stage='' lock='' backup_path='' installed=false
cleanup() {
  # Restore the previous installation if replacement failed after its rename.
  if ! $installed && [ -n "$backup_path" ] && [ -d "$backup_path" ] && [ ! -e "$destination" ]; then
    mv "$backup_path" "$destination" || printf 'Restore manually from %s\n' "$backup_path" >&2
  fi
  [ -z "$stage" ] || rm -rf "$stage"
  [ -z "$work" ] || rm -rf "$work"
  [ -z "$lock" ] || rmdir "$lock"
  return 0
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM HUP
work=$(mktemp -d "${TMPDIR:-/tmp}/nestworth-skill.XXXXXX")

if [ -n "$release_tag" ]; then
  [[ "$release_tag" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || die 'invalid release tag'
  command -v curl >/dev/null 2>&1 || die 'release download requires curl'
  base='https://github.com/MnemosyneLab/NestworthGo/releases'
  if [ "$release_tag" = latest ]; then base="$base/latest/download"; else base="$base/download/$release_tag"; fi
  source_path="$work/nestworth-skill.tar.gz"
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
    --connect-timeout 15 --max-time 120 --max-filesize 5242880 "$base/nestworth-skill.tar.gz" -o "$source_path"
  if [ -z "$expected_hash" ]; then
    curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
      --connect-timeout 15 --max-time 120 --max-filesize 4096 "$base/nestworth-skill.tar.gz.sha256" -o "$source_path.sha256"
  fi
elif [ -z "$source_path" ]; then
  script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
  if [ -d "$script_dir/skills/nestworth" ]; then source_path="$script_dir/skills/nestworth"
  else source_path="$script_dir/../skills/nestworth"; fi
fi
[ -z "$expected_hash" ] || [ ! -d "$source_path" ] || die '--sha256 applies to an archive, not a directory'

if [ -f "$source_path" ]; then
  [ "$(wc -c < "$source_path" | tr -d ' ')" -le 5242880 ] || die 'bundle exceeds 5 MiB'
  if [ -z "$expected_hash" ]; then
    [ -f "$source_path.sha256" ] || die 'archive requires adjacent .sha256 or --sha256'
    [ "$(wc -l < "$source_path.sha256" | tr -d ' ')" -eq 1 ] || die 'invalid checksum file'
    expected_hash=$(awk '{print $1}' "$source_path.sha256")
  fi
  [[ "$expected_hash" =~ ^[0-9a-fA-F]{64}$ ]] || die 'invalid SHA-256'
  actual_hash=$(hash_file "$source_path")
  [ "$actual_hash" = "$(printf '%s' "$expected_hash" | tr 'A-F' 'a-f')" ] || die 'archive checksum mismatch'
  tar -tzf "$source_path" > "$work/members"
  while IFS= read -r member; do
    member=${member%/}
    [[ "$member" =~ ^nestworth-skill(/[A-Za-z0-9._/-]+)?/?$ ]] || die "unexpected archive path: $member"
    case "/$member/" in *'/../'*|*'/./'*|*'//'*) die "unsafe archive path: $member" ;; esac
  done < "$work/members"
  tar -tvzf "$source_path" | awk '$1 !~ /^[-d]/ {bad=1} END {exit bad}' || die 'archive links/special files are not supported'
  mkdir "$work/extracted"
  tar -xzf "$source_path" -C "$work/extracted"
  source_path="$work/extracted/nestworth-skill/skills/nestworth"
fi
[ -d "$source_path" ] && [ ! -L "$source_path" ] || die 'source skill/bundle directory not found'
if [ ! -f "$source_path/SKILL.md" ] && [ -d "$source_path/skills/nestworth" ]; then
  source_path="$source_path/skills/nestworth"
fi
source_path=$(cd "$source_path" && pwd -P)
[ -f "$source_path/SKILL.md" ] && [ -f "$source_path/VERSION" ] && [ -f "$source_path/agents/openai.yaml" ] || die 'incomplete skill payload'
[ -z "$(find "$source_path" ! -type d ! -type f -print)" ] || die 'payload must contain only regular files/directories'
[ -z "$(find "$source_path" -type f -links +1 -print)" ] || die 'payload hard links are not supported'
find "$source_path" -type f -print > "$work/files"
while IFS= read -r file; do
  relative=${file#"$source_path/"}
  [[ "$relative" =~ ^[A-Za-z0-9._/-]+$ ]] || die 'unsupported payload filename'
  case "$relative" in
    SKILL.md|VERSION|agents/openai.yaml) ;;
    references/*.md) [ "${relative#references/}" = "$(basename "$relative")" ] || die 'nested references are not supported' ;;
    *) die "unexpected payload file: $relative" ;;
  esac
done < "$work/files"
grep -qx 'name: nestworth' "$source_path/SKILL.md" || die 'payload is not the nestworth skill'
version=$(cat "$source_path/VERSION")
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9.-]+)?$ ]] || die 'invalid skill version'
[ ! -L "$destination" ] || die 'destination skill is a symbolic link'
if [ -e "$destination" ]; then
  [ -d "$destination" ] || die 'destination skill is not a directory'
  if ! grep -qx 'managed_by=nestworth-skill-installer' "$destination/$marker" 2>/dev/null && ! $replace_unmanaged; then
    die 'existing skill is unmanaged; use --replace-unmanaged to preserve a backup and replace it'
  fi
fi

trees_equal() {
  [ -d "$destination" ] || return 1
  local source_count destination_count file relative
  source_count=$(wc -l < "$work/files" | tr -d ' ')
  destination_count=$(find "$destination" ! -type d ! -path "$destination/$marker" -print | wc -l | tr -d ' ')
  [ "$source_count" = "$destination_count" ] || return 1
  while IFS= read -r file; do
    relative=${file#"$source_path/"}
    [ -f "$destination/$relative" ] && [ ! -L "$destination/$relative" ] || return 1
    cmp -s "$file" "$destination/$relative" || return 1
  done < "$work/files"
}

if trees_equal; then
  printf 'Nestworth skill %s is already current at %s\n' "$version" "$destination"
  exit 0
fi
if $dry_run; then
  printf 'Would install Nestworth skill %s at %s\n' "$version" "$destination"
  [ ! -d "$destination" ] || printf 'Would preserve the previous directory, including local edits, outside the skills root.\n'
  exit 0
fi

mkdir -p "$skills_root"
# Resolve the actual root before deciding where to keep non-discoverable backups.
skills_root=$(cd "$skills_root" && pwd -P)
destination="$skills_root/nestworth"
lock_path="$skills_root/.nestworth-install.lock"
mkdir "$lock_path" 2>/dev/null || die "another install may be running; inspect $lock_path"
lock=$lock_path
# Recheck inside the install lock before replacing any directory.
[ ! -L "$destination" ] || die 'destination skill is a symbolic link'
if [ -e "$destination" ]; then
  [ -d "$destination" ] || die 'destination skill is not a directory'
  grep -qx 'managed_by=nestworth-skill-installer' "$destination/$marker" 2>/dev/null || $replace_unmanaged || die 'destination became unmanaged; rerun with --replace-unmanaged if intended'
fi
stage=$(mktemp -d "$skills_root/.nestworth-stage.XXXXXX")
cp -R "$source_path" "$stage/nestworth"
printf 'managed_by=nestworth-skill-installer\nversion=%s\n' "$version" > "$stage/nestworth/$marker"
if [ -d "$destination" ]; then
  backup_root="$(dirname "$skills_root")/nestworth-skill-backups"
  mkdir -p "$backup_root"
  backup_path="$backup_root/$(date -u +%Y%m%dT%H%M%SZ)-$$"
  mv "$destination" "$backup_path"
fi
mv "$stage/nestworth" "$destination"
installed=true
printf 'Installed Nestworth skill %s at %s\n' "$version" "$destination"
[ -z "$backup_path" ] || printf 'Previous skill and local edits: %s\n' "$backup_path"
printf 'Reopen your Codex chat if needed. Enable/connect MCP separately in the App.\n'
