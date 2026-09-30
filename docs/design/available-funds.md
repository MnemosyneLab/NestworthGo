# Available Funds, Term Deposits, and Locked Products

- **Status:** Implemented
- **Owner:** Nestworth product and application maintainers
- **Data source:** Current local assets, saved access rules, reservations, valuations, and FX
- **Validation boundary:** Automated tests exist for application, persistence, DTO, and UI behavior. Native macOS acceptance is a separate release gate.

## Product contract

Available Funds estimates what may be accessible from assets the household
already owns by a selected date. It supports Today, 7-day, 30-day, and a custom
future date. It is not a household budget, a future-income forecast, or a
recommendation that an amount is safe to spend.

The calculation uses current local asset values, current FX inputs, source
access policies, product terms, and reservations. It does not fetch data from a
provider, predict future prices or exchange rates, or create cash entries for
future events. Every result reports whether it is complete, partial, or
unavailable. Missing values remain unknown; they are never replaced with zero
to make a total look complete.

## Availability rules

- Each source has a normal access route and, when explicitly allowed, an early
  route. The estimate chooses one route for each source; alternatives are not
  added together.
- User-confirmed rules control access timing, settlement lag, caps, and fees.
  Assumed defaults are disclosed. Weekday estimates skip weekends only and do
  not include holidays.
- Reservations are recorded for a specific source and currency. They reduce
  that source only when its selected route is available within the chosen
  horizon. A reservation never silently reduces unrelated cash or another
  asset.
- Current FX may translate known native amounts into the household currency.
  Missing FX leaves native amounts visible and makes the base-currency result
  partial. Future FX conversion costs and quotes are not forecast.
- Due but unconfirmed product receipts are not spendable cash. The user must
  record money received before the account cash changes.

## Managed products

A term deposit or locked product is held inside an eligible Holdings Account.
The app can open a contract using tracked cash, record a product that was
already owned, and update a locked product's explicitly confirmed value.
Recording an existing product does not deduct cash or invent a purchase: the
user must account for whether the shown account cash already includes it.

Term deposits use principal as current carrying value. Simple interest is an
estimate until it is received; the calculation uses the selected 365- or
360-day basis, or an explicitly entered maturity amount. Locked products use
their latest recorded total valuation. Access, unlock and maturity dates,
fees, early-withdrawal rules and known assumptions are shown with the estimate.

Opening, recording, receiving, early redemption, interest receipt and renewal
are explicit operations. Previews show the resulting cash, product value,
fees and net-worth effect before confirmation. Receipt requires actual
amounts; a submitted redemption request is not cash. Renewal closes the old
contract and opens the new one in a single transaction. Eligible grouped
operations can be undone with revision and dependency checks; stale or unsafe
undo attempts fail without silently changing later activity.

## Limits

The current model does not support partial subscriptions or redemptions,
multiple lots inside one managed contract, automatic reinvestment, variable
rates, compounding, tax calculations, holiday calendars, jurisdiction-specific
settlement promises, future wages or expenses, or historical as-of liquidity
reports. Product due dates do not trigger background financial operations.
Bank transactions, live product feeds, OCR/import, and OS notifications are
outside this feature.

## Implementation references

- Domain rules and product lifecycle: internal/domain and internal/application
- SQLite acceptance and history: internal/infrastructure/sqlite
- Wails and frontend contracts: internal/wailsapi/liquidity and
  frontend/src/features/liquidity
- Deterministic behavior tests: internal/domain/liquidity*_test.go,
  internal/application/*product*_test.go,
  frontend/src/features/liquidity/*test.tsx
- The broader application persistence and IPC rules are in the
  [Data and Application Contracts](../architecture/data-and-ipc-contracts.md).
