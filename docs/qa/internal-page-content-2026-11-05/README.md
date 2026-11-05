# Internal semantic page reads

The implementation contract, current coverage and limits are maintained in
[built-in AI runtime](../../architecture/builtin-ai-runtime.md#on-demand-internal-page-content).

`browser.mjs` mounts the real desktop page host, new continuous sidebar and shared
DatePickerControl, NativeSelect and FilterableSelect in a **synthetic analysis-like
fixture**. It does not mount every ReturnAnalysisPage query projection, start a
native application, open a ledger, retrieve credentials, call OpenAI, or perform
financial writes. The API adapter simulates only local request-ID capture IPC.
Runtime/provider round trips and module/pagination behavior are covered separately
by Go tests with an isolated synthetic ledger and fake OpenAI responses.

Run with Node 24.22.0, an installed Playwright module and Chromium:

```sh
PLAYWRIGHT_MODULE=/path/to/playwright-core/index.mjs \
PAGE_CONTENT_OUTPUT=/tmp/nestworth-page-content-browser \
node docs/qa/internal-page-content-2026-11-05/browser.mjs
```

[Browser results](results.json) cover English at 1440px, Simplified Chinese at
390px and Traditional Chinese at 390px. They verify:

- Tool-requested capture with the sidebar initially closed and after reopening.
- Capturing the underlying active page while the narrow assistant sheet is open.
- Real shared date-picker label/value and live native select/checkbox state.
- Updated date/scope/checkbox values on a subsequent capture, with a new generation.
- Passwords, textarea notes, private subtrees, option database IDs and raw HTML
  absent from captured data; zero model starts and uncaught browser errors.

Unit/integration coverage additionally checks reviewed field allowlists,
redaction, text splitting, size limits, named modules, cursor/snapshot consistency,
asynchronous results, page-return/remount identity, canceled runs, desktop epoch,
known credential taint, the unchanged external MCP discovery/dispatch boundary,
and analysis navigation through the existing independent one-turn grant.

This is Linux browser/jsdom and synthetic Go verification. Native macOS GUI,
VoiceOver, packaging, real-provider acceptance, date/filter navigation, arbitrary
control execution, other page roots, chart geometry and portal/hover content
remain outside this change. The page tool reports unsupported/partial coverage
explicitly. No real household data was used.
