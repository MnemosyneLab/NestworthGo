# Gio desktop preview

This standalone Go module contains the Nestworth sidebar and a Gio application shell. Overview shows three sample balances. Directory contains a multiline input for testing input methods, editing, and selection. **Development → Component Preview** contains a sample form with an institution name, multiline notes, and a search of sample institutions. The form keeps its draft and submitted examples in memory.

The sidebar footer switches between Simplified Chinese and English, and between light and dark appearance. Labels use the message catalog in `localization.go` with an English fallback. Appearance colors come from `appearance.go`. Each preview launch starts in Simplified Chinese and light mode.

To check live switching, open the component preview, enter text in both fields, and select part of the notes. Switch language and appearance in both directions. The component preview stays open and keeps the draft and selection.

Type in the search field to filter the display-only results. Tab to its
clear button or click it to reset the query and focus the editor again.
The institution name allows a draft over 48 UTF-8 bytes and shows an error
until the draft fits. **Submit sample** advances the result counter only
when the name is valid. Return in the notes inserts a line break.

Use **About this form** to open a popover near the lower-right edge.
It opens above the button when space is tight below and follows it when
the window is resized. Escape, Tab, the Close button, and presses outside
the panel dismiss it and return focus to the trigger.

## macOS

On Apple Silicon with the repository's Go and Xcode toolchain, run from the repository root:

```sh
sh experiments/gio/preview-macos.sh
```

The preview launcher prepares an isolated synthetic fixture and presents the development `Nestworth.app` with bundle ID `com.nestworth.dev`. It prints the run directory containing the bundle and logs.

## Linux

Use Go 1.27.4 or newer, a C compiler, and pkg-config. The default build includes
X11 and Wayland support and needs their development libraries as well as EGL
and Vulkan headers. The launcher also uses Python 3 to verify the development
identity and the synthetic SQLite fixture. [Gio Linux requirements](https://gioui.org/doc/install/linux)

For example, on Debian 13 with Mesa:

```sh
sudo apt install gcc pkg-config python3 libwayland-dev libx11-dev \
  libx11-xcb-dev libxkbcommon-x11-dev libxcursor-dev libxfixes-dev \
  libegl-dev libvulkan-dev libgles2 libegl-mesa0 libgl1-mesa-dri
```

From a clean checkout, build and run the Gio tests without a desktop session:

```sh
cd experiments/gio
build_dir=$(mktemp -d)
CGO_ENABLED=1 go build -mod=readonly -o "$build_dir/Nestworth" .
CGO_ENABLED=1 go test -mod=readonly ./...
```

To open the preview, run the launcher from the repository root in a Wayland
or X11 desktop session with a working graphics driver:

```sh
sh experiments/gio/preview-linux.sh
```

The launcher builds the preview and seeds a fresh directory under
`TMPDIR` (default `/tmp`). It checks the development identity and the
`Analytics QA` household before starting the preview with the isolated
database and settings paths. The printed run directory contains the seed log.

### Optional X11 troubleshooting

If the graphics backend cannot load `libGLESv2.so.2`, check the GLES runtime
package (`libgles2` on Debian). If Mesa reports
`Failed to attach to x11 shm` and the window is blank, try a nested X11 display
with software rendering. Install Xephyr and xauth (`xserver-xephyr` and
`xauth` on Debian), choose an unused display number, and start it from a
terminal on the host display. The example uses `:23`. [Xephyr manual](https://manpages.debian.org/trixie/xserver-xephyr/Xephyr.1.en.html)

```sh
nested_dir=$(mktemp -d)
nested_auth="$nested_dir/Xauthority"
touch "$nested_auth"
xauth -f "$nested_auth" add :23 . "$(mcookie)"
Xephyr :23 -screen 1280x900 -nolisten tcp -auth "$nested_auth" -extension MIT-SHM &
nested_pid=$!
```

Once the Xephyr window opens, run the preview in the same terminal. The
display and rendering overrides apply to this launch. [Mesa environment variables](https://docs.mesa3d.org/envvars.html)

```sh
env -u WAYLAND_SOCKET WAYLAND_DISPLAY="$nested_dir/no-wayland" \
  DISPLAY=:23 XAUTHORITY="$nested_auth" LIBGL_ALWAYS_SOFTWARE=1 \
  sh experiments/gio/preview-linux.sh
kill "$nested_pid"
```
