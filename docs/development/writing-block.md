Please perform a comprehensive technical review of the entire `Nestworth-go` repository.

Context:
- Nestworth is a local-first personal net worth / asset tracking desktop application.
- The current implementation mainly uses Go + Wails v3 + a web frontend + SQLite.
- This is not just a code-style review. Focus on architecture, domain correctness, financial correctness, reliability, maintainability, and long-term extensibility.

Review the project from these areas:

1. Domain model
   - Account / position / holding / member / activity / history relationships
   - Multi-currency accounts, brokerage accounts, bank accounts, stocks, ETFs, funds, crypto, cash, etc.
   - Whether the model can represent realistic financial scenarios cleanly
   - Domain invariants and impossible/invalid states

2. Financial correctness
   - Decimal/money handling and rounding
   - FX conversion
   - Cost basis
   - Realized/unrealized gains
   - Transfers, fees, dividends, buy/sell operations
   - Net worth and historical valuation calculations

3. Architecture
   - Package/module boundaries
   - Domain/application/infrastructure separation
   - Dependency direction
   - Service/repository responsibilities
   - Over-coupling, duplicated business logic, God objects/services

4. Wails v3 integration
   - Go ↔ frontend service boundaries
   - Generated bindings and DTO design
   - Excessive bridge calls
   - Event usage
   - Lifecycle/startup/shutdown handling
   - Whether Wails-specific concerns leak into domain logic

5. SQLite and persistence
   - Schema design, constraints and indexes
   - Transaction boundaries
   - Migration strategy and migration tests
   - Repository implementation
   - Data consistency and crash safety
   - Backup/recovery considerations

6. Go backend quality
   - Error handling
   - Context usage
   - Concurrency/goroutines
   - Resource lifecycle
   - Interface usage
   - Testability
   - Global state and initialization
   - Significant code smells

7. Frontend
   - Component and feature boundaries
   - State management
   - Form and validation logic
   - Whether business logic is duplicated in the frontend
   - Loading/error/empty states
   - General maintainability and UX architecture

8. Reliability
   - Atomic operations
   - Crash/interruption behavior
   - Idempotency
   - Undo/correction behavior
   - Partial writes
   - Risk of silent data corruption

9. Security/privacy
   - Local database and credentials
   - Logging sensitive information
   - File/import handling
   - Wails frontend-to-Go trust boundary

10. Performance
   - SQLite query patterns
   - Large history/position datasets
   - Frontend rendering
   - Wails bridge overhead
   - Obvious scalability bottlenecks

11. Tests and engineering
   - Unit/integration tests
   - Financial calculation tests
   - Migration tests
   - Repository tests
   - Frontend tests
   - CI/build/release coverage

Do not spend much space on minor formatting or naming issues unless they indicate a broader problem.

For every important issue, include:
- Severity: Critical / High / Medium / Low
- Location or relevant files
- Problem
- Why it matters
- Recommended fix

Also distinguish between:
- actual bugs
- architectural problems
- technical debt
- missing tests
- optional improvements

Output a Markdown report, preferably structured as:

# Nestworth-go Technical Review

## Executive Summary
## Critical / High Priority Findings
## Domain & Financial Model
## Architecture
## Persistence
## Wails Integration
## Backend
## Frontend
## Reliability & Security
## Performance
## Testing & Engineering
## Technical Debt
## Recommended Roadmap

At the end, provide a prioritized remediation roadmap:
1. Fix immediately
2. Fix before the next major feature
3. Medium-term refactoring
4. Optional improvements

Please inspect the actual implementation thoroughly before reaching conclusions. Do not infer problems purely from filenames or architecture assumptions; cite concrete code evidence wherever possible.