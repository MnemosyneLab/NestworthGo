# Gio desktop preview

The Gio preview opens an isolated synthetic Nestworth household. **Overview** reads the current valuation. **Accounts** browses accounts by institution, creates total-balance, manual-valuation and empty Holdings accounts, and supports account settings, archive and restore. **Settings → Appearance** edits general display preferences. **Activity → Ledger activity** previews and records cash contributions, withdrawals and transfers between cash accounts, and opens their saved details. **Directory** manages members, institutions and groups. **Manage → Component Preview** opens a working form for trying the shared controls. The [desktop migration roadmap](docs/roadmap.md) tracks the remaining Wails workflows.

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

## Directory

The **Members**, **Institutions** and **Groups** tabs each keep their own name
search and archive filter. **Members** opens by default. **Show archived**
includes active and archived entries. Confirm **Archive** to archive an
active entry, or choose **Restore** from an archived row.

Members and groups can be added and edited with a name and an icon. The picker
uses the same selectable icon keys and translated names as Wails. The service
rejects archiving a member who is a live account's only active owner or the
household's last active member. Archiving a group preserves existing account
references to that group.

**Add institution** takes a name and type. The type sets the default icon, and
**Icon** searches generic symbols and bank logos by name or alias. **Edit**
can save the name and icon together. The list and object view display the
saved icon, including historical wordmarks.

**View** opens a read-only destination for a member, institution or group.
**Back** and **Forward** in the page bar traverse the current window's visits.
Back from an object returns to its list with the tab, filters, scroll and
available source focus restored. **Recent** shows up to 20 distinct departures
from this session. Reopening one adds a visit and can replace the forward
branch when you have gone Back. Object views reread the stored ID on entry
and can be reloaded. The trail and Recent start fresh after a restart.

Directory rereads the list after each write. If the write succeeds and the
list fails to reload, **Retry reload** reads the list again without repeating
the write. The next launch opens the same synthetic ledger with active entries
and an empty search.

## Persistent test data

The application ID is `com.nestworth.gio`. On first launch, Nestworth Gio
creates the `Gio Institutions QA` household. Subsequent launches, including
after rebuilding the app, reopen that household and its saved preferences.

| Platform | Default profile directory |
| --- | --- |
| macOS | `~/Library/Application Support/com.nestworth.gio/` |
| Linux | `$XDG_CONFIG_HOME/com.nestworth.gio/`, or `~/.config/com.nestworth.gio/` |

The profile contains `data/nestworth.db` and `data/settings.json`. Gio takes
the same per-database instance lock as Wails, verifies the fixture identity
and settings, and then opens the database for writes. The native startup
window reports an occupied ledger, a pending restore, or an invalid profile.
Close the owner before opening another session. Gio leaves unrecognized
pending restores in place. The empty `data/nestworth.db.instance-lock` file
can remain after quit. Its operating-system lock is released when the session ends.

To run a disposable scenario or reopen an older verified Gio fixture, set
`NESTWORTH_GIO_RUN_DIR` to its private, absolute directory when invoking
the launcher. If you also specify `NESTWORTH_DATABASE_PATH` and
`NESTWORTH_SETTINGS_PATH`, they must point to those two files under that root.

### Empty onboarding fixture

On Linux, build the preview and use its executable to prepare and verify a
new private run. The command refuses an existing `data` directory and the
default Gio profile.

```sh
sh experiments/gio/preview-linux.sh --build-only
gio_run=$(mktemp -d)
export NESTWORTH_GIO_RUN_DIR="$(cd "$gio_run" && pwd -P)"
unset NESTWORTH_DATABASE_PATH NESTWORTH_SETTINGS_PATH
gio_preview="bin/gio/linux-$(go env GOARCH)/Nestworth Gio/bin/Nestworth Gio"
"$gio_preview" --prepare-onboarding-fixture
"$gio_preview" --verify-pilot-fixture
"$gio_preview"
```

