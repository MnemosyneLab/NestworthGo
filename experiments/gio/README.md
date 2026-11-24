# Gio desktop preview

This standalone Go module contains the Nestworth sidebar and a Gio application shell. Overview shows three sample balances. **Directory → Institutions** opens the institutions pilot. **Development → Input Method Test** contains a multiline input for testing input methods, editing, and selection. **Development → Component Preview** contains an in-memory form with text fields, search, two selects, and date and time pickers.

The sidebar footer cycles through Simplified Chinese, English, and Traditional Chinese (`zh-TW`), and switches between light and dark appearance. The **Color theme** button shows the current theme and cycles through Indigo, Lavender, Mint, Sky, and Peach. The four added themes use the corresponding light and dark Wails [color tokens](../../frontend/src/index.css). Each launch starts in Simplified Chinese with the light Indigo palette.

To check live switching, open the component preview, enter text in both fields, and select part of the notes. Switch language, appearance, and color theme. The component preview stays open and keeps the draft and selection.

Type in the search field to filter the display-only results. Tab to its
clear button or click it to reset the query and focus the editor again.
The institution name allows a draft over 48 UTF-8 bytes and shows an error
until the draft fits. **Submit sample** advances the result counter only
when the name is valid. Return in the notes inserts a line break.

Use **About this form** to open a popover near the lower-right edge.
It opens above the button when space is tight below and follows it when
the window is resized. Escape, Tab, the Close button, and presses outside
the panel dismiss it and return focus to the trigger.

The account-type list skips disabled rows. In the institution picker, type
to filter the long list, then use Alt+Down to move from the editor to the
results. Arrow keys move the highlight and Return commits it. Alt+Up moves
back to the query. Escape, Tab,
or an outside press discards the search and preserves the selection. Tab
continues to the next field, and Shift-Tab returns to the previous field.

The calendar uses a synthetic today of February 28, 2024 and an explicit
range from February 10, 2024 through March 3, 2025. Arrow keys move the
highlight, Page Up and Page Down change month, and Shift with either page key
changes year. Return selects the highlighted day. The footer changes the
first weekday without changing the selected date.

In the time picker, Left and Right switch between hours and minutes. Select
either part to start with `00` in the other part. The footer switches between
12- and 24-hour display. The results panel shows the canonical date and time
and both pickers offer a clear action.

## Institutions pilot

The pilot lists active institutions from the synthetic ledger. Search filters
the list by name. Choose **Edit** to change an institution name, **Save** to
apply it, and **Reload** to read the list again. The pilot uses the same
application service as the Wails directory and rereads the list after each save.

Each launcher creates a private run directory and seeds a synthetic
`Gio Institutions QA` household. Gio requires explicit database and settings
paths in that directory. It checks the paths, settings, and the ledger's
fixture marker before connecting to the writable database. A missing or
invalid fixture stops startup.

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
identity. [Gio Linux requirements](https://gioui.org/doc/install/linux)

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

The launcher builds the preview in a fresh directory under `TMPDIR`
(default `/tmp`) and prints the path. To reopen the same fixture and see a
saved edit, use the printed run directory in a terminal on the same display:

```sh
run_dir=/absolute/path/from-launcher
export NESTWORTH_GIO_RUN_DIR="$run_dir"
export NESTWORTH_DATABASE_PATH="$run_dir/data/nestworth.db"
export NESTWORTH_SETTINGS_PATH="$run_dir/data/settings.json"
"$run_dir/Nestworth.app/Contents/MacOS/Nestworth"
```

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
