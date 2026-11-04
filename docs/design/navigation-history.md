# Desktop navigation history

Nestworth's desktop shell has Back, Forward and Recent controls in the existing
page bar, including the narrow desktop icon-rail layout. The separate mobile
application keeps its existing browser/native navigation. No global keyboard
shortcuts are installed, so native host, input, IME and dialog shortcuts retain
their existing behavior.

## Destinations and one-time actions

Every sidebar, account selection and cross-page object link enters the same
navigation boundary. The boundary checks unresolved Sheet confirmations before
changing any history, recent items, page state or session identity. Back and
Forward use an entries/cursor model; opening a destination after Back removes
the forward branch. The existing contextual Back and investment-account
creation return flow use the same cursor.

A `RevisitableDestination` excludes account creation/value-update actions,
activity-sheet opening, receipt verification, reserve/policy editing and
product-creation intents. These may execute once on an explicit original link;
traversing history does not dispatch a financial mutation or replay that intent.
Closed activity/product sheets are not reopened by history traversal. Existing
maintenance request consumption also remains authoritative for closed editors
and for cancelled or superseded asynchronous navigation.

Leaving a page updates its current entry from live application state. This
captures account/instrument identity, supported History and analysis filters,
analysis tabs/cursors, portfolio view, maintenance search, historical date and
context, scroll and the source focus target. In-page changes replace the current
visit's effective state; typing and scrolling do not append navigation steps.
Tasks' default tab is resolved at entry time. Historical defaults resolve once
after the origin loads and are retained by their capture instance.

This is application navigation, not a general URL/router or DOM serialization
facility. Unexposed local control state (for example Settings subsections and
some page-local sorting) retains the existing mounted component behavior rather
than claiming to be a separately recoverable deep link. Authoritative object
reads remain responsible for deleted/unavailable destinations; history never
substitutes a different account or instrument.

## Sessions and bounds

Back/Forward retains at most 40 navigation entries. Recent is an independent MRU
of up to 20 semantic destinations and survives truncation of the forward branch.
It deduplicates equivalent destinations while preserving distinct objects and
supported filters/dates. Choosing Recent opens a new navigation step so Back
returns to the selection source. Historical selections fork their editable page state while initially sharing the
selected immutable capture. Tasks selections reuse their workspace, including
its editable draft. An edited visit replaces its own obsolete MRU reference so
the menu never advertises an earlier date whose capture has been released.

Historical visits own independent mounted page instances; hiding and restoring
one does not re-freeze settled data. Forks share a read-only query handle only
for the exact selected date/comparison/mode/generation. Changing parameters
uses the fork's own query identity. Refreshing a shared capture first copies its
settled data into a private key, preserving existing failed-refresh/error/retry
behavior without overwriting another visit's result. Pending reads are cancelled
only when no visible host still observes them, and normal zero-time garbage
collection releases a key after its last host unmounts. Only captures reachable from the
bounded history or Recent are retained. Ordinary pages use existing shared
mounted hosts. Overview's current reads are released on Settings and historical
views. Inactive historical headers are suppressed as well as their content.

Tasks workspaces own reconciliation drafts by their monotonic request identity.
They survive Forward, branch truncation and ordinary history eviction. Since
Reconciliation has no application-level draft-discard API, the first version
conservatively retains at most 20 Tasks sessions until the ledger closes. It
refuses admission of a 21st new Tasks session with a localized explanation;
history/Recent traversal of an existing session remains available. The Recent
menu also contains a separate **Retained Tasks sessions** group with up to 20
workspace entries, labelled by business context and a stable session ordinal.
This recovery group is independent of semantic MRU deduplication, MRU eviction
and forward truncation: same-account workspaces retain distinct reachable
drafts even when their normal Recent destination is replaced. Every selection
uses the same confirmation gate. This bounds memory and guarantees an entry
point for each retained editable workspace.

## Privacy and lifecycle

Everything is held in React memory only. Navigation targets, labels and history
are not written to SQLite, settings, local/session storage, URLs, logs or
telemetry. Recent displays business names, page/view labels and supported dates
or a filtered indicator; it does not display amounts, notes, raw object IDs or
saved financial results. Deleted object names fall back to localized
unavailable-account/instrument labels.

Quitting/reloading clears history. Initial bootstrap hydration and the first
onboarding identity keep their workspace, preserving the Accounts completion
destination; the completion callback runs before bootstrap invalidation. A later
change between household identities remounts the workspace and releases its history and retained sessions once any
unresolved confirmation allows the boundary to change. The
current supported backup restore requires application restart, which also
clears history; there is no promise of restoring navigation across that boundary.

## Accessibility

The shared Button and Base UI ActionMenu provide disabled button semantics,
localized accessible names, menu keyboard navigation, Escape/outside dismissal
and trigger focus restoration. The MRU popup is width/height constrained and
scrollable. Restore requests are bound to the target entry and page, survive intermediate
external-store commits, and restore scroll in the layout phase before paint.
Connected visible source focus (or its ID/main-content fallback) is restored in
a later frame; pointer, keyboard or explicit focus changes cancel only that
focus step so it cannot steal focus from a new menu or overwrite a user's scroll.
