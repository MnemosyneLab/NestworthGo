# Bundled bank logos

The institution and account icon picker offers generic icons and a searchable
bank/financial logo category. Accounts start with an empty icon override and
inherit their institution's current icon. The reset control clears only that
account's override. Without an institution, the account-type icon is shown.
Existing explicit choices are retained, even if they equal a former default.

`icon_key` already supports an empty string; the business schema stays at 15.
The Account DTO now preserves that sentinel instead of substituting a type
icon. Create/edit, list, detail and portfolio views share the same resolution
rule. Verified SQLite snapshots retain both overrides and inheritance. The existing
version 2 JSON business export deliberately excludes UI icon keys; that contract
is unchanged. JSON export is not a supported restore format; restore uses the
existing SQLite backup path.

## Source and rights

- Source: https://github.com/icongo/bank-logos
- Pinned commit: `ffca539a043900fbf2a4fd6a5d32f1706ae5dfd1`
- Actual tree: **608 SVGs** (424 `logos/`, 184 `other/`), all usable by the
  screened path-only renderer. The README's “612” differs from the actual tree.
- MIT, Copyright (c) 2022 IconGo; full license and notice ship in
  `frontend/public/bank-logos/` and therefore in the embedded production app.
- Trademark rights are not granted by the repository license. Inclusion does
  not imply endorsement or affiliation.
- Labels and aliases use the pinned README's image alt text and original
  filenames. They retain source spelling, historical names and variants;
  no unverified expanded bank identities or translated brand names are added.
  Search is case-insensitive and ignores spaces, hyphens and underscores.

`tools/bank-logos/manifest.json` records the source path and original/sanitized
SHA-256 for every file. `UPSTREAM-README.md` retains the source name evidence.
IDs use `bank-logo:` followed by the source slug; `other/` files use a stable
hash of their source-relative path to avoid collisions and unsafe filenames.
Do not rename IDs when updating display names.

## Import and rendering

To reproduce, check out the pinned upstream revision in a separate clean
checkout and run:

```sh
python3 tools/bank-logos/import_logos.py /path/to/bank-logos
gofmt -w internal/domain/bank_logos_generated.go
python3 -m unittest discover -s tools/tests -v
```

The importer validates every asset before writing output. It rejects DTDs,
entities, processing instructions, scripts, event attributes, external
references, stylesheets, foreign objects, animation, unexpected namespaces,
invalid viewBoxes and unsupported markup. Only a root SVG and static paths
with local solid paint are retained. Reviewed root sizing/editor metadata is
removed; original viewBoxes, paths and colors are preserved.

Assets are ordinary bundled files, rendered with `img` and `object-contain`;
there is no raw SVG injection, remote URL construction or runtime download.
Lookup must succeed in the fixed catalog before a local asset URL is used.
Unknown IDs use the existing generic fallback. Logos have a small white
backplate in both themes so source black/colored artwork remains legible;
SVG backgrounds stay as supplied and no artwork is stretched or recolored.
The picker uses 304 reviewed presentations rather than exposing both variants.
`tools/bank-logos/presentation.json` explicitly groups all 608 original IDs;
`frontend/src/lib/bankLogoPresentation.json` is the generated display catalog.
Most groups use a symbol. The 33 groups whose compact source still includes
lettering retain one wordmark fallback rather than losing that institution.
Monograms and stylized letters that form the brand symbol remain intact.
The `other/` pairs are explicitly reviewed: filename suffixes alone are not
reliable (for example, the Shenzhen Commercial and Great Wall West China
pairs put the symbol first). Search retains names and aliases from both sources.

Old saved IDs resolve to the preferred presentation and show its selected state,
but are never rewritten by rendering or opening the picker. Only an explicit
selection changes an override. The original catalog, source assets, Go allowlist,
snapshot behavior and schema remain unchanged.

Display assets under `bank-logos/display/` preserve every original path and paint;
only the viewBox changes. Chromium path bounds exclude a first white backplate
covering the source canvas (within 1% for source rounding), include all foreground
paths, and add 3.5% breathing room on each side. This removes blank canvas without
stretching, recoloring, or cropping foreground artwork. A white backplate remains
in both themes. No runtime measurement or SVG injection is used by the app.

Regenerate after reviewing any source change (Playwright is a development-only
tool, not an application dependency):

```sh
NODE_PATH=/path/to/playwright/node_modules node tools/bank-logos/build_presentation.cjs
```

Set `CHROMIUM_PATH` if Chromium is not at `/usr/bin/chromium`. Re-run this after
importing the upstream catalog. Picker marks are 48px, selected previews 36px,
and account/institution list and detail slots 32px; wider wordmarks retain their
natural proportions with `object-contain`.
The image payload is outside the JS bundle and picker images load lazily.

## Verification

Automated coverage includes asset hashes and SVG rejection cases, persistence
and verified snapshot reopen, the unchanged JSON export contract, inheritance/override/reset,
institution changes, unknown IDs, search/category selection, keyboard use,
localization, and unchanged generic choices. The standard Python CI discovery
runs the asset screening tests alongside the existing installer tests.

Chromium component review checks all 608 images for successful decoding and
keyboard override/reset, plus light/dark screenshots. This is frontend browser
verification with synthetic state, not native macOS/Wails acceptance. No real
ledger, provider credentials or R2 account is used.
