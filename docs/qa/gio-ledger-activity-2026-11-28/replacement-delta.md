# Native acceptance of additional investment replacements

The delta began at PR #302 head `8d36a278d7d5e58721084e21828bdaa9822df624`. Native Wails used `93f4a85ffc9a99a341c6354e7e2f8a5aacda4fa9`. The initial native Gio matrix used `9b14b8128ca18bb169ec7e7fde9fc387d48e8a0a`. Their corresponding application sources match the delta baseline. Both apps ran on Mac mini with macOS 27.2, arm64 and Go 1.27.4, in English at 1280×800 with Light appearance. Launch verified the fixed QA identities `com.nestworth.dev` and `com.nestworth.gio` and the independent synthetic database and settings paths. It also verified the household markers in the cash, position and debt fixtures.

## Native replacement matrix

Open **Ledger activity**, filter by the original account, and use the row's **Actions → Fix** in Wails or **More actions → Correct buy**, **Correct sell** or **Correct dividend** in Gio. Select the replacement type, fill the named endpoints and exact values below, then preview and confirm. Both native apps submitted all eight accepted cases. The two PRPOS cases use independent copies of the same position fixture. The debt and value rows run in the shown order on their own paired profiles.

| Original and replacement | Entered values and behavior | Wails | Gio |
| --- | --- | --- | --- |
| PRBUY buy → Cash transfer | Send 17.25 EUR from Sample Buy Desk to Sample Dependent Desk with a 0.1255 EUR source fee. The exact balance at original time is 982.6245 EUR at the source and 1017.25 EUR at the destination. | [Preview](replacement-delta-native/wails-transfer-preview.jpg) | [Preview](replacement-delta-native/gio-transfer-preview.jpg) |
| PRSELL sell → Money removed | Record a 13.4567 EUR expense in Sample Sell Desk. The replacement leaves 786.5433 EUR at the original time. | [Preview](replacement-delta-native/wails-expense-preview.jpg) | [Preview](replacement-delta-native/gio-expense-preview.jpg) |
| PRDIV dividend → Currency conversion | In Sample Dividend Desk, sell 12.3456 USD, buy 10.125 EUR and enter a 0.1234 USD fee. The exact balances at original time are 17.531 USD and 910.125 EUR. Selling 31 USD was refused against the historical USD 30 available balance. | [Preview](replacement-delta-native/wails-fx-preview.jpg) · [Insufficient USD](replacement-delta-native/wails-fx-insufficient.jpg) | [Preview](replacement-delta-native/gio-fx-preview.jpg) · [Insufficient USD](replacement-delta-native/gio-fx-insufficient.jpg) |
| PRPOS buy → Investment transfer | Move 2.5 PRPOS units from Sample Position Source to the existing holding in Sample Position Destination. A transfer of 5 units was refused because the later eight-unit sale would fail. | [Preview](replacement-delta-native/wails-position-transfer-preview.jpg) · [Later sale refusal](replacement-delta-native/wails-position-transfer-refused.jpg) | [Preview](replacement-delta-native/gio-position-transfer-preview.jpg) · [Later sale refusal](replacement-delta-native/gio-position-transfer-refused.jpg) |
| PRPOS buy → Investment quantity adjustment | Select **Add** on the source holding. Enter 1.25 units and unit cost 10.125 EUR. Removing 5 units was refused because the later eight-unit sale would fail. | [Preview](replacement-delta-native/wails-position-adjustment-preview.jpg) · [Later sale refusal](replacement-delta-native/wails-position-adjustment-refused.jpg) | [Preview](replacement-delta-native/gio-position-adjustment-preview.jpg) · [Later sale refusal](replacement-delta-native/gio-position-adjustment-refused.jpg) |
| PRBUY buy → Debt draw | Draw 125.125 EUR from Sample Replacement Loan into Sample Replacement Bank. The bank reaches 1125.125 EUR and the loan has 525.125 EUR outstanding. | [Preview](replacement-delta-native/wails-debt-draw-preview.jpg) | [Preview](replacement-delta-native/gio-debt-draw-preview.jpg) |
| PRSELL sell → Debt payment | Pay 85.25 EUR principal and 2.125 EUR interest or fee from Sample Replacement Bank. The bank reaches 1037.75 EUR and the loan has 439.875 EUR outstanding. | [Preview](replacement-delta-native/wails-debt-payment-preview.jpg) | [Preview](replacement-delta-native/gio-debt-payment-preview.jpg) |
| PRDIV dividend → Value update | Set the absolute value of Sample Replacement Bank to 1234.5678 EUR with reason Reconciliation. The loan remains 439.875 EUR outstanding. | [Preview](replacement-delta-native/wails-value-update-preview.jpg) | [Preview](replacement-delta-native/gio-value-update-preview.jpg) |

