# Nestworth Assets

These assets were migrated from the original Nestworth application and are
kept separately from Go packages so they can be reused by Fyne windows,
release packaging, documentation, and future installers.

## Formal brand assets

| File | Intended use | Dimensions |
| --- | --- | --- |
| `brand/logo-mark.png` | Standalone Nestworth mark | 912 × 912 |
| `brand/wordmark.png` | Horizontal wordmark | 1501 × 301 |
| `brand/app-icon.png` | Application and launcher icon source | 1024 × 1024 |
| `brand/favicon.png` | Small documentation/web icon | 256 × 256 |

## Draft artwork

`brand/drafts/` contains the original design explorations: `AppIcon.png`,
`AppIcon2.png`, `LogoMark.png`, `NestRings.png`, and `Wordmark.png`. These are
reference material and are not current application assets.

## Native icon package

`icons/` contains the migrated platform icon set, including PNG sizes,
`icon.icns`, and `icon.ico`. The macOS packaging workflow uses `icon.icns` for
the `.app` bundle and the DMG volume icon. The Go/Fyne packaging workflow
regenerates platform metadata from the canonical brand PNG and then preserves
this native ICNS resource.

The runtime embeds a byte-identical copy of `brand/app-icon.png` at
`internal/app/resources/app-icon.png`, so the application and About dialog keep
their icon when launched outside the repository directory.

## Usage rules

- Treat the formal brand files as the canonical visual identity.
- Do not edit or overwrite the originals in place; add a new named variant when
  a design change is intentional.
- Keep user-imported media outside this directory.
- Do not include build output, generated bundles, `.DS_Store`, or personal data.
