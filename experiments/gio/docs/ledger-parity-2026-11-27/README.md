# Ledger parity: synthetic Linux checks, 2026-11-27

These captures use independent copies of the same synthetic 18-activity
ledger. No personal ledger data was used. Gio ran as a native X11 window.
The Wails reference was the compiled frontend in headless Chromium connected
to the real Go server; it was **not** a native Wails window. The before Gio
and Wails captures used `ef09d4cc9c2ed226674f2ffd3d6b57ae45188c14`.
The after applications used the implementation in this change, including main
`bdadfcb72a24b686f34a593bf1a1f98f80b04109`. Settings were Simplified Chinese,
Asia/Singapore ledger history, and two display decimal places. The original
paired audit PDF was read separately; it is not reproduced here.

| View | Before | After Gio | Wails reference |
| --- | --- | --- | --- |
| Ledger, 1280 × 800 | [Gio](gio-before-list.png), [Wails](wails-before-home.png) | [Gio](gio-final-list.png) | [Wails](wails-final-home.png) |
| Fee actions | [Gio](gio-before-fee-actions.png) | [Gio](gio-final-fee-actions.png) | [Wails](wails-final-fee-actions.png) |
| Fee details | [Gio](gio-before-fee-detail.png) | [Gio](gio-final-fee-detail.png) | [Wails](wails-final-fee-detail.png) |
| 15 → 16 AUD correction review | Gio could not start this operation | [Gio](gio-final-fee-correction-preview.png) | [Wails](wails-final-fee-correction-preview.png) |
| Saved correction | — | [Gio](gio-final-fee-corrected.png) | [Wails](wails-final-fee-corrected.png) |
| Separate fee reversal review | — | [Gio preview](gio-reverse-fee-reversal-preview.png) | [Wails confirmation](wails-reverse-fee-reversal-step.png) |
| Ledger, 900 × 600 after correction | — | [Gio](gio-final-narrow.png) | [Wails](wails-final-narrow.png) |

Both correction previews showed AUD 17,500 before the replacement and AUD
17,484 after it. Confirming in independent copies produced 20 activities and
an AUD 17,484 balance in each. SQLite readback found the original 15 AUD fee,
a 16 AUD replacement with reason `fee`, classification `fee`, direction
`removed`, and a 15 AUD inverse with classification `fee`, direction `added`,
linked to the original and correction group. The replacement and inverse
retained the original effective instant, `2026-11-13T00:00:00.000Z`.

In separate fresh copies, confirming reversal produced 19 activities and AUD
17,500 in each. Each inverse retained fee classification and linked to the
original. Reversal used its own current effective instant; the two independent
runs therefore have different instants. The fixed-clock Gio regression tests
also exercise correction/reversal idempotent retries, audit links, replay,
protected records, the ordinary expense and contribution paths, and rejection
of tax and unsupported fee shapes.

A fresh pair retained 18 unfiltered rows, 11 for US Brokerage, four for AAPL,
and seven for one month. Gio captures: [account](gio-filters-account.png),
[instrument](gio-filters-aapl.png), [month](gio-filters-month.png), and
[Escape returning focus to the row](gio-filters-month-after-escape.png).
The Ledger drawer restored its filtered state and focused link after visits to
an [account](gio-filters-month-return-from-account.png) and an
[instrument](gio-filters-instrument-return.png). The merged Accounts action
[drawer](gio-filters-account-action-drawer.png) still closes with Escape and
[returns focus](gio-filters-account-action-escape.png). From AAPL's Related
transactions action, Ledger opened with AAPL filtered and Back returned focus
to that action in the instrument. Automated shell tests cover the scoped
account and instrument return paths and delayed reload.

The full Gio Go suite passed after the main integration:

```sh
go -C experiments/gio test -p 2 -mod=readonly -count=1 ./...
```

The regression suite uses a fixed millisecond clock to check untouched draft
time through preview, posting, readback, ordering and reopen, as well as the
intentional minute precision after a date/time edit. The real GUI runs above
cover the fee operations and presentation. Native Wails, other operating
systems, Wayland, screen readers, IME and packaging were not tested in this
Linux run. See [parity.md](../parity.md) for intentionally retained differences.
