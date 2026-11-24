# Gio desktop preview

The Gio preview opens an isolated synthetic Nestworth household. **Settings → Appearance** edits general display preferences, **Activity → Ledger activity** reads the ledger, and **Directory → Institutions** lists, searches and renames institutions. Overview shows three sample balances. **Manage → Component Preview** opens a working form for trying the shared controls. The [desktop migration roadmap](docs/roadmap.md) tracks the remaining Wails workflows.

## General settings and sidebar

Open **Settings → Appearance** to edit appearance, color theme, language,
timezone, date and time formats, and decimal and grouping separators. The
form shows the household base currency and the resolved display timezone.
If the timezone differs from the one used to start ledger history, the form
shows both. Ledger activity keeps its recorded civil dates.

**Save changes** applies the draft. **Discard changes** returns to saved
values. **Restore display defaults** asks for confirmation and resets the
eight editable fields. Reopen the same verified run directory to see saved
preferences. A new fixture uses the shared defaults, including system
appearance, system language and the Indigo color theme.

The sidebar footer applies its choices immediately. Its language menu offers
English, Simplified Chinese, Traditional Chinese and Japanese. Its appearance
button cycles through system, light and dark. Both are available when the
sidebar is collapsed. Use Settings to return the language to **System** or
to choose an Indigo, Lavender, Mint, Sky or Peach accent.

## Module layout

| Location | Responsibility |
| --- | --- |
| [main.go](main.go) and the two preview launchers | Build and launch the desktop executable, load bundle assets, and manage the isolated database lifetime. |
| [internal/desktop](internal/desktop) | Gio window, shell, page layouts, asynchronous preference updates and frame-level interaction tests. |
| [internal/ui](internal/ui) | Shared editors, selects, date and time pickers, buttons, overlays, tooltips and visual tokens. |
| [internal/preferences](internal/preferences) | Reads and writes General preferences through the shared settings store and resolves system appearance, language and timezone. |
| [internal/history](internal/history) | Activity list reads, pagination and retry state. This model can be tested without a Gio window. |
| [internal/i18n](internal/i18n) | Typed message keys, language selection, translations and catalog checks. |
| [internal/pilot](internal/pilot) | Synthetic household preparation, file and marker verification, and safe database opening. |
| [docs](docs) | [Migration stages and acceptance](docs/roadmap.md) and [interaction and control design](docs/foundation.md). |

Keep page-specific query state beside its feature model and Gio widget state
in the desktop view. Shared financial rules and storage remain in the parent
module. Run `go test -mod=readonly ./...` from this module for changes here.

### Adding UI text

Add a typed message key in `internal/i18n` and a value in each English,
Simplified Chinese, Traditional Chinese and Japanese catalog. Use the
[Wails translations](../../frontend/src/i18n/locales) for existing workflows.
Resolve copy through the active language for visible text, tooltips, errors and
accessible labels. Display user-entered names, notes and canonical data values
directly. Run the same translation checks as CI from the repository root:

```sh
go -C experiments/gio test -mod=readonly -count=1 ./internal/i18n
```

## Component preview

To check live switching, enter text in the component preview and select part of
the notes. Change language or appearance from the sidebar. The form keeps the
draft and selection. Change the color theme in Settings, save, and return to
the preview to see the same draft in the new palette.

Type in the search field to filter the display-only results. Tab to its
clear button or click it to reset the query and focus the editor again.
The institution name allows a draft over 48 UTF-8 bytes and shows an error
until the draft fits. **Submit sample** advances the result counter only
when the name is valid.

To test a native input method, focus **Notes**, type a word using Pinyin, and
confirm a candidate. Check that the chosen text appears once and the submission
counter stays unchanged. Press Return after composition ends to insert a line
break, then select and edit text across the two lines.

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

## Persistent test data

The application ID is `com.nestworth.gio`. On first launch, Nestworth Gio
creates the `Gio Institutions QA` household. Subsequent launches, including
after rebuilding the app, reopen that household and its saved preferences.

| Platform | Default profile directory |
| --- | --- |
| macOS | `~/Library/Application Support/com.nestworth.gio/` |
| Linux | `$XDG_CONFIG_HOME/com.nestworth.gio/`, or `~/.config/com.nestworth.gio/` |

The profile contains `data/nestworth.db` and `data/settings.json`. Gio
verifies the household marker and settings before opening the database for
writes. If an existing profile fails verification, startup reports the error.

To run a disposable scenario or reopen an older verified Gio fixture, set
`NESTWORTH_GIO_RUN_DIR` to its private, absolute directory when invoking
the launcher. If you also specify `NESTWORTH_DATABASE_PATH` and
`NESTWORTH_SETTINGS_PATH`, they must point to those two files under that root.

## Ledger activity

Open **Activity → Ledger activity** (简体中文 **活动 → 账务记录**) to browse
recorded activities in service order. Each row shows the activity label,
recorded local date and optional note. The first request reads up to 50 rows.
If the service returns a continuation, load the next page to see older rows.
Reload starts again at the newest entry. A failed read offers a retry and
keeps previously displayed rows available.

Newly created profiles include a small set of posted sample activities.
Opening an older, verified fixture that contains no activities shows the
empty state. The route only reads the ledger.

## macOS

On Apple Silicon with Python 3 and the repository's Go and Xcode toolchain,
run this command from the repository root:

```sh
sh experiments/gio/preview-macos.sh
```

The launcher builds and opens `bin/gio/Nestworth Gio.app`. To return to the
same profile later, double-click that bundle in Finder or run
`open "bin/gio/Nestworth Gio.app"` from the repository root. Use
`sh experiments/gio/preview-macos.sh --build-only` to build without launching.

## Linux

Use Go 1.27.4 or newer, Python 3, a C compiler, and pkg-config. The default build includes
X11 and Wayland support and needs their development libraries as well as EGL
and Vulkan headers. [Gio Linux requirements](https://gioui.org/doc/install/linux)

For example, on Debian 13 with Mesa:

```sh
sudo apt install gcc pkg-config python3 libwayland-dev libx11-dev \
  libx11-xcb-dev libxkbcommon-x11-dev libxcursor-dev libxfixes-dev \
  libegl-dev libvulkan-dev libgles2 libegl-mesa0 libgl1-mesa-dri
```

From the repository root, build and run the Gio tests without a desktop session:

```sh
sh experiments/gio/preview-linux.sh --build-only
CGO_ENABLED=1 go -C experiments/gio test -mod=readonly ./...
```

To open the preview, run the launcher from the repository root in a Wayland
or X11 desktop session with a working graphics driver:

```sh
sh experiments/gio/preview-linux.sh
```

The launcher prints the installed executable path under `bin/gio/`.
Running that executable directly reopens the saved profile.

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
