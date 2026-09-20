# Feature Brainstorm — 2026-09-20

- **Owner:** Walt / Nestworth product
- **Status:** Ideas for discussion; no implementation or release commitment
- **Scope:** New uses for a personal, local-first household finance desktop app
- **Validation boundary:** Captured from a brainstorming conversation. Feasibility, overlap with existing features, and effort have not been assessed.

## Starting point

Explore what would make Nestworth useful in everyday life, independently of
the existing roadmap. With its owner as the primary user, the app can support
specific personal workflows instead of optimizing for a broad audience.

The central opportunity is to use account, holding, and activity data to
answer questions about life choices and next actions.

## 1. “What if I…” scenario sandbox

**Question:** What happens to my finances if I buy a home, stop working for
six months, or sell a holding?

Create an independent scenario from the current portfolio, add hypothetical
changes, and compare its cash, debt, allocation, and future balances with
the actual position.

**Small first version:** One-time expenses, recurring income and expenses,
and asset transfers. Market forecasting is unnecessary for the initial
version. Keep assumptions visible and simulated changes separate from the
actual ledger.

## 2. Give money a purpose

**Question:** Which money is already committed to an emergency fund, home
purchase, travel, or long-term investing?

Allocate existing assets to purposes across accounts and show progress
toward each goal. An account balance alone does not explain whether its
money is available for something else.

**Small first version:** Manual allocations, target amounts, and an
unallocated balance. Prevent the same money from being counted toward
multiple goals simultaneously.

## 3. “How much can I actually use?”

**Question:** How much money could I access today, within a week, or within
a month, after allowing for commitments?

Show liquidity alongside net worth. Cash, listed investments, property,
locked products, and retirement accounts have different access constraints.
Combine availability with money assigned to goals or upcoming expenses.

**Small first version:** User-defined availability categories and committed
amounts. Distinguish estimated proceeds from cash already available.

## 4. Account screenshot or statement → update draft

**Question:** Can I update Nestworth without manually re-entering every
balance and holding?

Drop in a bank or broker screenshot or statement. Extract balances and
positions, match them with existing records, and show a reviewable diff:

- AAPL: 12 → 15 shares.
- USD cash: 18,072.50 → 17,430.20.
- A previously unrecorded holding was found.

Apply changes only after review. A balance difference does not establish
whether a change was spending, investment return, or something else;
unexplained changes must remain unresolved until clarified.

**Small first version:** One frequently used broker statement format, a
comparison screen, and explicit confirmation of updates. This idea has the
clearest potential to reduce routine maintenance effort.

## 5. Month-end reconciliation assistant

**Question:** Does the ledger agree with the actual accounts, and what
explains any difference?

Compare entered statement balances and holdings with Nestworth. Help the
user investigate missing trades, fees, dividends, splits, or valuation-date
differences. Record the date through which an account has been reconciled.

**Small first version:** Enter a statement date and balances, inspect
differences, and explicitly mark an account reconciled. This complements
checks for data completeness by checking agreement with the real account.

## 6. Life-event ledger

**Question:** What did moving home, renovating, changing jobs, having a
child, or taking a long trip actually cost?

Associate financial activities across accounts with a life event. Later,
review its total cost, outstanding deposits or reimbursements, and whether
it required drawing on long-term investments.

**Small first version:** Event labels attached to selected activities and
an event summary. The workflow should work without detailed daily expense
tracking.

## 7. Personal investment decision journal

**Question:** Why did I buy this, and does that reasoning still hold?

Record the rationale, intended holding period, and conditions for
reconsideration alongside a position. A later review can compare the
original reasoning with what happened.

**Small first version:** Dated notes linked to holdings or trades, with a
user-selected review date. The purpose is to preserve decision context and
support learning from personal history.

## 8. Personal allocation rules

**Question:** Is my portfolio still within the limits I chose for myself?

Let the user define a maximum concentration in one asset, a cash reserve
range, or target allocations. Show deviations and simulate how hypothetical
adjustments would change the result.

**Small first version:** User-defined rules, deviation indicators, and
simulated adjustment amounts. Trade execution is outside the initial scope.

## 9. Private financial calendar

**Question:** What money needs to arrive or leave soon, and which account
will cover it?

Bring together deposit maturities, debt payments, insurance renewals,
expected income, and large planned expenses. Link events to accounts to
show whether current balances cover known upcoming outflows.

**Small first version:** Manually entered events and a 30-day account-level
cash outlook. Keep expected events clearly distinct from posted activity.

## 10. One-page monthly review

**Question:** What changed this month, what needs attention, and what is
coming next?

Produce a saveable monthly summary covering:

- Net worth changes, separating money contributed from investment changes.
- Important life events and financial activities.
- Reconciled accounts and remaining data gaps.
- Known large income and expenses in the following month.

**Small first version:** A deterministic summary based on existing data,
with optional personal notes. AI-generated prose could be added later, but
is not necessary for the initial feature.

## Suggested starting points

These are discussion preferences, not an approved priority order.

| Direction | Why it stands out | Small initial scope |
| --- | --- | --- |
| Screenshot / statement update drafts | Reduces the effort of maintaining useful records | One broker format and a reviewable diff |
| Scenario sandbox | Gives the desktop app a new use in personal decision-making | Copy the current position, simulate changes, compare outcomes |
| Money purposes + available balance | Connects asset records with concrete life plans | Manual allocations and available / unallocated totals |

The scenario sandbox is the strongest exploratory choice. If maintaining
records is the main day-to-day friction, start with import drafts instead.
The next product decision should follow that actual pain point rather than
the desire to add another chart.
