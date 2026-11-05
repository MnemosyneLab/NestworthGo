# AI live conversation browser acceptance

Synthetic-only Linux Chromium checks for the real ConversationTimeline,
AssistantMarkdown, theme CSS and ResizeObserver behavior. No ledger, desktop
host or provider network is used. The fixture deliberately includes long
Markdown, an unfinished code fence, older turns and a real disclosure element.

Run after installing locked frontend dependencies and generating Wails bindings,
with Node 24, Chromium and an external Playwright Core installation:

```sh
PLAYWRIGHT_MODULE=/absolute/path/to/playwright-core/index.mjs \
AI_BROWSER_OUTPUT=/tmp/nestworth-ai-live-browser \
node docs/qa/ai-live-2026-11-05/browser.mjs
```

Results in `results.json` passed both light/dark themes: chunk following, tool
expansion height following, sidebar-width changes, manual scroll pause, explicit
resume, 390px no-overflow and session isolation. No browser page errors.
Generated screenshots are synthetic previews only. They are not evidence of a
native Wails host, live provider streaming, Mac geometry or VoiceOver behavior.
The Go provider/runtime tests cover the actual SSE/EOF, tool and persistence
paths separately; frontend tests cover incremental state/details and Markdown
safety. Live-provider quality and latency remain unmeasured.
