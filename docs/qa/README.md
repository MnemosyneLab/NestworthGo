# QA Archive

This index points to selected verification evidence. Reports apply only to the
source revisions and scopes they name; they are not a live status board. Use
the [current release record](../releases/v0.3.8.md) for published scope and
open acceptance boundaries.

## Product validation

- [Review of main after 93e10c8 — 2026-10-21](review-93e10c8-to-fcfd7d9.md) — Read-only review through `fcfd7d9`. Three confirmed defects: a funding-gap stamp on sibling holdings, a doubled plus on sub-unit JPY/KRW, and three raw Investment review dates.
- [v0.3.8 Mac package verification — 2026-10-14](v0.3.8-release-package-2026-10-14.md) — Asset/package integrity checks; native acceptance is explicitly owner-deferred.
- [PR101/PR102 post-merge browser acceptance — 2026-10-17–18](post-merge-pr101-pr102-2026-10-17/REPORT.md) — Real Wails server and cloud Chromium against isolated synthetic households; partial pass with a reproducible investment-review replay defect. See the maintained [real-browser workflow](../development/real-browser-acceptance.md).

Older analytics run logs and screenshots were removed after current design,
tests, and later post-merge verification became canonical. They were tied to
obsolete source revisions and did not establish current release or native
acceptance.
