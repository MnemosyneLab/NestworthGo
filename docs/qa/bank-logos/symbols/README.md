# Bank symbol presentation review

2026-10-02, Chromium on Linux. Baseline: `58e1630` (latest main when work
started). Synthetic state renders the real `IconPicker`, `EntityIcon`, and
Base UI `Sheet`; no personal ledger or external provider was used.

- [Desktop before/after, light and dark (1000px)](comparison-1000.png)
- [Narrow before/after, light and dark (390px)](comparison-390.png)

The side-by-side examples retain the same 20px and 32px sample slots to show
artwork improvement independently of slot size. Actual account list/detail
and institution list slots are now 32px; selected picker previews are 36px and
choices 48px. Wordmarks such as Citi remain proportionate when no standalone
symbol is supplied. Both themes use a small white surface for brand contrast.

## Evidence

- Inspected all 212 `-rect` assets and all 184 `other/` assets in contact sheets.
  Paired each with its original catalog variant; reviewed filename exceptions.
- 304 presentation images successfully decoded with nonzero dimensions, with
  no page errors. All original paths and paint match the source; only viewBox
  bounds change. White full-canvas backplates are excluded from measurement,
  with 3.5% padding retained around the complete foreground bounds.
- Keyboard Escape from both a bank search field and a result button closes the
  picker, returns focus and keeps the unsaved Sheet input. A second Escape
  closes the Sheet. Automated account-page tests exercise the actual edit form.
- 61 focused frontend tests passed, including all 608 saved IDs, single-choice
  variant search, fallback retention, selected-state aliasing, unchanged stored
  IDs, inheritance/override/reset and account Sheet regression.
- Python asset/screening/installer suite: 15 tests passed.
- Go domain and application tests passed.

User reference `libfile_55d93bf26c5c819184fd72ee1d1948f7` resolved as
`IMG_6759.png`, but the supported Library transfer failed twice with
`download failed`; its pixels could not be inspected. The work above is based
on actual repository assets and browser output, not a claim of matching that
unavailable image. Library saving of the comparison images also failed before
upload with an HTTP 401 from the hosted-app helper; no Library IDs were created.
Native macOS/Wails acceptance and a full real-data application session remain
unverified.

## Reproduce the component review

Install Playwright outside the application (no runtime dependency is added),
then run Vite on port 9245. From the repository root, temporarily copy
`review.html` to `frontend/bank-review.html` and `review.tsx` to
`frontend/src/bank-review.tsx`. Create `/tmp/nestworth-bank-review` and run:

```sh
NODE_PATH=/path/to/playwright/node_modules node docs/qa/bank-logos/symbols/capture.cjs after
NODE_PATH=/path/to/playwright/node_modules node docs/qa/bank-logos/symbols/verify.cjs
```

The capture script renders both viewport sizes and themes. Capture `before`
from a checkout of `58e1630` with the same temporary harness, then run
`compare.cjs` to assemble the two comparison images. Remove both temporary
frontend harness files after reviewing; they are not production routes.

## Real directory-edit regression follow-up

The earlier Sheet-only component check did **not** cover institution editing:
`DirectoryEntityList` edits an institution inline. It had neither a Cancel
control nor an Escape handler after its picker closed. This explains the user's
second-Escape failure; it was not a Base UI portal/focus bug.

The actual `DirectoryPage` was reproduced in Chromium in zh-CN with seeded
TanStack Query data. Real keyboard events left the inline editor open before
this fix and dismissed it after it. The row now owns second-Escape cancellation,
adds Cancel, discards only unsaved edits, and restores focus to its Edit button.
The first Escape remains owned by the picker and preserves the draft.

- [Actual directory before: second Escape leaves editor open](real-directory-escape-before.png)
- [Actual directory after: second Escape cancels edit](real-directory-escape-after.png)
- [First Escape after selection retains institution draft](directory-first-escape-selected.png)
- [Actual AccountForm in Sheet retains draft on first Escape](account-first-escape-selected.png)

`flow-review.tsx` renders the real DirectoryPage and AccountForm, with only data
queries seeded; it does not replace their UI or event handlers. Temporarily copy
it and `flow-review.html` into the corresponding frontend locations, then run
`real-flow.cjs` with the same Playwright setup above. It uses browser keyboard
input from search, result focus, and after selection, plus Cancel/reopen for both
flows. No manually dispatched DOM events are used. Account edit is a Sheet;
institution edit is inline. Native Wails remains an outstanding acceptance step.
