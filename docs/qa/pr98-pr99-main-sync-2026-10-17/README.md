# PR98 integration and verification evidence

This directory retains raw frontend check output and synthetic UI evidence used to complete PR98 after merging the already-merged PR99 changes.

## Source and scope

- PR99 squash merge on `main`: `bb6f8e9f18ff9009e1d84885aacff84f9d31cc58`.
- PR98 branch before sync: `221bc429a1f6df37538547db668e086ad305c8ff`.
- PR98 sync merge commit: `7cf7cd49e78080889c23c1968d0c223ec886a295`.
- Focused app test commit: `76dfef803895c1639a4a423b8f7338ea179a19d3` (real `App.test.tsx` round trip, in-memory Wails fixtures only).
- Typed navigation cleanup commit: `838cdf48d7d633c17b253d892e0f4967f4bf3eb5`.
- Browser screenshot and trace source checkout: isolated synthetic integration run `36a2366c9a0f8408eac6325fac43ec3c61fde061`, from PR98 and PR99 tips before the PR99 squash merge. The Playwright flow and assertions are retained here. Screenshots illustrate the same real application route flow; they are not a native desktop run.
- No real ledger data was used.

## Current successful local frontend checks

- `pnpm run generate:bindings`: passed with Wails `v3.0.0-beta.28` and `TMPDIR=/workspace/.tmp`.
- `pnpm run check:bindings`: passed; generated bindings are current.
- `./node_modules/.bin/vitest run src/App.test.tsx -t 'preserves filtered page two through the real instrument route' --reporter=dot`: 1 passed, 39 skipped.
- `pnpm run typecheck`: passed with `CGO_ENABLED=0` (the TypeScript compiler emitted no diagnostics).
- `pnpm run lint`: passed.
- `pnpm run build`: passed; Vite emitted its existing chunk-size warning for the 800 kB PageState bundle.
- `pnpm run test`: 140 files / 1,508 tests passed, 0 failed.

The corresponding raw stdout is in `bindings-generate-retry.log`, `bindings-check.log`, `targeted-app-vitest.log`, `typecheck-retry.log`, `lint.log`, `build.log`, and `full-frontend-test.log`.

## Initial environment attempts

`bindings-generate.log` records an initial generation attempt that failed when the default temporary filesystem ran out of space; retrying with workspace-backed `TMPDIR` passed. `typecheck.log` records an initial bindings-generation failure because this Linux environment lacks GTK4/WebKitGTK development packages; retrying with `CGO_ENABLED=0` passed. These are retained as raw setup logs and are not reported as passing checks.

## Synthetic browser screenshots and trace

`pr98-pr99-playwright-flow.mjs` runs a headless Chromium flow at 390×844 through Overview → Investment Review → filtered page two → Instrument Detail Holdings → Back twice. Wails service responses use synthetic in-memory fixtures, while app navigation is real. `browser-results.json` records the assertions, page errors, and focus destinations. `pr98-pr99-second-page.trace.zip` retains the Playwright trace. Screenshots are under `screenshots/`; `07-review-phone-390.png` is the 390×844 viewport capture.

These browser artifacts are from the isolated pre-squash integration checkout noted above. The later current-branch focused Vitest run separately exercises the real App route round trip and focus restoration.

## Platform and remaining acceptance

The Linux frontend build and browser/Vitest checks do not constitute a native macOS GUI run. Native macOS acceptance is deferred to the parent task’s Atlas environment. The performance budget and incremental replay acceptance also remain deferred; they are not claimed as passed here. Go test, vet, and race checks are expected from PR98’s exact-head GitHub CI run.
