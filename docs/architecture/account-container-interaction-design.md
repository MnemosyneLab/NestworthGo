# Account Container Interaction Contract

## 1. Document status and authority

- Status: Implemented / current contract
- Companion domain contract: [account-container-and-position-model-design.md](account-container-and-position-model-design.md)
- Baseline: Nestworth-go `0.2.1` / schema v8 / current Wails frontend
- Data policy: this is a breaking cutover for an unreleased version. Only a
  fresh schema v8 database is supported; there is no legacy-data, old
  interaction, or old-page compatibility layer.

This document freezes how the Account container model is created, viewed, and
operated in the desktop app. Legal combinations, classification, inclusion,
and tracking immutability still come from the domain contract. This document
does not redefine the domain model.

Product principle:

> At create, let the user describe "what real-world account this is and how
> detailed the record should be". The detail page shows "what is inside this
> account". Overview then re-aggregates by underlying asset.

Asset Type and Account Type are two views. Do not mix them. Do not introduce
SubAccount. The user always sees one real-world Account and the cash and
holdings inside it.

## 2. Final decision summary

1. Account is the primary navigation object. Clicking an Accounts-list row
   opens Account detail, not metadata edit.
2. Create chooses Institution first, then the real-world account type. Ordinary
   users do not pick `balance_sheet_role` or internal tracking enums directly.
3. Bank accounts and digital wallets ask whether to record the account total
   only or cash and holdings separately. Brokerages, investment accounts, and
   crypto exchanges default to cash and holdings.
4. Accounts with `tracking_mode=holdings` all show Cash + Investments. A mixed
   bank account and a brokerage use the same detail structure.
5. Accounts with `tracking_mode=balance` or `manual_value` use a light Simple
   detail that shows only the current balance or valuation.
6. Account detail must provide a real Buy / Sell primary path. "Record
   existing position" is a separate secondary action. It must not pose as a
   buy and must not reduce cash.
7. A Composite Account must record cash by currency. The Account default
   currency is the default input and display context, not the only allowed
   cash currency.
8. Before History has started, opening cash, opening holdings, and opening
   valuations may be entered. Trade-like actions first start History
   explicitly, then return to the original action.
9. Overview defaults to underlying Asset Type, with By institution and By
   account type as additional views. Titles must name the aggregation
   dimension.
10. Portfolio is a separate page. It contains only whole-account
    `include_in_portfolio=true` asset accounts. Cash and holdings of a
    Composite Account enter together.
11. Amounts, market values, completeness, and classification come from backend
    read models. The frontend only composes and displays them; it does not
    recompute financial authority.
12. A household-wide holdings table may remain as an "all holdings index", but
    it no longer owns Account detail or Portfolio responsibilities.

## 3. Current implementation and boundaries

The current `0.2.1` implementation already closes the Account-container
interaction loop:

- `AccountsPage` groups by Institution. Clicking an account opens detail in the
  same workspace. Create uses an Institution-first wizard.
- The create wizard reads legal combinations from Catalog and asks in
  real-world account-type and recording-method language. Role, Tracking, and
  inclusion come from backend Catalog results. At least one owner must be
  selected.
- `AccountDetail` shows cash by currency and investments by Instrument for
  Holdings Accounts, and a single current value for Balance / Manual Value
  Accounts. Amounts, completeness, and missing reasons come from backend
  valuation DTOs.
- Detail provides opening cash, cash reconcile, deposit/withdraw, FX
  conversion, transfer, buy, sell, record existing position, and Simple value
  update. Actions that need History first show Start History.
- Overview provides component-grained `assetsByType` / `liabilitiesByType`
  and keeps account-level `byAccountType`, `byInstitution`, and `byGroup`.
  Portfolio is a separate navigation page and works with whole-account
  inclusion.
- Archived accounts can be viewed read-only. Account settings, archive, and
  restore live on the detail page, not as implicit row-edit actions.

Standing boundaries:

- Simple Account (`balance` / `manual_value`) values must use the Account
  default currency. Cash components of a Holdings Account may use other
  system-supported currencies.
- `tracking_mode` is immutable after create. This version does not support
  tracking transition or component-level inclusion.
- Holdings Account Portfolio inclusion remains whole-account: cash and all
  holdings enter or are excluded together.
