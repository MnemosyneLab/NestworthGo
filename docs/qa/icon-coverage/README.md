# Selected icon visual QA

Chromium/Linux, synthetic data, actual IconPicker and EntityIcon. Each tile
shows 20px, 24px and 32px renderings. New marks and corrected old-ID glyphs were
inspected in both themes at 1000px and 390px, without clipping or stretching.
All four branded SVGs decoded. Cross-language search for 以太坊 from an English
instrument picker, keyboard selection and Escape focus restoration passed in
all four browser configurations.

- [Desktop light](light-1000.png) · [Desktop dark](dark-1000.png)
- [Narrow light](light-390.png) · [Narrow dark](dark-390.png)

Reproduce by temporarily copying `review.tsx` to
`frontend/src/coverage-review.tsx` and `review.html` to
`frontend/coverage-review.html`, starting Vite on 9245, making
`/tmp/nestworth-icon-review`, then running `review.cjs` with Playwright available
through NODE_PATH and Chromium at `/usr/bin/chromium`. Remove the temporary
frontend files afterwards. These are component screenshots, not native Wails
acceptance. The real directory and account edit Escape checks are documented
in [the bank regression evidence](../bank-logos/symbols/README.md).

The Library upload helper is unavailable in this executor (HTTP 401); these
images are committed for review rather than claiming a successful Library save.

Validation: 642/642 full frontend tests, 96 focused tests, 19 Python checks,
Go domain/application tests, TypeScript, ESLint and production build passed.
Build still reports >500KB chunks. The initial bank head passed both GitHub
test/race jobs; updated-head CI is reported on the draft PR.
