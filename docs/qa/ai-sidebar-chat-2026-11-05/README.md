# Continuous AI sidebar

The desktop assistant now reads as a continuous conversation: right-aligned user
messages, plain assistant replies, and collapsed completed tool reads attached to
their answer. Conversation selection/new/delete remain in the compact header;
ordinary Send and the read-only provider indicator stay beside the composer.
The byte counter is accessible but visually hidden until an overflow error.

The design borrows interaction patterns, without adding UI dependencies, from
[assistant-ui conversation management](https://www.assistant-ui.com/docs/primitives/thread-list),
[AI Elements tool disclosures](https://elements.ai-sdk.dev/components/tool) and
[explicit confirmations](https://elements.ai-sdk.dev/components/confirmation).
Existing Nestworth Button, Input, NativeSelect, InfoPopover and Sheet own control,
focus and theme behavior. Message rendering remains text-only and React-escaped.

History uses existing IPC: Inspect for the latest attempt, five preceding
Messages reads initially, five more on explicit loading. Pages accumulate in
chronological order. Updating the latest attempt never switches to an exclusive
turn view. Failed history fetches retry independently without replaying Start.
The viewport follows only while the reader stays near the bottom; a Latest
messages button resumes following after scrolling away.

Only real host read evidence is displayed. Its details show a localized
allowlisted tool name, read start time recorded before dispatch, paging and
conservative missingness. CapturedAt is not a completion or quote timestamp.
There are no raw tool arguments or results in the UI and no invented per-tool
progress. Run status and Stop remain visible for active attempts; fixed safe
diagnostics remain visible for failures. Privacy, scope, page-context and
retention details live in the info popover. Page navigation retains a separate
explicit per-message Send action inside a disclosure; disclosure itself does not
grant permission. Closing the sidebar retains the conversation.

## Evidence

`browser.mjs` mounts the real React sidebar with synthetic IPC responses. It does
not open a ledger, launch a native app, retrieve credentials, or call a provider.
The fixture key/model and all message/account content are synthetic. Run with
Node 24.22.0, an installed Playwright module and Chromium:

```sh
PLAYWRIGHT_MODULE=/path/to/playwright-core/index.mjs \
AI_BROWSER_OUTPUT=/tmp/ai-sidebar-browser \
node docs/qa/ai-sidebar-chat-2026-11-05/browser.mjs
```

[Results](results.json) cover 1440px English light/dark, 320px Simplified Chinese
light and 390px Traditional Chinese dark: completion, cancellation while offline,
selector-only Start, continuous history/loading earlier, preserved reading
position during completion, keyboard disclosure, explicit navigation and its
review guard, page-target clicks preserving a nonempty draft without sending it,
zero provider calls, no horizontal overflow and no browser errors.

- [Desktop light](desktop-light.png), [desktop dark](desktop-dark.png)
- [Narrow light](narrow-light.png), [narrow dark](narrow-dark.png)

Automated frontend evidence is recorded in the PR against its exact head. The
Linux environment generated Wails bindings using Go 1.27.3 and the Wails purego
build tag (GTK/WebKit development headers are unavailable). This is browser/jsdom
verification, not native macOS, VoiceOver, packaging or live-provider acceptance.
No backend runtime or resource budgets were changed. The bounded UI may retain
up to the existing 64 turns and fetches five older turns per explicit load;
there is no new runtime polling or provider traffic.

## Budget integration and draft-submission regression

After synchronizing main's budget finalization fix (#229), the nine exact budget
subcodes and their safe diagnostic display live in ConversationTimeline. The Go
vocabulary contract checks that actual owning component; runtime and frontend
allowlists still have exactly the same nine rules. All budget guidance and
unknown-code rejection tests remain. The page-context/navigation orchestrator
continues to live in ModelConversation and PageContext.

Account target buttons inside the composer explicitly use `type="button"`.
A regression test types a nonempty draft, opens navigation options and clicks an
account target: navigation occurs, the draft is retained, and neither Start nor
GrantNavigation is invoked. The browser fixture checks the same boundary.

Already observed latest answers remain in conversation-scoped memory while their
positional history request is pending or failed. The epoch/conversation key
isolates that memory on session changes. A persistent polite status region
announces each new successful reply once, without moving focus; loading existing
history and repeated polling do not announce old replies. Immediate first replies
are associated with the current explicit submission request.