- Historical Simple Account bucket names come from current metadata and are
  marked `current-metadata-derived`.

## 4. Goals and non-goals

### 4.1 Goals

- Users can create bank, brokerage, digital-wallet, property, credit-card, and
  similar accounts using real-world account mental models.
- Users can see multi-currency cash, holding quantities, and market values on
  Account detail.
- Users can record cash in a mixed bank account and buy funds,
  wealth-management products, gold, and similar instruments there.
- Users can distinguish opening positions, balance reconciliation, deposits and
  withdrawals, trades, and FX conversion.
- Overview and Portfolio aggregation dimensions are clear and match domain
  classification.
- Every immutable rule is explained at create. Edit screens do not offer
  controls that are guaranteed to fail.

### 4.2 Explicit non-goals

- No schema v6 migration, legacy-data conversion, or old UI compatibility.
- Do not introduce SubAccount.
- Do not support tracking-mode conversion after create.
- Do not support component-level inclusion; the three inclusion switches stay
  whole-account.
- Do not add Activity kinds.
- Do not guess or force tracking from Institution name.
- Do not expand cash rows into a demand/time-deposit sub-account model.

## 5. Product language

The primary path uses the product language on the left. Writes still use the
domain values on the right. Catalog legal combinations are the only option
source for create and edit.

| User sees | Domain value |
| --- | --- |
| Record the account total only | `tracking_mode=balance` |
| Record cash and holdings separately | `tracking_mode=holdings` |
| Record a single estimated value | `tracking_mode=manual_value` |
| Asset / Liability (explicit only when creating Other) | `balance_sheet_role` |
| Include in net worth / portfolio / liquid assets | The three Account inclusion switches |

The primary path must not show: `holdings`, `balance`, `manual_value`,
`tracking mode`, `balance sheet role`, SubAccount, or Investment-only.

Settings may use more formal wording, but still show product names:

| Setting | Example |
| --- | --- |
| Account type | Bank account, Brokerage, Credit card |
| Tracking method | Detailed positions / Account total / Manual value |
| Immutable hint | This cannot currently be changed after account creation |

## 6. Information architecture and navigation

```text
Overview
Accounts          Real-world account list and detail
Portfolio         Asset accounts included in the portfolio
History           Cash and position changes
Instruments       Household-level instrument catalog
Market data
Settings
```

- Accounts is the primary entry for "where the money is and what is inside the
  account".
- Portfolio is the investment view within `include_in_portfolio`.
- Instruments is a reusable instrument catalog. Presence there does not mean
  any account already holds the instrument.
- If a household-wide holdings table remains, name it "all holdings index". It
  is a cross-account lookup tool, not a substitute for Portfolio or Account.

## 7. Creating an Account

Create uses a stepped wizard instead of exposing the three domain fields on
one form. After success, a Composite Account opens empty Cash + Investments
detail; a Simple Account opens balance/valuation detail.

### 7.1 Flow

```text
1. Where is it held?              Institution (optional; can be created here)
2. What kind of account is it?    Account type
3. How would you like to track it? Shown only when two common recording methods exist
4. Account details                Name, default currency, owners, inclusion
5. Review and create
```

Immutable choices must be echoed in natural language on Review, for example:

```text
China Merchants Bank · Bank account
Record cash and holdings separately
After creation this cannot currently be changed to an account total
```

### 7.2 Institution

- Step 1 lists existing Institutions and offers "No matching institution" and
  "Add an institution".
- Institution stays optional. Cash, property, vehicles, and collectibles often
  have no custodian. Banks, brokerages, and credit cards should be prompted to
  choose one, but it is not required.
- Institution does not decide the triple. The same institution may hold a
  bank account, a credit card, and an investment account.

### 7.3 Account type, Role, and recording method

Primary list:

```text
Bank account
Brokerage
Investment account
Crypto exchange
Digital wallet
Property
Vehicle
Credit card
Loan
Other
```

"More account types" keeps equally legal but less common Catalog types: Cash
on hand, Pension, Insurance policy, Collectible, Receivable. The frontend must
not hard-code a reduced enum. The display set and legal combinations come from
Catalog.

Except for `other`, Role is written automatically by
`DefaultBalanceSheetRole(type)`. `other` must ask "Is this an asset or a
liability?" and then show the matching legal recording methods.

