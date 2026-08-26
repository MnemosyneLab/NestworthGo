# Nestworth Assets

This directory contains the brand artwork and native icon resources used by
the Wails desktop application, release packaging, documentation, and future
installers. Assets are kept outside Go packages so the same source artwork can
be reused by each target platform.

## Formal brand assets

| File | Intended use | Dimensions |
| --- | --- | --- |
| `brand/logo-mark.png` | Standalone Nestworth mark | 912 × 912 |
| `brand/wordmark.png` | Horizontal wordmark | 1501 × 301 |
| `brand/app-icon.png` | Application and launcher icon source | 1024 × 1024 |
| `brand/favicon.png` | Small documentation/web icon | 256 × 256 |

## Design explorations

`brand/drafts/` contains named artwork explorations. They are reference
material and are not copied into the application without an explicit asset
decision.

## Native icon package

`icons/` contains the platform PNG sizes, `icon.icns`, and `icon.ico` used by
Wails packaging. The generated `build/appicon.png` is the packaging input
produced from `brand/app-icon.png`.

## Usage rules

- Treat the formal brand files as the canonical visual identity.
- Do not overwrite originals in place; add a named variant for an intentional
  design change.
- Keep user-imported media outside this directory.
- Do not commit build output, generated bundles, `.DS_Store`, or personal data.
