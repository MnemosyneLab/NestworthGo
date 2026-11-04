# Built-in assistant page guide acceptance — 2026-11-04

Initial acceptance source: `d0b49f697c594031c4fc7b3b71e3c7a9141d1e42`, based on main
`d47055514975345b37c558514fd1e2a58ff93c32` (merged #203, including #200).
[PR #204](https://github.com/MnemosyneLab/NestworthGo/pull/204) remains a draft.
Later evidence-only commits must retain the source fingerprints in
[source-evidence.json](source-evidence.json).

A later currency-completeness correction changes the account guide mapper;
these initial fingerprints/screenshots remain historical acceptance evidence.
The guide now takes the amount, completeness and missing reasons from the same
native `displayValue` projection. It must not borrow household/base FX failures.
Three mobile component regressions cover complete native/missing household FX,
partial native value and missing native value/complete household valuation. The
first and third failed before the correction; all pass afterward. A real
authenticated HTTP regression verifies that a USD 123.4567 account without
USD/CNY FX has complete native evidence and incomplete household evidence.
The shared mapper also preserves exact/zero/signed values and the native currency
when its value is omitted. On follow-up source
`851e01a0f526c5ddd0124568cea09bdc2976a46e`, all **55 affected frontend tests**
passed, as did lint, typecheck, production build, the real HTTP regression and
mobile-package vet. A fresh authenticated real-service mobile run confirms
native USD 100 is available with no native missing inputs while the household
CNY projection has missing FX. It dispatched no write request and its fixture
exited successfully after explicit cleanup. Current-head CI is reported in PR #204.

[Follow-up results](native-fx-results.json) ·
[Source fingerprints](native-fx-source-evidence.json) ·
[Native USD guide](native-fx-guide-390.png) ·
[Native USD evidence](native-fx-evidence-390.png).
Run [native-fx-browser.py](native-fx-browser.py) with `--root` pointing at a fresh
`TestMobileBrowserFixture` directory and `--output` for evidence. It closes the
fixture explicitly and uses the already documented unsandboxed browser mode.

This is an installed **local deterministic page guide**, not a complete AI
assistant. General model conversation, credential entry, durable conversations,
model navigation and AI financial preview/confirmation remain unavailable. The
complete read/write target and dependency order are maintained in
[the architecture contract](../../architecture/builtin-ai-page-guide.md).

## Evidence and isolation

Linux amd64, Go 1.27.3, Node 24.22.0, pnpm 12.14.0, Wails 3.1.0,
Chromium 154.0.8037.57 and Python Playwright 1.62.0. The production frontend was
embedded in the real Wails HTTP-server build. Its build metadata reports the
source SHA above and `vcs.modified=false`; its SHA-256 is recorded with the source
fingerprints. No native GUI was launched.

The desktop used a fresh `analytics-linux-qa-v3 / complete-cny` fixture with
the maintained 2026-07-26 anchor. Database/settings/XDG/tmp paths were explicitly
isolated beneath `/tmp/nestworth-ai-qa`; the synthetic marker was verified before
launch. The authenticated mobile fixture uses a separate temporary synthetic
household and short-lived test credentials. No real ledger, real API key, paid
request, provider credentials or backup was used.

Chromium OS sandbox startup was explicitly attempted and failed in this cloud
container. The recorded browser run uses the explicit `--allow-no-sandbox`
fallback. The failure and mode are retained in the results; this is not sandboxed
browser or native macOS GUI evidence.

## Checks

- **Complete native macOS automated CI passed on the source SHA:**
  [check 41791990587](https://github.com/MnemosyneLab/NestworthGo/actions/runs/41791990587).
  **167 files / 1738 tests passed**, with no failed or pending frontend tests.
  Repository Go tests, vet/native build, tooling, generated bindings,
  frontend lint/typecheck/build, formatting and whitespace gates passed.
  The final PR head's current checks remain linked from
  [PR checks](https://github.com/MnemosyneLab/NestworthGo/pull/204/checks).
- **Provider acceptance passed:**
  [41789277362](https://github.com/MnemosyneLab/NestworthGo/actions/runs/41789277362)
  at `8ba745c852c2ab76db68dca7aa7ea509a365a008`. Its `internal` tree is
  byte-identical to this source: `8828ccd43a4a9556cba2e9a6cb371dbd35302c8c`;
  provider sources/module files are unchanged. This is the existing fake-provider
  backend acceptance, not an installed UI model run.
- Local final focused frontend run: **7 files / 33 selected tests passed**,
  36 unrelated cases skipped by the explicit filter. It covers page-host bounds,
  exact/missing/subtotal values, stale/duplicate/writer-intent refusal,
  three-language keyboard/evidence/review flows, Stop/disconnect, manual read
  refresh, idle desktop behavior, mobile integration and locale/boundary guards.
  This is not a local complete-suite pass.
- Local final production frontend and HTTP-server builds passed. Closed status
  and mobile live Read/auth/revocation/unsupported-route Go tests passed;
  affected-package vet and whitespace checks passed. Generated files are not
  committed. The existing ECharts large-chunk advisory remains.

An earlier local complete attempt under concurrent compilation/coding observed
1709 passes and 26 failures, primarily timeouts and subsequent missing-element
failures (902.54s). A later five-file serial attempt observed 374 passes and two
old App navigation timeouts (429.64s). They are not clean exact-commit full-suite
evidence and must not replace the complete source-head macOS result. One focused
attempt also timed out while waiting for mobile bootstrap; the clean focused
rerun passed. No assertion or deadline was relaxed. An initial GTK-dependent
binding attempt, a Go embed read overlapping Vite output replacement, and a
root-CWD frontend setup attempt failed; correctly configured `CGO_ENABLED=0`,
stable-bundle and `frontend/`-CWD reruns passed.

## Real-service browser acceptance

[Structured results](browser-results.json) record **14 passing scenarios**:
desktop page/metric evidence, disambiguation, keyboard and dock geometry;
expanded workspace; disconnect without replay; account navigation through
existing Back history; separate closed review and focus return; 640px Sheet
bounds; both Chinese desktop locales; authenticated 390px mobile guide/review
and preserved recording; mobile account navigation; 320px mobile/offline;
both Chinese mobile locales; and unchanged financial facts.

There were zero page-script errors, unexpected console errors or external
browser requests. The expected unauthenticated mobile-session probe's 401 and
existing favicon warning are classified explicitly in `knownWarnings`.
All three mobile guide form controls measured 44 CSS pixels high. Geometry is
checked after the Sheet transition, including title containment. The fixture's
existing detail action and central Quick record are separate legitimate buttons;
the acceptance checks the central action inside the original navigation.

Read-only hashes of seven desktop financial fact/receipt tables are identical
before and after guide, navigation and closed-review interactions. This shows
these interactions produced no financial changes; it does not validate an AI
writer, which is absent. Expired previews, unknown results and concurrent
financial confirmation remain backend-core/future UI acceptance, not simulated
success states in this guide.

- [Desktop guide and evidence](desktop-guide.png)
- [640px conversation](narrow-guide.png)
- [Simplified Chinese desktop](desktop-zh-CN.png) · [Traditional Chinese desktop](desktop-zh-TW.png)
- [390px mobile guide](mobile-guide-390.png) · [Separate review](mobile-review-390.png)
- [320px mobile guide](mobile-guide-320.png)
- [Simplified Chinese mobile](mobile-zh-CN-320.png) · [Traditional Chinese mobile](mobile-zh-TW-320.png)

## Reproduction and performance limits

Follow [real browser acceptance](../../development/real-browser-acceptance.md).
Create a new maintained synthetic fixture and verify its marker, generate
bindings/build the frontend from `frontend/`, then build the server with
`CGO_ENABLED=0 GOFLAGS='-mod=readonly -tags=server -p=1'`. Start it with explicit
isolated database/settings/XDG/tmp paths and port 18861. Start the opt-in
`TestMobileBrowserFixture` using `NESTWORTH_MOBILE_BROWSER_FIXTURE` below the same
QA root and `NESTWORTH_MOBILE_BROWSER_BIND=127.0.0.1:18862`. It expires after five
minutes; start the browser only after `connection.json` exists. Run
[browser-acceptance.py](browser-acceptance.py) with `--root` and `--desktop`;
the sandbox fallback requires its explicit flag. Stop the temporary services
afterward. Never use a personal server/database for this journey.

[Source CI timings](source-ci-timings.json): macOS job 215s; binding generation
3s, frontend build 5s, correctness lanes 148s and frontend Vitest 115.91s.
Persistent-tool/cache setup took about 1s and locked frontend installation 1s.
The provider job took 145s. These unpaired observations are not global latency,
RSS, p95 or performance-regression claims. No new polling, provider work or
financial queries run from the closed assistant. Active context/message bounds
are documented in the architecture contract.

Native macOS GUI/VoiceOver, physical iOS/Android, secure credential storage,
native close/restart/restore acceptance, packaging/signing and live-provider
compatibility remain unverified. Linux HTTP and emulated touch screenshots do
not establish those gates.