| `account_type` | Primary-path recording | Options at create |
| --- | --- | --- |
| `cash_on_hand` | Record the account total only | None |
| `bank_account` | Ask; default to account total only | Account total / cash and holdings separately |
| `digital_wallet` | Same as bank account | Account total / cash and holdings separately |
| `brokerage` | Record cash and holdings separately | Advanced: record a single market value |
| `investment_account` | Record cash and holdings separately | Advanced: record a single market value |
| `crypto_exchange` | Record cash and holdings separately | None |
| `pension` | Record cash and holdings separately | Advanced: record a single total |
| `insurance_policy` / `property` / `vehicle` / `collectible` | Record a single estimated value | None |
| `receivable` | Record the account total only | Advanced: record by valuation |
| `credit_card` / `loan` | Record the account total only | None |
| `other` + asset | Must choose | Account total / cash and holdings / a single estimated value |
| `other` + liability | Record the account total only | None |

Bank and digital-wallet question:

```text
How should this account be recorded?

○ Record the account total only
  Best for ordinary deposits or a single balance.

○ Record cash, funds, wealth-management products, gold, and similar holdings separately
  Best for mixed accounts. This cannot currently be changed to an account total after creation.
```

Brokerages, investment accounts, and crypto exchanges enter detailed
recording directly. Users are not required to understand tracking.

### 7.4 Details and Inclusion

Primary fields: name, default currency, owners. Institution is echoed and can
be changed by going back. Group, icon, and the three include switches
live under "More settings".

Owners must be selected explicitly, at least one. Nobody is preselected.
Continue / Add account stay disabled until then. The create submit path must
also reject empty ownership; disabling the button is not enough. Do not default
an empty list to "all household members, equal shares". After several owners
are selected, leaving shares blank may still split equally among the selected
people, or the user may enter explicit percentages such as 70/30 that must sum
to 100%. An empty owner set disables the primary button. It is not a §14
inline error and not an unresponsive Continue.

Inclusion defaults use `SuggestedInclusion`. Checking Portfolio for a
Composite Account must show the whole-account explanation:

```text
Include in portfolio

Every cash balance and holding in this account will enter the portfolio,
not only funds or other investments.
```

A Simple Account may enter an initial balance or valuation during create. A
Composite Account does not enter a fictional total; after create it opens
detail to add cash or opening holdings.

## 8. Accounts list

The Accounts page is the real-world account entry, not a metadata-edit table.

- Default grouping is by Institution. Accounts with no institution go under
  "No institution".
- Each row shows name, Account type product name, household-base total, and
  data-completeness status.
- The Account type chip means account type only. It must not impersonate
  underlying asset types such as Cash / Stock.
- Clicking opens detail. Edit, archive, and icon actions live in Account
  settings on the detail page.
- Archived Accounts are omitted from the active list by default. When an
  archived item is opened, detail is read-only until it is restored.
- Incomplete totals show "Partial valuation" and the missing-pricing reason.
  Missing amounts are not treated as zero.

## 9. Account detail

### 9.1 Shared page header

```text
MooMoo SG Brokerage                         128,420 CNY

MooMoo SG · Brokerage
Included in net worth · Included in portfolio
```

The title total uses the backend Account valuation. The header also holds:

- `asOf` time;
- complete / partial valuation status;
- archived read-only status;
- Account settings entry;
- whole-account inclusion hint.

### 9.2 Composite: Cash + Investments

Every Account with `tracking_mode=holdings` uses the same layout:

```text
Cash
------------------------------------------------
SGD                              12,000 SGD
USD                               8,500 USD
CNY                               3,000 CNY

[Add cash balance]  [Deposit or withdraw]  [Convert]

Investments
------------------------------------------------
NVDA     Stock          20       3,648 USD
QQQ      ETF            15       ...
CMB WM A Bank product   1       200,000 CNY

[Buy investment]  [Record existing position]
```

A CMB mixed account and a brokerage use the same structure. Differences come
only from internal cash and Instrument types, not from a second Account page.

Cash rows aggregate by currency. Holding rows at least show Instrument, type,
quantity, current market value, and valuation status. If the current quote is
missing, show quantity and "Missing current price". Do not show a derived
zero market value.

Empty states must include the next action:

```text
Cash          No cash balances yet       Add cash balance
Investments   No holdings yet             Buy investment
```

