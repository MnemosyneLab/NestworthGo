# macOS investment-amendment acceptance

The public PR #302 baseline and exact Wails reference build were `93f4a85f`. The tested Gio application source was `80a249ce`. Both signed QA apps ran natively on Mac mini with macOS 27.2, arm64 and Go 1.27.4. Wails used its native WebView and real Go host with the production frontend bundle from identical frontend source. App language was English and the appearance was Light. Launch verified the fixed bundle identity and independent database and settings paths with the synthetic household marker.

The four primary profiles started with copies of the service-generated `Synthetic PR302 investment amendments` ledger. Seed SHA-256 `3692a68f0ab9d53c72c9f7e71843d5879048f60473bd4473c8535f53cb07d58c`. At 1280×800 we opened and cancelled each draft, then entered the corrected values, reviewed, and committed. At 900×600 we cancelled and separately confirmed all three reversals. The Gio confirmations were double-clicked. In Wails use the row's Actions → Fix or Undo menu. In Gio filter by the named account and open its row's More actions. The original record also exposes the actions in its detail drawer.

| Activity | Correction input | Cash and holdings after correction | Cash and holdings after reversal |
| --- | --- | --- | --- |
| Buy, Sample Buy Desk / PRBUY | Quantity 8.5, gross 85.125 EUR, fee 0.25 EUR | 914.625 EUR, 8.5 units | 1000 EUR, 0 units |
| Sell, Sample Sell Desk / PRSELL | Quantity 5.25, gross 93.5 EUR, fee 0.375 EUR | 893.125 EUR, 14.75 units | 800 EUR, 20 units |
| Cash dividend, Sample Dividend Desk / PRDIV | Amount 13.4567 USD | 43.4567 USD and 900 EUR, 10 units | 30 USD and 900 EUR, 10 units |

On closing the apps, SQLite in all four profiles matched these exact amounts, currencies, and quantities. Each correction ledger had three original-linked correction reversals and three replacements, with both sides sharing the original effective time and the correction group. Each reversal ledger had three current-time reversals linked to their originals. Cash dividend fields use the cash currency even when the instrument quotes in EUR.

Gio reuses the guarded preview and commit pipeline. The correction uses the recorded transaction kind, account, instrument, holding and currency. Quantity, gross, fee and note are editable for trades. Cash received and note are editable for dividends. Wails offers a more general replacement form. Gio adds explicit exact-effect review to the reversal flow. Cancelling from a row action restored the active account filter and row action focus. Returning from the posted receipt focused View replacement or View reversal on the original transaction. Wails restored the row action on cancellation, and the WebView document received focus after posting.

The dependent buy (PRDEP) was recorded for 10 units, followed by a sale of 7. A correction to 6 units and an attempt to reverse the buy were both refused in native testing. SQLite retained the quantity of 3 and 989.375 EUR with no additional links. The holding-associated fee and tax kept their money-removed details and remained ineligible for investment amendments in Gio.

Both 900×600 apps ran additional independent profiles seeded with a later 889 EUR withdrawal at Sample Sell Desk and a later 42 USD withdrawal at Sample Dividend Desk. Seed SHA-256 `829cf05c9e5e571b03293ceff1ee506de53871a96b1bf1ed3552c04ed3bdf764`. Reducing sale gross to 89 EUR or the dividend to 11 USD failed historical replay. Current-time reversals were rejected for insufficient cash. In Gio neither review exposed Confirm, and Cancel returned to the selected row and scope. All eight compared SQLite tables, covering activities, effects, detail, audit groups, cash and holdings, matched the seed in both clients. Sell cash remained 0.9167 EUR with 15.5 units. Dividend cash remained 0.3456 USD and 900 EUR with 10 units.

## Original native captures

