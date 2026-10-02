# Selected financial icons

The generic picker keeps the existing Lucide visual system and adds only 12
choices: 8 line icons and 4 explicit digital-asset brand marks. No new frontend
dependency, business type or database schema is introduced. All 75 previous
non-bank IDs and their selection availability remain compatible.

| ID | Choice | Lucide glyph / source |
| --- | --- | --- |
| `fund` | Fund / ETF | Layers |
| `bond` | Bond | ScrollText |
| `term-deposit` | Term deposit | Timer |
| `lending` | Lending | HandCoins |
| `land` | Land | LandPlot |
| `commercial-property` | Commercial property | Store |
| `collectible` | Collectibles | Amphora |
| `digital-asset` | Digital assets | Blocks |
| `crypto-logo:btc` | Bitcoin brand | spothq color/btc.svg |
| `crypto-logo:eth` | Ethereum brand | spothq color/eth.svg |
| `crypto-logo:sol` | Solana brand | spothq color/sol.svg |
| `crypto-logo:usdc` | USD Coin brand | spothq color/usdc.svg |

These line icons are visual suggestions, not exclusive standardized symbols
for asset classes. Timer emphasizes the fixed term; Layers represents a grouped
investment. Existing house, car, Gem, protection and cash choices are retained.

## Selection and rights

- [Lucide](https://lucide.dev/license): existing dependency, consistent line
  style, works offline and inherits theme color. ISC with the bundled MIT
  notices for Feather-derived icons. Its full package license ships under
  `public/icon-licenses/`. Selected glyphs were verified in installed v1.32.0.
- [spothq/cryptocurrency-icons](https://github.com/spothq/cryptocurrency-icons):
  only the four reviewed color SVGs, pinned at
  `1a63530be6e374711a8554f31b17e4cb92c25fa5`. CC0-1.0 license and notice ship
  beside the assets. Brand/trademark rights remain with their owners. Static
  SVG screening rejects scripts, external references, styles and unexpected
  elements/attributes; source hashes and all original bytes are retained.
- Additional Tabler/Phosphor families were not selected in the approved plan:
  the current Lucide exports cover these gaps without another visual system or
  package. The approved shortlist is intentionally small.

Regenerate the four assets offline from a clean pinned checkout using:

```sh
python3 tools/crypto-logos/import_logos.py /path/to/cryptocurrency-icons
```

## Registry, compatibility and UI

`iconCatalog.ts` is the single registry for generic choices and their renderer.
Each entry must declare either a Lucide component or a bundled brand file.
`EntityIcon` uses the same lookup; the Go stable-ID allowlist is checked against
it by Python tests. Only an explicit selection picks a brand: account/instrument
symbols and names are not automatically mapped. The old `bitcoin` ID still
renders the original Lucide Bitcoin line icon.

Stored vault, percentage, calendar-clock, wallet-cards and currency badge IDs
now render their matching Lucide glyphs instead of unrelated Archive,
TrendingUp, Calendar, CreditCard or dollar fallbacks. Money-in/out IDs use the
corresponding directional Banknote glyphs. Aliases that are genuinely synonymous
remain aliases.

Categories put accounts, banking, investment, physical assets, liabilities and
receivables, protection, digital assets and currencies ahead of general/system
utilities. The latter remain selectable. Brand choices are available to every
existing generic-picker entity kind, including instruments and groups. Search
matches EN, simplified Chinese and traditional Chinese labels regardless of
current language, plus stable IDs. Generic/crypto grid thumbnails are 24px;
color brands keep their native circular backgrounds and proportions.

Snapshot round-trip tests cover all new IDs, explicit overrides and the empty
inheritance sentinel. Unknown IDs stay local generic fallbacks and never become
URLs. JSON business export continues excluding UI icon keys.