### 9.3 Cash-action semantics

These actions must not all be called "Add cash":

| User action | Meaning | Existing command / write |
| --- | --- | --- |
| Add cash balance / Reconcile balance | Calibrate the current balance of one currency to a resulting value | `AppendAccountCashValue` |
| Deposit | External funds enter the account; the user enters a change amount | `MoneyAdded` |
| Withdraw | Funds leave the account; the user enters a change amount | `MoneyRemoved` |
| Convert currency | FX conversion in the same account or between accounts | `FXConversion` |
| Transfer | Move cash between two accounts | `CashTransfer` |

The "Add cash balance" form chooses currency first, then the resulting
balance in that currency. A Composite Account may choose any supported
currency and defaults to the Account default currency. The confirm page must
explain that after History has started the difference is recorded as a
reconciliation.

### 9.4 Buy / Sell versus "Record existing position"

These two operations must stay separate:

- **Buy investment / Sell** is a trade. It changes cash and holdings through the
  existing `ChangeTrade`.
- **Record existing position** records an opening quantity or corrects the
  current holding quantity and does not reduce cash. It uses `CreateHolding`
  / Position Adjustment.

"Record existing position" is a secondary or advanced entry. The copy is
explicit:

```text
Record a quantity already in this account.
This is not a buy and does not reduce cash.
```

The primary path does not create zero-quantity placeholder Holdings:

- Record existing position requires a quantity greater than zero;
- when the user first Buys an Instrument the account does not yet hold, the
  application layer creates the Holding and completes the Trade in the same
  transaction;
- an Instrument may be chosen from the Household catalog or created in the
  sheet with the minimum required information; an Instrument is not a
  SubAccount.

The complete loop for "a bank account can buy a fund" is: create a mixed bank
account → record cash in the matching currency → Buy investment → choose or
create the fund → enter quantity and trade details → complete the trade and
refresh cash, holdings, and valuation.

### 9.5 History has not started

While History has not started, detail may record opening state:

- opening cash balances;
- existing holdings and quantities;
- Simple Account initial balance or valuation.

Event-like actions such as Deposit, Withdraw, Buy, Sell, Convert, and
Transfer need History. When the user triggers them:

1. Explain why History must start and which start date will be used;
2. Open the existing Start History flow;
3. After success, return to the original Account and original action, keeping
   safe already-filled fields;
4. Cancel writes nothing and does not silently start History.

Do not send the user out of the current task with "please go to Settings
first".

### 9.6 Simple detail

`balance` and `manual_value` do not show Cash / Investments:

```text
Primary home                                  5,000,000 CNY

Property
Last recorded value 5,000,000 CNY · 2026-08-27

[Update value]
```

```text
CMB credit card                                8,420 CNY

Credit card · Liability
Current balance 8,420 CNY

[Update balance]
```

Liabilities are shown as absolute amounts, with "Liability" explaining the net
worth direction. Do not introduce a minus sign in the detail title unless the
whole app adopts a consistent sign rule.

- History not started: write the initial Account value.
- History started: balance accounts use the existing Balance Adjustment;
  valuation accounts use Manual Valuation / Value Update.

### 9.7 Account settings

In settings:

- Editable: name, Account type (only still-legal compatible values),
  Institution, group, owners, the three inclusion switches, and icon;
- Read-only: Tracking method, with "This cannot currently be changed after
  account creation";
- Read-only: Role; Role of `other` also cannot change after create;
- Account type updates do not recompute inclusion, create Activities, or change
  amounts;
- Holdings Accounts always show the whole-account hint next to Portfolio
  inclusion.
- Owners use the same gate as the create wizard: at least one owner must be
  selected. Save is disabled until then, and the update submit path also
  rejects empty ownership. Do not default an empty list to all household
  members. After several owners are selected, blank shares may still split
  equally among the selected people, or explicit shares may be entered.

## 10. Overview

Overview still uses net worth, assets, and liabilities as top-level totals.
The default primary asset chart is by underlying asset:

```text
Asset allocation          assetsByType, component grain
Cash
Stocks
ETFs
Mutual funds
Bank products
Crypto
Precious metals
Property
...

By institution            byInstitution, Account grain
China Merchants Bank
MooMoo SG
DBS
No institution

By account type           byAccountType, Account grain
Bank account
Brokerage
Property
...
```

