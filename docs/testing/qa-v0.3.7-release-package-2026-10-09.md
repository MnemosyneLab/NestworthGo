# v0.3.7 release package acceptance

**Status:** Published release assets were verified through GitHub's release
API; limited package acceptance is user-reported. This record combines public
asset metadata with release-task and local payload-check results reported by the
user. The local task report was not available in this cloud workspace and was
not independently inspected here.

## Release identity and assets

- Release: [Nestworth v0.3.7](https://github.com/MnemosyneLab/NestworthGo/releases/tag/v0.3.7)
- Published: `2026-10-09T00:17:23Z`; release is neither draft nor prerelease.
- Tag/commit: `4d72b3dfaac34b86f24923760663ef0c11c3ec54` (v0.3.7, build 8).
- Five uploaded assets and the GitHub-reported SHA-256 digests:

| Asset | Size | SHA-256 |
| --- | ---: | --- |
| `Nestworth-0.3.7-arm64.dmg` | 22,706,083 bytes | `7aa4ca054bd7d1625d41ea8699f70bb03a60931a71c89599c468cd30183e7805` |
| `Nestworth-0.3.7-arm64.zip` | 18,446,687 bytes | `85f01d27d2ed56e16253d0315f6ba195c89d53f91734e5fe316c4e24211bd5ec` |
| `nestworth-skill.tar.gz` | 35,857 bytes | `c010294ec767c8f9fcf921047b628d35966d4978ae8c273ebaf79cbc9371c48c` |
| `nestworth-skill.tar.gz.sha256` | 89 bytes | `46cf7ac587abac7037e6ec168a8d17073078b98963ed437f648ea45a7c4e2b69` |
| `SHA256SUMS` | 184 bytes | `da0048e5153b188f3d76ee5f1687ed6ee1cd94ded4ac3cbfa6e714a645888f6f` |

The listed digests are GitHub asset digests, not a claim that this cloud task
downloaded and re-hashed the payloads. The release includes the adjacent skill
checksum file and DMG/ZIP checksum manifest.

## Reported package checks

The user reports that the full release packaging task and 51 tool checks passed
on Go 1.26.8. The extracted app payload from both DMG and ZIP passed synthetic
onboarding followed by quit, relaunch, and persistence checks. These targeted
package checks establish those tested flows only; they are not a full GUI
regression.

The user reports that app bundle metadata declares arm64 and a macOS 12
deployment target. macOS 12 runtime behavior was not tested. The published app
is ad-hoc signed; it is not Developer ID signed and was not notarized.

## Separate acceptance and remaining limits

Real Codex/MCP/skill client acceptance remains the separately scoped run at app
source `2cad89eddd0c02de28f68a631fc26a72d6f6fcc3`, recorded in
[`qa-mcp-skill-acceptance-2026-10-09.md`](qa-mcp-skill-acceptance-2026-10-09.md).
Do not describe that earlier client test as package acceptance. The user's full
GUI regression was explicitly deferred. Other managed-product write variants
and real market/provider connections were not covered by the reported package
checks.
