# NW-008: exact default time at the history boundary

Baseline: `b08dfb301b7da2310d0f78cdc23d832822f78bc4`.

Starting history at `12:34:45.123` and opening Record change in the same
minute used to submit `12:34:00`. The server correctly rejected this as earlier
than the starting point. The new default captures an exact browser instant once,
shows its local seconds and milliseconds, and sends RFC3339 `effectiveAt` with
empty local date/time fields. Editing the amount, changing transaction kind,
waiting across a minute, previewing, or confirming does not recapture that time.

Editing either date or time explicitly selects the existing manual `HH:MM`
contract (the start of that minute). If that minute starts before the exact
history boundary, the form explains the problem and blocks preview. “Use current
time” explicitly captures another instant and invalidates any old preview, even
when the displayed minute has not changed. Closing and reopening starts a fresh
capture. The backend still rejects pre-origin and future instants; no origin,
schema, historical Fix timestamp, or backend boundary rule changes.

## Regression evidence

- `HistoryPage.test.tsx`: exact defaults in UTC/Shanghai with nonzero origin
  seconds and milliseconds; immutable time across waiting and Preview/Confirm;
  manual minute rejection versus an exactly aligned origin; later valid manual
  minute; same-minute recapture invalidating an asynchronous preview; fresh
  capture after close/reopen; kind change preserving time; date edit invalidating
  a pending preview and switching precision. Existing Record/Fix stale-preview
  and transfer-correction tests remain in the executed suite.
- `exactTime.test.ts`: displayed seconds/milliseconds in all three UI languages;
  local minute boundaries, including a historical second-offset timezone.
- `TestExactDefaultTimeAndManualMinuteBoundaries`: real temporary SQLite and Wails
  API; UTC/Shanghai × second/millisecond origins × cash addition/position transfer.
  Preview and delayed Confirm preserve the exact timestamp after database reload.
  Both API operations still reject pre-origin and future instants; a legal manual
  minute previews successfully. These assert correct contracts, not a passing
  characterization of the original defect.
- Local targeted frontend suite: 76 tests passed, including locale coverage.
  Wails history tests, targeted race checks, binding generation/typecheck and
  changed-file lint are also run before delivery. The coordinator owns the final
  production build, complete gates and independent real IPC browser verification.

JavaScript capture/display precision is milliseconds. Manual IANA timezone
resolution, including DST gap/ambiguity validation, remains server-authoritative;
this change does not silently adjust invalid times or relax validation for clock
skew. Confirmation uses the reviewed command without fetching a newer time.