Liabilities stay in `liabilitiesByType` and are not merged into Asset
allocation.

The UI must explain the dimension difference:

> Asset allocation groups cash and holdings by their underlying type. By
> account type groups each real-world account as a whole. A mixed bank
> account appears as a bank account and also in several underlying-asset rows.

`byAccountType` must be added to `OverviewResult` by the backend. The frontend
must not recompute it from the Account list. Rules:

- Aggregate only Accounts with `include_in_net_worth=true` and role=asset;
- Place the Account's current valued subtotal into `account_type` as a whole;
  do not split Composite Accounts;
- Use the same as-of, conversion, and incomplete semantics as top-level
  Overview assets;
- Missing valuations are not treated as zero;
- Percentage denominators use the asset valued subtotal from the same result.

By member / by group may remain as secondary blocks, but they must not compete
with Asset allocation for the primary position.

## 11. Portfolio

Portfolio is a separate page that uses the existing
`PortfolioService.Portfolio` and shows only Accounts with
`include_in_portfolio=true` and role=asset.

```text
Portfolio                              320,000 CNY

Allocation                             byInstrumentType
Cash            12%
Stocks          35%
ETFs            40%
Crypto           5%
...

Accounts
MooMoo SG Brokerage                   180,000 CNY
IBKR                                  140,000 CNY
```

- Clicking an Account opens the same Account detail.
- When a Composite Account is included, cash and all holdings appear. The page
  and settings repeat the whole-account explanation.
- Excluded Accounts are not shown and do not enter the denominator.
- Missing quotes or FX rates keep backend incomplete / excluded-amount
  semantics and are not treated as zero.

## 12. Action mapping to existing domain commands

| Detail entry | Domain intent | Constraint |
| --- | --- | --- |
| Add cash balance | `AppendAccountCashValue` | Composite: any supported currency; enter the resulting balance |
| Deposit / Withdraw | `MoneyAdded` / `MoneyRemoved` | History has started; enter a change amount |
| Convert | `FXConversion` | History has started |
| Transfer | `CashTransfer` | History has started |
| Buy / Sell | `ChangeTrade` | History has started; first Buy atomically creates the Holding |
| Record existing position | `CreateHolding` / Position Adjustment | Does not move cash; quantity > 0 |
| Update Simple value | Account Value / Balance Adjustment / Manual Valuation | Uses the Account default currency |

Do not add Activity kinds to support these screens. Detail only pre-fills
Account context and chooses the correct command. It does not copy a different
History semantics.

## 13. Read-model and API seams

### 13.1 Existing capabilities the frontend can compose

| Surface | Authority | Frontend responsibility |
| --- | --- | --- |
| Create wizard | Catalog `accountCombinations` + `CreateAccount` | Show real-world language and submit a legal combination |
| Accounts list | `ListAccounts` + Institution + Account valuations | Grouping and navigation |
| Detail total / completeness | `AccountValuation(s)` | Display directly |
| Current cash | cash components in the valuation | Display by currency |
| Holding quantity | `HoldingsByAccounts` | Align with valuation components by `holdingId` |
| Instrument metadata | Instrument list/detail | Show name, type, quote currency |
| Current price | current quote read model | Show when present; show missing otherwise |
| Portfolio | `PortfolioService.Portfolio` | Page layout and navigation |

`ListAccountCashValues` is observation history. The frontend must not pick
one "latest record" as the current-cash authority. Current state uses cash
components at the same as-of as Account valuation, so history ordering,
currency gaps, and refresh timing do not create two sources of truth.

### 13.2 Current backend facts

- Composite cash multi-currency validation is shared by the application and
  domain layers and covers write paths before and after History starts.
- `OverviewResult.ByAccountType` is produced by the backend and shares
  conversion, as-of, and incomplete rules with the rest of Overview.
- Account detail composes existing Account, Holding, Instrument, and valuation
  read models. It does not add a separate financial write model.

### 13.3 Frontend prohibitions

- Do not multiply quantity by quote to produce an authoritative market value;
- Do not convert FX or sum net worth locally;
- Do not treat missing quotes, FX rates, or components as zero;
- Do not infer a current balance from append-only observation history;
- Do not copy `IsValidAccountCombination`, classification, or
  SuggestedInclusion rules;