The Wails quantity-adjustment form opened with **Added** selected. In the initial Gio build it opened on **Remove**, so the accepted Gio run selected **Add** explicitly. [Original Gio default](replacement-delta-native/gio-position-adjustment-default-before.jpg).

Gio now starts a new quantity adjustment on **Add units**. Reselecting the active type retains a manually selected direction. The signed Gio build from `5d3166c4eb25f7e264505b4bf9ac3617c63f68a0` opened on Add and confirmed the same 1.25-unit adjustment on a fresh copy without changing direction. The ledger readback matched the Wails adjustment. After saving, returning to the original and pressing Escape restored its row actions focus. [Fixed default](replacement-delta-native/gio-position-adjustment-default-final.jpg) · [Preview](replacement-delta-native/gio-position-adjustment-final-preview.jpg) · [Posted ledger](replacement-delta-native/gio-position-adjustment-final-posted.jpg).

The focused Gio desktop History Investment and Archived Sale tests and the history and i18n packages passed on macOS at this code commit. The new regressions cover choosing and keeping an adjustment direction, matching exact Wails projections for positions, debt and valuation, and refusing to invalidate the later sale or transfer into an account without a matching holding. The one production change initializes the direction on Gio's correction form.

## Persisted ledger readback

Independent read-only SQLite comparisons of the synthetic profiles matched all eight replacements. Each correction group contained exactly one reversal, one replacement and one replacement mutation receipt. The reversal inverted every original effect direction. Each replacement matched Wails by endpoint, effect role, classification, direction, currency and exact amount or quantity. The original activity, reversal and replacement shared the same effective instant. Both PRBUY cases retained `2026-11-25T02:00:00.000Z`, both PRSELL cases `2026-11-26T03:00:00.000Z`, both PRDIV cases `2026-11-26T04:00:00.000Z` and both PRPOS cases `2026-11-26T01:00:17.123Z`. The three refused native inputs added zero correction groups.

| Profile | Exact closing state after replay |
| --- | --- |
| Cash | Sample Buy Desk 982.6245 EUR and 0 PRBUY. Sample Dependent Desk 1006.625 EUR and 3 PRDEP after its later trade. Sample Sell Desk 786.5433 EUR and 20 PRSELL. Sample Dividend Desk 910.125 EUR, 17.531 USD and 10 PRDIV. The stored FX rate was 0.820130248834. |
| Investment transfer | Sample Position Source 1.5 PRPOS and 976 EUR. Sample Position Destination 3.5 PRPOS and 189 EUR. |
| Investment quantity adjustment | Sample Position Source 5.25 PRPOS and 976 EUR. Sample Position Destination 1 PRPOS and 189 EUR. The replacement preserved the 10.125 EUR unit cost. |
| Debt and value | Sample Replacement Loan 439.875 EUR outstanding and Sample Replacement Bank 1234.5678 EUR. The last value update recorded a 196.8178 EUR remeasurement. |

The two PRPOS profiles kept the eight-unit sale for 96 EUR at `2026-11-27T01:00:00.000Z`. Gio offers the original note in its correction draft and saved it in these runs. Wails saved the replacement note blank when the native form was left blank. The Wails bank value update required choosing **Reconciliation** from its reason picker. Gio initially offered **Reconciliation**, and both saved that reason.

## Archived holding boundary

A separate service-backed readback probe at `8d36a278d7d5e58721084e21828bdaa9822df624` reopened a copy of the prior Wails native quantity-edit ledger. That edit left 0.25 PRSELL units archived. A 40 EUR quote valued the residual at 10 EUR. The archived position was excluded from the account, Overview and Portfolio valuation. Sample Sell Desk showed cash only at 1044.9167 EUR, Overview showed 3951.6662 EUR, and the Portfolio securities subtotal was unavailable. [Earlier native quantity probe](replacement-followup.md).

An explicit sell against the archived holding was refused. Restoring the position on the copy with no active duplicate exposed 0.25 units and 10 EUR in the account and Portfolio and brought Overview to 3961.6662 EUR. A subsequent explicit Wails sale reduced the restored holding to zero. On an independent copy, a Wails Buy without a holding ID opened a separate active one-unit PRSELL holding. The original 0.25 units stayed archived, and restoration then failed because an active holding already used the same instrument.

The pristine archived-zero profile still accepted a historical Wails gross-only correction from 155 EUR to 158 EUR at quantity 15.5. It retained zero units, the archive flag and the original effective instant `2026-11-27T01:00:00.000Z`. Gio's correction keeps the archived trade's side, endpoints and quantity fixed. This allows monetary corrections while protecting against residual units hidden from valuation or blocked from restoration by an active duplicate.

## Capture format

The linked JPEGs are the original native windows. Gio captured 2560×1600 Retina pixels and Wails captured 2558×1600 pixels including its title bar.