The macOS development bundle provides the same commands through
`Nestworth Gio.app/Contents/MacOS/Nestworth Gio`. Keep the profile at its
original path and launch the executable with the same environment to reopen
it. The marker is paired with `data/onboarding.fixture` and the canonical
run path. Prepare another fresh fixture to repeat onboarding.

Enter the household name and at least one initial member. The base currency
starts at CNY. **Start ledger history** is optional. When enabled, the date
starts at today in the displayed History timezone and can be moved earlier.
Creation opens an empty Accounts page. Reopening the same profile loads the
saved household. Ordinary preview launches continue to seed the standard
sample household.

### Disposable startup recovery fixture

Use a fresh private directory. Preparation creates a shared-service backup,
installs its replacement inside the disposable profile, and leaves it at the
`replacement-installed` journal phase. The first direct window launch checks
the pending plan, finalizes the restore and opens the fixture. It shows the
USD 42.25 Recovery Sample Reserve, Mint on Dark appearance and day-first dates.

```sh
sh experiments/gio/preview-linux.sh --build-only
gio_run=$(mktemp -d)
export NESTWORTH_GIO_RUN_DIR="$(cd "$gio_run" && pwd -P)"
unset NESTWORTH_DATABASE_PATH NESTWORTH_SETTINGS_PATH
gio_preview="bin/gio/linux-$(go env GOARCH)/Nestworth Gio/bin/Nestworth Gio"
"$gio_preview" --prepare-recovery-fixture
"$gio_preview"
"$gio_preview" --verify-pilot-fixture
```

Run the last command after quitting the window. Verification alone never
reconciles a pending restore, so use the bundled executable for the first
launch. The fixture is bound to its original canonical root. Prepare another
directory to repeat the scenario. Arbitrary restore journals, backups and
crash layouts remain blocked in the preview.

## Overview

Overview reads net worth, assets and liabilities from the shared Go service.
It shows the household base currency, local read time and available quote
dates. When values are missing, the page identifies affected accounts or
instruments and labels available figures as known contributions or subtotals.
An empty household has its own state. Reload rereads the ledger and stored
quotes. A failed reload keeps the previous figures and their original read
time visible with a retry action.

## Accounts

Accounts groups rows by institution. From a Directory institution's detail,
**View accounts** opens that institution's list. Select an account to read its
owners, institution, group and available values. Account currency and household
currency are shown separately when they differ. Known subtotals, missing
prices or exchange rates, and stale cached quotes carry their own labels.

**Show archived** includes archived records. If a simple archived account has
a recorded value in its own currency, that value remains visible. **Back to
accounts** restores the preceding account list, including its institution
scope, archive filter, scroll and available row focus. **Reload** refreshes
the current list or account.

In the account list of an active institution, choose **Add bank account**.
Enter a name, currency and opening balance, choose active owners, and review
the amount, equal ownership shares and net-worth inclusion before confirming.
The standard bank account tracks its total balance in one currency. After
creation, the new account opens by ID. **Back to accounts** returns to the
institution's list, and the next visit to **Overview** refreshes the valuation.
If the new account's details fail to load, **Retry** only reads its details.

Open a standard bank account's **Account settings** to change its name,
institution, group, icon, net-worth inclusion, or ownership. Choose equal
shares or enter positive shares totaling 100%. **Archive account** keeps
the recorded balance and ledger history. Turn on **Show archived** to find
the archived account, then use **Restore account** in its detail. A failed
refresh after a confirmed change can be retried without repeating the change.

From **Accounts** with all institutions shown, choose **Add liability account** to
create a credit card, loan, or other liability. **Add asset account** offers
cash on hand, a bank account, a digital wallet, a receivable, or another
asset. These forms record one total in the selected account currency. Choose
an existing institution or leave the account unassigned. Enter opening debt
as a positive amount. Detail shows the liability sign and any unavailable
household-currency conversion.

**Account settings** can change the type to another total-balance type with
the same asset or liability role. The account ID, currency and recorded
balance stay fixed. The institution-scoped **Add bank account** entry keeps
its existing quick form.

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
