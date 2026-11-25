# Gio desktop preview

The Gio preview opens its own Nestworth ledger. An empty profile opens household setup. **Overview** reads the current valuation. **Accounts** browses accounts by institution, creates total-balance, manual-valuation and empty Holdings accounts, and supports account settings, archive and restore. Bank account details can reconcile a native balance, and Holdings account details can reconcile cash by currency. **Settings → Appearance** edits general display preferences. **Activity → Ledger activity** can start history for an existing household without positive positions. It filters saved records and previews and records contributions, withdrawals, cash transfers and currency conversion within a Holdings account. Eligible cash movements can be corrected or reversed from their details. **Directory** manages members, institutions and groups. **Manage → Component Preview** opens a working form for trying the shared controls. The [desktop migration roadmap](docs/roadmap.md) tracks the remaining Wails workflows.

## Checking an interrupted transfer

After reopening the same profile, a receipt notice can link to
the recorded transfer or in-account conversion. If the outcome is
unconfirmed or the current ledger has no receipt, check **Ledger activity**
before recording again. **Dismiss receipt** closes the notice. The preview
only queries the ledger during this restart check.
[Profile and receipt behavior](docs/foundation.md#transfer-and-conversion-receipts-after-restart)

## Settings and sidebar

Open **Settings → Appearance** to edit appearance, color theme, language,
timezone, date and time formats, and decimal and grouping separators. The
form shows the household base currency and the resolved display timezone.
If the timezone differs from the one used to start ledger history, the form
shows both. Ledger activity keeps its recorded civil dates.

**Market data** sets the FX refresh provider and quote cache duration. Save,
replace, or remove a CoinGecko key in its own card. The stored key stays in the
local database. **Diagnostics** sets the application file log level and shows
the log path in the active Gio profile. **About** shows the version, build,
license, and preview app identity.

**Save changes** applies the shared draft across tabs. **Discard changes**
returns to saved preferences. **Restore all preference defaults** asks for
confirmation and preserves the ledger and provider key. Reopen the same
verified run directory to see saved preferences. A new fixture uses system
appearance, system language, the Indigo color theme, a 12-hour quote cache,
and file logging off.

The preview remembers the last window size in the active profile. Resize the
window and reopen that profile to restore the same dimensions.

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
| [internal/ui/lucide](internal/ui/lucide) | Lucide icons, the set the Wails frontend uses, drawn as round-capped strokes from generated path tables. |
| [internal/snapshot](internal/snapshot) | Renders recorded Gio operations to PNG through the headless GPU backend. Tests and tooling only. |
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

Under **Chart examples**, scroll to **Sankey flow → Dense allocation**.
The synthetic example has 24 account paths across four columns. In the
diagram, hover or select a node or band to trace its upstream and downstream
flow. Use Tab to focus the chart, arrow keys to move, Return to select, and
Escape to clear a selection. The compact list includes each account's asset
class and institution and scrolls the focused row into view. Item details use the supplied display amounts.

The same dense fixture is available in the standalone native gallery:

```sh
go -C experiments/gio run -mod=readonly ./tools/chartgallery \
  -chart sankey -variant 4 -size 1100x900
```

Resize to about 710dp and 390dp to see the medium diagram and the compact
list. **Data rows** exposes all nodes and links, including the smallest flows.

## Visual snapshots

`TestVisualSnapshots` renders the shell and a control-state gallery to PNG
files for review. It needs a GPU backend (Metal on macOS, EGL or Vulkan on
Linux) but no window server or screen-recording permission. It asserts
nothing and is skipped unless an absolute output directory is set:

```sh
NESTWORTH_GIO_SNAPSHOT_DIR=/tmp/gio-shots go -C experiments/gio test \
  -mod=readonly -count=1 -run TestVisualSnapshots ./internal/desktop
```

The output covers Component Preview and Settings in light and dark with each
color theme and language, open select, date and time pickers, a gallery of
buttons, text fields and surfaces with rest, hover, press, focus, error,
disabled and read-only states, the design tokens, shadow levels, type scale,
the confirmation dialog, the Lucide icon sheet and the Overview, Settings and
Component Preview pages at four window sizes. Images
use 2x density. Keep a copy before changing a control and compare the same
file names afterwards. Review them separately from native macOS checks:
headless frames do not exercise the window system, input methods or VoiceOver.

## Visual design system

`internal/ui` carries the Wails design tokens: OKLCH colors converted to sRGB
(`oklch.go`, `theme.go`), radii, control heights, type scale and shadow levels
(`tokens.go`), and the animation primitives (`motion.go`). Shared controls
draw their own states: buttons come in six variants and four sizes with
animated hover, a press scale and a focus ring; text fields, selects and
pickers share one field frame; cards and floating layers share one surface
with shadows. Motion runs only while a tween is moving, so an idle window
requests no frames. `ui.SetReducedMotion` turns every tween into a jump.

Fonts are system fonts only (Avenir Next, with Helvetica Neue and CJK
fallbacks, and Iowan Old Style for display titles). Plus Jakarta Sans is not
bundled. Icons are Lucide. After changing the icon list in
`tools/lucidegen`, regenerate the tables from `experiments/gio`:

```sh
go run ./tools/lucidegen -out internal/ui/lucide/icons_gen.go
```

The generator reads `frontend/node_modules/lucide-react`, so run `npm ci` in
`frontend` first. The directory icon picker still uses Material icons.

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

The application ID is `com.nestworth.gio`.

| Platform | Default data directory |
| --- | --- |
| macOS | `~/Library/Application Support/com.nestworth.gio/data/` |
| Linux | `$XDG_CONFIG_HOME/com.nestworth.gio/data/`, or `~/.config/com.nestworth.gio/data/` |

Gio uses the Wails SQLite format, instance lock, schema migrations, startup
restore reconciliation and settings store. An empty profile opens household
setup. An existing ledger opens and saves to the Gio profile. The
`nestworth.db.instance-lock` file can remain after quit. Its operating-system
lock is released with the session.

### Use a local Wails copy on macOS

1. Quit Wails and Gio completely. Move any existing Gio `data` directory
   aside as a backup. Use independent files in the standard Gio directory.
2. From the repository root, create the SQLite copy and bring over the
   presentation settings. Use the actual Wails source directory if you
   configured a custom location.

   ```sh
   (
     set -e
     wails_data="$HOME/Library/Application Support/Nestworth"
     gio_data="$HOME/Library/Application Support/com.nestworth.gio/data"
     install -d -m 700 "$gio_data"
     test -z "$(ls -A "$gio_data")"
     /usr/bin/sqlite3 -readonly "$wails_data/nestworth.db" ".backup \"$gio_data/nestworth.db\""
     chmod 600 "$gio_data/nestworth.db"
     if test -f "$wails_data/settings.json"; then
       install -m 600 "$wails_data/settings.json" "$gio_data/settings.json"
     fi
   )
   ```

   SQLite's `.backup` includes committed changes in the source WAL. Gio creates
   its own SQLite sidecars and lock. Copying just the main database while
   a nonempty WAL remains can lose transactions. Keep the copy local and
   private because a Wails database may contain stored provider keys. The copy
   consists of the ledger and presentation settings. Keychain and external
   service settings stay with the original installation.
3. Build with `sh experiments/gio/preview-macos.sh --build-only` and
   double-click `bin/gio/Nestworth Gio.app`, or run
   `open "bin/gio/Nestworth Gio.app"`. Subsequent normal launches reopen the
   copy.

### Run an independent onboarding test

Use the same database and settings overrides as Wails to give a synthetic
test a fresh directory. Omit these overrides when opening the normal Gio
profile.

```sh
gio_run=$(mktemp -d)
NESTWORTH_DATABASE_PATH="$gio_run/nestworth.db" \
NESTWORTH_SETTINGS_PATH="$gio_run/settings.json" \
sh experiments/gio/preview-linux.sh
```

Use `preview-macos.sh` in the same command on macOS. An empty ledger opens
household setup. Enter a household name and an initial member, and decide
whether to start History. Creation opens Accounts. Reopening the same
database loads the saved household.

## Wealth History

Open **Insights → Wealth history** to view Allocation for the previous month
end, or the first closed History date when History started later. Switch
between asset classes and currencies. Select a chart category or its text
row to filter Accounts. **Return to allocation** restores the chart or row
focus and scroll position. **Compare dates** and **Compare with current**
offer paired category values for assets and liabilities.
**Category comparison** charts adjacent A and B amounts on one scale.
**Change** ranks comparable account balance changes and keeps other accounts
available for inspection. Open valuation details for both endpoints and their
components, or follow **View in Ledger activity** with the captured scope.
**Reconciliation evidence** shows the shared supported contributions, residual
and component records.

Choose **Trend** to read daily net worth, assets and liabilities in the saved
household base currency. Trend starts with the last closed day in the History
timezone. Select **From date** and **Through date**, then choose **Load range**
for up to 31 closed dates. The table shows
available totals, known subtotals, missing inputs and quote coverage. Tab past
the range and metric controls to reach the chart, then the table. Change the
chart between net worth, assets and liabilities. Use the arrow keys and Enter
on a chart date to set a range endpoint or open that day's Accounts. In the
table, use arrows, Page Up, Page Down, Home and End to move through rows.

**Reload** uses the existing ledger and stored quotes. After a read failure,
**Retry** repeats the selected range and the previous capture remains visible
until a new read succeeds.
[Wealth History scope and verification](docs/roadmap.md#read-only-wealth-history)

## Overview

Overview reads net worth, assets and liabilities from the shared Go service.
It shows the household base currency, local read time and available quote
dates. When values are missing, the page identifies affected accounts or
instruments and labels available figures as known contributions or subtotals.
An empty household has its own state. Reload rereads the ledger and stored
quotes. A failed reload keeps the previous figures and their original read
time visible with a retry action.

**Net worth history** shows the shared daily series. Choose a period or
expand **Daily valuation data** to read the values by date. Coverage links
open Data health for the period or day. **Open Wealth history** goes to
the analysis page. Before History starts, the card links to the existing
starting-point workflow in Ledger activity.

In **Data health**, inspect a snapshot finding to see its dates and blocking
inputs. A ready finding offers **Preview snapshot repair**. Confirm to rebuild
the displayed closed dates from saved data. **Stop after current batch** keeps
completed work, and Retry starts at the batch that failed.

The asset and liability category cards use the household's base currency.
**View allocation breakdowns** jumps to institution, account type, member
and group. Open a row to see the matching accounts. **Back to accounts**
returns from an account to its filtered list. The toolbar Back restores
the Overview row and its focus.

**Asset composition Sankey** shows each positive allocation by asset class,
institution and account. The section shows the valuation read date,
quote and FX reference dates, and oldest carried reference when available.
It also shows the known subtotal when coverage is incomplete and counts of
missing and zero-value components. Use arrows and Enter in the diagram to open an
account. **View allocation details** jumps to the keyboard-scrollable list of
all paths, including the smallest ones. Back from an account restores the
diagram or list control that opened it.
[Overview allocation scope and verification](docs/roadmap.md#overview-asset-composition-sankey)

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
Opening a ledger that contains no activities shows the
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
