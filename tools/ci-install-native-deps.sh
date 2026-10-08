#!/usr/bin/env bash
# CI only. Retry acquisition, never a possibly partially completed dpkg run.
set -euo pipefail
export LC_ALL=C

log_dir=$(mktemp -d)
trap 'rm -rf "$log_dir"' EXIT
packages=(gcc pkg-config libgtk-4-dev libwebkitgtk-6.0-dev libsoup-3.0-dev)
options=(-o Acquire::Retries=0 -o Acquire::http::Timeout=30
  -o Acquire::https::Timeout=30 -o DPkg::Lock::Timeout=30)

diagnose() {
  echo '::group::Native dependency failure diagnostics'
  # Each diagnostic is bounded and best effort; it must not replace apt's status.
  timeout -k 2s 5s ps -eo pid,ppid,stat,etime,args || true
  sudo -n timeout -k 2s 5s dpkg --audit || true
  sudo -n timeout -k 2s 5s tail -n 60 /var/log/apt/term.log /var/log/dpkg.log || true
  echo '::endgroup::'
}

transient_network_failure() {
  local log=$1 fetch_error
  local transient='Temporary failure resolving|Could not resolve|Could not connect|Connection (failed|timed out|reset)|Could not handshake|Timeout was reached|[45][0-9][0-9] (Too Many Requests|Internal Server Error|Bad Gateway|Service Unavailable|Gateway Time)'
  # Unknown E: errors (locks, signatures, missing packages, dpkg, etc.) fail
  # immediately, even when a transport error is also present.
  if grep '^E:' "$log" | grep -Ev '^E: (Failed to fetch |Some index files failed to download|Unable to fetch some archives)' >/dev/null; then
    return 1
  fi
  if grep -Eiq 'Certificate verification failed|certificate issuer is unknown|Hash Sum mismatch|unauthenticated packages' "$log"; then
    return 1
  fi
  # A transient failure for one URL must not hide a permanent/unknown failure
  # for another. apt update may report acquisition failures as W: lines.
  while IFS= read -r fetch_error; do
    if ! [[ "$fetch_error" =~ $transient ]]; then
      return 1
    fi
  done < <(grep -E '^[EW]: Failed to fetch ' "$log")
  grep -Eq "$transient" "$log"
}

run_phase() {
  local phase=$1 budget=$2 attempts=$3
  shift 3
  local attempt status log
  local -a pipeline_status
  for ((attempt=1; attempt<=attempts; attempt++)); do
    log="$log_dir/$phase-$attempt.log"
    printf '%s phase=%s attempt=%s/%s budget=%ss\n' "$(date -u +%FT%TZ)" "$phase" "$attempt" "$attempts" "$budget"
    # timeout runs as root so both TERM and KILL reach apt and its children.
    # sudo is noninteractive. tee streams apt output; preserve both statuses.
    set +e
    sudo -n timeout --verbose -k 10s "${budget}s" env \
      DEBIAN_FRONTEND=noninteractive LC_ALL=C \
      apt-get "${options[@]}" "$@" 2>&1 | tee "$log"
    pipeline_status=("${PIPESTATUS[@]}")
    set -e
    status=${pipeline_status[0]}
    if ((pipeline_status[1] != 0)); then
      echo "::error::Cannot retain apt diagnostics (tee status ${pipeline_status[1]})"
      if ((status == 0)); then status=${pipeline_status[1]}; fi
      diagnose
      return "$status"
    fi
    printf '%s phase=%s attempt=%s exit=%s\n' "$(date -u +%FT%TZ)" "$phase" "$attempt" "$status"
    if ((status == 0)); then return 0; fi
    if ((attempt < attempts)) && { ((status == 124 || status == 137)) || { ((status == 100)) && transient_network_failure "$log"; }; }; then
      echo "::warning::Acquisition phase $phase failed ($status); retrying once after 10s"
      sleep 10
    else
      echo "::error::Native dependency phase $phase failed (exit $status)"
      diagnose
      return "$status"
    fi
  done
}

# Strict update avoids silently using stale indexes after a failed download.
# Download-only never configures packages, so acquisition can be safely retried.
# Worst case: 2*(120+10) + 2*(180+10) + (180+10) + 20 backoff
# + <=21 diagnostics = <=871s; workflow's 15m cap is the final backstop.
run_phase update 120 2 -o APT::Update::Error-Mode=any update
run_phase download 180 2 install --download-only -y "${packages[@]}"
run_phase configure 180 1 install --no-download -y "${packages[@]}"
