# Desktop built-in AI read-only acceptance — 2026-11-04

## Source and environment

Runtime/UI source: `c537bf2d1213b9d5fc96837c58c7b8f160035d1b` on
`feat/builtin-ai-readonly`, incorporating main `e5a805a` (#223).
Draft PR: [#224](https://github.com/MnemosyneLab/NestworthGo/pull/224).
#221 remains independent: this branch does not edit external MCP handlers,
permissions, transport or skill files. Recheck shared read contracts when that
branch lands; the built-in adapter delegates to their registered implementation.

Local environment: Linux amd64, Go 1.27.3, Node 24.22.0, pnpm 12.14.0;
locked frontend dependencies. GTK/WebKit development libraries are absent.
Bindings, desktop tests/build and root Go checks use `CGO_ENABLED=0` with
`-tags purego` where Wails is involved. This is not native macOS or normal
GTK/CGO build evidence. Generated bindings/dist are not committed.
All data and credentials in automated acceptance are synthetic. No live-provider
credential is configured; no paid model request was made.

## Implemented product flow

1. Existing household, desktop Settings → Built-in AI credentials: save/replace/
   delete an OpenAI key through the existing masked new-input control. Stored key
   values never return to the frontend. Main-database plaintext storage and full
   backup inclusion remain the accepted contract; no Keychain/signing requirement.
2. Enter an OpenAI Responses model ID and explicitly enable desktop read-only
   requests. Key saving alone never enables requests. Provider origin is fixed to
   `https://api.openai.com/v1`; third-party providers/custom endpoints are not exposed.
3. Open the existing right-hand assistant; enter and explicitly send a question.
   It uses real provider wiring and the shared controlled in-process capabilities,
   without internal HTTP/MCP transport. The model can request only current overview,
   financial summary, account list/detail and their frozen page/item continuations.
4. Accepted/running feedback and Stop remain visible until terminal state. The
   provider stream is completion-gated; provisional tokens are not displayed.
   Errors are fixed public messages; no raw provider response/error is rendered.
   Network failures, reconnect and UI refresh never replay a model request.
5. Successful model-native context and display turns persist in the private AI
   sidecar. Reopening restores the most recently updated conversation; older turns
   load one per page. New/delete conversation and latest/previous/next controls
   are available. Retention limits remain 20 conversations, 64 turns each and
   30-day pruning on startup/create; oversized context requires a new conversation.
6. A page selector contains only overview/accounts/account + optional account UUID.
   It grants no authority. `get_current_page` resolves fresh backend values and
   returns tool source/read time; exact visible filters/screen values are not sent.
   Unsupported pages have no model page tool. Existing account-target buttons use
   explicit user clicks, private mapping, expiry and unresolved-review guards.
7. Data evidence shows a host-generated source/read-time trace, per-response
   remaining-page flags and conservative reported/not-summarized missingness.
   It is separate from model prose. Original quote/source dates, currencies,
   quality, gaps and frozen capture metadata remain in related tool results.
   The trace does not certify that the model consumed all relevant pages.
8. Configuration/revision fences, active-run conflicts, explicit disable/cancel,
   model-context reset on model change, host epoch/restore fencing and secret-taint
   checks preserve the existing runtime boundaries. Financial writes, projection-
   maintaining analysis, provider refresh and mobile model permissions stay closed.

## Automated and browser evidence

- Root `go test -tags purego -p 2 ./...`: passed during implementation. Application
  package 310.880s; MCP package 128.537s while frontend checks ran concurrently.
  Final runtime/evidence changes were then checked again in all affected packages,
  including assistant, builtinai, airuntime, credentials, aistate, desktop and mobile.
- Final affected Go checks and purego desktop build passed. Provider nested-module
  `./check.sh` passed: tests (including fake Responses/SSE), vet and build.
- Eino acceptance exercises desktop Configure/Start, narrowed shared capabilities,
  real synthetic SQLite account reads via `get_current_page`, complete native
  history, persisted host evidence, restart and duplicate-request non-replay.
  Provider failure/truncation/incomplete/writer/cancellation/credential-echo cases
  remain covered. The fake transport permits only its loopback endpoint.
- `builtinai` integration reconstructs 27 synthetic account rows across frozen
  pages, validates current-account existence and source time, rejects extra tool
  authority/client fields/revoked or cancelled sessions, and rejects oversized or
  malformed tool results. Runtime tests cover disable during a run, model switch,
  selector-bound dedup and a credential canary in the selector.
- Targeted frontend/translation checks: 40 passed before the final full run.
  They cover explicit activation, keyless/closed states, byte limits, history,
  running/stop, fixed errors, uncertain Start non-replay, desktop layout/focus,
  offline behavior and localized host evidence.
- Initial full frontend run overlapped the heavy Go suite and source edits:
  1827/1830 passed. Two App navigation tests hit their unchanged 5s deadlines;
  a catalog/raw-source snapshot mismatch occurred while trace keys were added.
  Isolated App retest: all 62 passed. Targeted translation/AI checks passed.
  These initial failures are retained here rather than treated as a green run.
- Final full frontend on `c537bf2`: **173 files, 1830 tests passed**, 351.87s.
  The code remained fixed throughout that run. Results are recorded in
  `evidence/checks.txt`. ESLint, TypeScript, root Go vet, gofmt and production build passed. The existing >500 kB
  vendor/chunk warning remains; no global performance claim is made.
- [Browser results](evidence/results.json): real React/settings/dock/Sheet
  components with a synthetic IPC adapter, Chromium on Linux. English 1440px light
  and Simplified Chinese 768px dark both passed explicit key/config activation,
  selector-only send, completed/stop feedback, host evidence, protected explicit
  account navigation and no horizontal overflow. Zero browser page errors.
  [Desktop](evidence/chat-1440.png) / [narrow Sheet](evidence/chat-768.png).
  These browser fixtures are not Wails/native/provider acceptance.

`browser.mjs` is reproducible with Node 24, Chromium and `PLAYWRIGHT_MODULE`
pointing to an installed Playwright module. It creates and removes its own
synthetic Vite harness; it never opens a household database or model connection.

## Native Mac and real-provider acceptance still required

Use the exact reviewed source and the repository's existing dev identity
(`com.nestworth.dev`, `Nestworth.app`). Follow
[isolated native QA](../../development/local-workflow.md#isolated-native-development-qa)
and verify explicit synthetic database/settings paths and the synthetic household
before launch. Never copy or upload a real ledger for testing. A first-time
onboarding run needs an app restart before the household-bound runtime is available.

1. Build/generate on the native Mac; verify bundle identity and isolated launch
   overrides. Ad-hoc development must not depend on Keychain or signing credentials.
2. Save an authorized test key locally in the UI, choose a Responses model ID
   actually available to that account, then explicitly enable. Confirm no provider
   request occurs during key/config save, startup or reopening the panel.
3. On the synthetic overview ask for current totals, currencies and missing inputs;
   on account list/detail ask about the selected account and inspect Data evidence.
   Check complete/partial values against the GUI, source dates and relevant pages.
   Named account questions disclose related account identities to the provider.
4. Verify actual Responses streaming/event/native-reasoning compatibility, model
   access and billing behavior. These cannot be inferred from a fake provider.
   Check Stop, invalid key/model, disconnected network, timeout, new conversation
   after context exhaustion, close/reopen, app restart and model switch.
5. Check keyboard/VoiceOver, narrow Sheet/dock resize and native focus. Confirm
   financial writes and mobile model calls remain unavailable. Explicit account
   buttons must respect pending/unknown financial review guards.
6. Check credential replacement/deletion and isolated backup/restore end to end.
   Database backups intentionally retain keys; replacing/deleting a live key does
   not remove historical backup copies. Model text is untrusted analysis, and
   direct key-taint guards are not general secret detection or forensic erasure.

Pending outcomes: native Mac GUI/package/accessibility, actual provider/model/
encrypted-signature compatibility, physical phone regression and full application
power-loss/backup-restore acceptance. No real API key was requested by the agent.
