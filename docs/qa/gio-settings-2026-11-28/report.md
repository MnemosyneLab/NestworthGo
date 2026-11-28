# macOS Gio market settings follow-up

Both signed QA apps ran natively on Mac mini (macOS 27.2 arm64, Go 1.27.4). Wails used its Go host and native WebView. The common baseline was `43f99113` and the fixed Gio package was built from `dac716ef`. The Wails Go executable was rebuilt at the baseline with an existing production bundle from identical frontend source. Both apps used Simplified Chinese, Light appearance, and independent copies of the same `Synthetic instrument performance` seed. The launcher checked bundle identity, binary revision and isolated paths before starting.

At 1280×800 the baseline Gio pointer click selected Market and left focus on the page container. Tab then focused Toggle sidebar. Keyboard Right from Appearance selected and focused Market, and Tab moved to Data. The credential reveal button was centred against the combined label and editor. Both Wails sizes advanced from the selected Market tab into its panel, then FX provider, quote cache duration and the CoinGecko editor. Reverse Tab crossed those controls in reverse order. In this native WebView, Option+Tab from the editor reached the reveal button. Space revealed the fake value and mouse activation hid it again.

The Gio fix sets focus on tab activation and gives the Market tab and its first control a local Tab/Shift+Tab transition. There is no extra panel focus stop in Gio. The editor and reveal button use the same height and align at the bottom of the row. Both adjustments are local to Settings.

On both Gio sizes, pointer selection and Left/Right activation focused Market. Tab moved through FX provider → quote cache duration → CoinGecko input → reveal. Shift+Tab traversed the same controls in reverse and returned to Market. The focus indicators are in the originals below. Space on the reveal button displayed `test-only-demo-key`; mouse activation masked it again. The fake value was never saved or sent to a provider. After closing the apps, the four isolated settings files contained none of the draft value.

| Window | Baseline Gio | Fixed Gio | Wails reference |
| --- | --- | --- | --- |
| 1280×800 | [Before alignment](native/gio-wide-before-shown.jpg) · [Mouse then Tab](native/gio-wide-before-mouse-tab.jpg) · [Arrow then Tab](native/gio-wide-before-keyboard-tab.jpg) | [Focus on FX](native/gio-wide-after-forward-fx.jpg) · [Reverse focus on Market](native/gio-wide-after-reverse-market.jpg) · [Reveal focus](native/gio-wide-after-forward-show.jpg) · [Masked](native/gio-wide-after-hidden.jpg) · [Revealed](native/gio-wide-after-shown.jpg) | [Focus on FX](native/wails-wide-forward-tab.jpg) · [Masked](native/wails-wide-hidden.jpg) · [Revealed](native/wails-wide-shown.jpg) |
| 900×600 | [Before alignment](native/gio-narrow-before-shown.jpg) | [Focus on FX](native/gio-narrow-after-forward-fx.jpg) · [Reverse focus on Market](native/gio-narrow-after-reverse-market.jpg) · [Reveal focus](native/gio-narrow-after-forward-show.jpg) · [Masked](native/gio-narrow-after-hidden.jpg) · [Revealed](native/gio-narrow-after-shown.jpg) | [Focus on FX](native/wails-narrow-forward-tab.jpg) · [Masked](native/wails-narrow-hidden.jpg) · [Revealed](native/wails-narrow-shown.jpg) |

Original window captures retain their dimensions. Gio captured 2560×1600 and 1800×1200 Retina pixels. Wails captured 2558×1600 and 1798×1200, including the native title bar.

The macOS Gio `TestSettings` desktop suite passed on the fix source. Credential save/retry and connected service settings were outside the native follow-up.