- Do not recompute inclusion after an Account type update;
- Do not use floating point for money.

Unit price shows only the current backend quote. When there is no reliable
quote, omit the column or show "Missing current price". Do not reverse a
display value from market value and quantity.

## 14. State, errors, and accessibility

Every new page and sheet must cover:

- loading skeleton;
- empty states such as no cash, no holdings, and no Portfolio Account;
- explanations for partial valuation, missing quote, and missing FX;
- inline API validation errors that preserve user input;
- a recoverable flow when History has not started;
- archived Account read-only state;
- after a successful write, invalidate only the related Account, Overview,
  Portfolio, and History queries;
- duplicate-submit protection and a clear in-progress state.

Interaction and accessibility:

- Every action is keyboard-complete. Opening a sheet focuses the title or
  first field; closing returns focus to the trigger;
- Do not use color alone for asset/liability, included/excluded, or
  complete/incomplete;
- Tabs, radios, and menus use correct semantics and `aria-selected` /
  `aria-checked`;
- Amounts show a currency code or an unambiguous symbol;
- Destructive actions such as delete and archive do not sit beside Buy /
  Update value with the same visual weight.

## 15. Behavior matrix

### 15.1 Create and immutability

- Bank accounts ask for a recording method by default and default to account
  total only. Choosing detailed recording creates a legal holdings combination.
- Brokerages do not ask using internal tracking terms and default to Cash +
  Investments.
- Credit cards are created as liabilities automatically. Other chooses asset or
  liability explicitly.
- The primary path does not show internal enums or SubAccount.
- Edit cannot change Role / Tracking. Compatible type updates create no
  Activity, change no amounts, and recompute no inclusion. Illegal type updates
  are rejected.
- Continue / Add account / Save stay disabled with no owners selected. Create
  and update submit paths reject empty ownership. Do not default an empty
  owner list to equal shares among all household members. Equal split among
  already-selected owners with blank shares remains allowed.

### 15.2 Composite Account

- A MooMoo Account with default currency SGD can record SGD, USD, and CNY
  cash at the same time.
- MooMoo and a CMB mixed account use the same detail structure and can show
  holdings of different Instrument types.
- Current cash comes from valuation cash components. Holding quantities and
  valuations stay aligned.
- Missing quote / FX shows partial valuation and the reason. Amounts are not
  treated as zero.
- Record existing position does not reduce cash and does not allow a
  zero-quantity placeholder.
- The first Buy of an unheld fund creates the Holding automatically. Cash and
  quantity update after the same successful operation.

### 15.3 History boundary

- Before History starts, opening cash, existing holdings, and Simple initial
  values may be recorded.
- Buy / Deposit and similar actions from detail enter Start History and return
  to the original action after success. Cancel writes nothing.
- After History has started, cash reconciliation, Trade, FX, and Transfer
  produce existing Activity kinds. No new kinds are added.

### 15.4 Overview and Portfolio

- A CMB mixed account appears as a whole under By account type "Bank account".
  Its cash / fund / gold components also enter the matching Asset allocation
  rows.
- `byAccountType` uses the same as-of, conversion, and incomplete semantics as
  top-level Overview.
- Portfolio omits unchecked Accounts. Checking a Composite Account includes
  cash and holdings as a whole and shows the explanation.
- The all-holdings index and Portfolio keep different names and purposes so
  users do not treat them as the same view.

### 15.5 End-to-end critical journey

```text
Create a China Merchants Bank mixed account
→ Choose "Record cash, funds, wealth-management products, gold, and similar holdings separately"
→ Add a CNY cash balance
→ Click Buy investment
→ If needed, complete Start History and return
→ Choose or create a fund and complete the first buy
→ Account detail shows the reduced cash and the fund holding together
→ Overview classifies by Cash / Mutual fund
→ If the whole account is included, Portfolio contains that account's cash and fund
```

## 16. Explicitly deferred items

These can be designed later and do not block this contract:

- tracking-mode conversion;
- component-level inclusion;
- user aliases or deposit-product subtypes for cash components;
- a fuller trade model with tax lots, order status, and fee splits;
- permanent Account-detail URLs / multi-window routing; the first version may
  switch full-width inside the Accounts workspace;
- a more complex Account-detail aggregation DTO unless existing read models
  cannot keep a consistent as-of.