| Scenario | Gio | Wails |
| --- | --- | --- |
| Buy correction at 1280×800 | [Review](native/gio-wide-buy-review.jpg) · [Posted](native/gio-wide-buy-posted.jpg) | [Review](native/wails-wide-buy-review.jpg) · [Posted](native/wails-wide-buy-posted.jpg) |
| Buy reversal at 900×600 | [Confirmation](native/gio-narrow-buy-reverse.jpg) · [Posted](native/gio-narrow-buy-reversed.jpg) | [Confirmation](native/wails-narrow-buy-reverse.jpg) · [Posted](native/wails-narrow-buy-reversed.jpg) |
| Sell correction at 1280×800 | [Review](native/gio-wide-sell-review.jpg) · [Posted](native/gio-wide-sell-posted.jpg) | [Review](native/wails-wide-sell-review.jpg) · [Posted](native/wails-wide-sell-posted.jpg) |
| Sell reversal at 900×600 | [Confirmation](native/gio-narrow-sell-reverse.jpg) · [Posted](native/gio-narrow-sell-reversed.jpg) | [Confirmation](native/wails-narrow-sell-reverse.jpg) · [Posted](native/wails-narrow-sell-reversed.jpg) |
| Cash dividend correction at 1280×800 | [Review](native/gio-wide-dividend-review.jpg) · [Posted](native/gio-wide-dividend-posted.jpg) | [Review](native/wails-wide-dividend-review.jpg) · [Posted](native/wails-wide-dividend-posted.jpg) |
| Cash dividend reversal at 900×600 | [Confirmation](native/gio-narrow-dividend-reverse.jpg) · [Posted](native/gio-narrow-dividend-reversed.jpg) | [Confirmation](native/wails-narrow-dividend-reverse.jpg) · [Posted](native/wails-narrow-dividend-reversed.jpg) |
| Prefilled buy and dividend forms | [Buy](native/gio-wide-buy-form.jpg) · [Dividend](native/gio-wide-dividend-form.jpg) | [Buy](native/wails-wide-buy-form.jpg) · [Dividend](native/wails-wide-dividend-form.jpg) |
| Buy audit and focus | [Cancel to original row](native/gio-wide-buy-cancel-focus.jpg) · [Original after correction](native/gio-wide-buy-original-return.jpg) · [Original after reversal](native/gio-narrow-buy-original-return.jpg) | |
| Dependent buy blocks | [Correction](native/gio-wide-dependent-blocked.jpg) · [Reversal](native/gio-narrow-dependent-blocked.jpg) | [Correction](native/wails-wide-dependent-blocked.jpg) · [Reversal](native/wails-narrow-dependent-blocked.jpg) |
| Sell proceeds already spent | [Blocked correction](native/gio-narrow-restricted-sell-fix.jpg) · [Blocked reversal](native/gio-narrow-restricted-sell-undo.jpg) | [Blocked correction](native/wails-narrow-restricted-sell-fix.jpg) · [Blocked reversal](native/wails-narrow-restricted-sell-undo.jpg) |
| Cash dividend proceeds already spent | [Blocked correction](native/gio-narrow-restricted-dividend-fix.jpg) · [Blocked reversal](native/gio-narrow-restricted-dividend-undo.jpg) | [Blocked correction](native/wails-narrow-restricted-dividend-fix.jpg) · [Blocked reversal](native/wails-narrow-restricted-dividend-undo.jpg) |
| Holding-associated cash outflow | [Fee detail](native/gio-wide-protected-fee.jpg) · [Tax detail](native/gio-wide-protected-tax.jpg) | |

Original JPEGs are unchanged in size and bytes. Gio captured 2560×1600 and 1800×1200 Retina pixels. Wails captured 2558×1600 and 1798×1200 including its native title bar.

## Automated and remaining checks

The macOS Gio history and localization packages plus the desktop history/custody regression set passed at `80a249ce`. This covers original effects, audited replacement and reversal, sell cost and realized gain, duplicate mutation retry, standalone bank expense and fee, and cancellation. The additional spent-proceeds tests at `580e697f` compare Gio to the Wails service and assert the absence of new writes. Automated tests use shared application services and independent SQLite profiles.

Broader replacement type or endpoint changes, archived holding corrections, VoiceOver and Linux native windows were outside this acceptance. The full original ledger activity parity checklist is larger than the investment-amendment increment.

The subsequent Mac mini investigation verified cross-type and cross-endpoint replacement, editable dividend cash currency, and the archive boundary. [Replacement follow-up and raw native evidence](replacement-followup.md).
