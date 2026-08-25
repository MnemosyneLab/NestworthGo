#!/usr/bin/env bash
# Per-boot runtime setup for the Nestworth desktop app.
#
# Nestworth is a Fyne GUI application, so running it needs an X display. This
# script brings up a headless X server (Xvfb) on :99 plus a lightweight window
# manager (openbox) so agents can launch and drive the GUI. It is idempotent:
# it starts each service only if it is not already running, then returns.
#
# To run the app against this display:
#   export DISPLAY=:99
#   go run ./cmd/nestworth        # or ./bin/nestworth
set -euo pipefail

DISPLAY_NUM=":99"
export DISPLAY="${DISPLAY_NUM}"

# Runtime dir keeps GLFW/dbus from warning about a missing XDG_RUNTIME_DIR.
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/tmp/runtime-$(id -u)}"
mkdir -p "${XDG_RUNTIME_DIR}"
chmod 700 "${XDG_RUNTIME_DIR}"

# Headless X display for the Fyne desktop GUI.
if ! xdpyinfo -display "${DISPLAY_NUM}" >/dev/null 2>&1; then
  Xvfb "${DISPLAY_NUM}" -screen 0 1280x900x24 >/tmp/xvfb.log 2>&1 &
  for _ in $(seq 1 40); do
    if xdpyinfo -display "${DISPLAY_NUM}" >/dev/null 2>&1; then
      break
    fi
    sleep 0.25
  done
fi

# Lightweight window manager so app windows receive focus and activation,
# which keyboard/mouse-driven GUI testing depends on.
if ! pgrep -x openbox >/dev/null 2>&1; then
  openbox >/tmp/openbox.log 2>&1 &
fi

echo "Xvfb + openbox ready on ${DISPLAY_NUM}"
