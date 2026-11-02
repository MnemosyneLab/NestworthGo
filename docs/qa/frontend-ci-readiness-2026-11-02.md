# Frontend CI readiness investigation (2026-11-02)

The subsequent hosted result and revised five-minute CI layout are recorded in
[the CI budget follow-up](ci-five-minute-budget-2026-11-02.md).

## Scope and evidence

Baseline: `5046e32c456160e7b33d8dea3086cb2203860d51` on main. The
implementation and mobile assets are unchanged by this test/workflow fix.
The existing money, navigation, keyboard, focus, date precision and interrupted
submission assertions remain in place. No retries, skips or timeout increases
were added.

[Main's second failed attempt](https://github.com/MnemosyneLab/NestworthGo/actions/runs/41376141053/attempts/2)
had eight failures across six files in frontend shard 2. Its missing `Bank`
control is reproducible locally under constrained CPU affinity.
[PR186's run](https://github.com/MnemosyneLab/NestworthGo/actions/runs/41378540997)
also had five missing instrument-search options. Their failure DOMs contain the
complete NVDA/BTC/SPY/ARKK input values and the loading copy. That supports a
request/render-readiness diagnosis rather than lost keystrokes; it does not
establish that every timeout has the same cause.

## Controlled local samples

Linux x64, Node 24.22.0, pnpm 12.14.0, Vitest 4.1.11 and the frozen lockfile.
Default affinity exposes five logical CPUs with a four-CPU cgroup quota.
The constrained samples use `taskset -c 0,1`; no other checks ran concurrently.
All fixtures are synthetic service mocks; no native application or ledger was
opened. Generated Wails bindings used the environment's existing Debian native
dependency container and Go 1.27.3.

Each sample runs the same 301 tests in these eight files:

```text
src/App.test.tsx
src/test/keyboardOnly.test.tsx
src/features/history/HistoryPage.test.tsx
src/features/historical-overview/HistoricalOverviewPage.test.tsx
src/features/historical-overview/HistoricalDatePicker.test.tsx
src/features/investments/InstrumentManagement.test.tsx
src/features/liquidity/ProductPickers.test.tsx
src/features/upcoming-actions/ReviewEntryIntegration.test.tsx
```

Command: `pnpm exec vitest run <the eight paths above> --maxWorkers=N
--reporter=json --outputFile=/tmp/<sample>.json`, prefixed by
`taskset -c 0,1` for the two-CPU samples. The fixed sample also uses the default
reporter to validate the new CI reporter combination.

| Code | CPU affinity | Workers | Passed / total | Elapsed |
| --- | --- | ---: | ---: | ---: |
| Baseline | Default | 4 | 301 / 301 | 60.84s |
| Baseline | Default | 2 | 301 / 301 | 72.07s |
| Baseline | Two CPUs | 4 | 300 / 301 | 92.91s |
| Baseline | Two CPUs | 2 | 301 / 301 | 79.96s |
| Readiness fixes | Two CPUs | 4 | 301 / 301 | 93.29s |

These are small, sequential qualitative samples, not a throughput or reliability
guarantee. Reducing workers improved individual interaction times and constrained
wall time, but increased default-affinity wall time by about 18%. Hosted-runner
CPU contention remains a hypothesis; these samples do not prove it caused CI's
other failures. Full-suite and repeated affected-test results belong to the PR's
validation record, tied to its implementation head.

## Changes and limits

- Account-focused historical tests now use the supported initial navigation
  context. The removed detached `findByRole().then(click).catch(...)` raced
  assertions and could act on a subsequent mount. The reopen test now names its
  intended account context explicitly. Primary-view tests retain real tab and
  category interactions.
- Search tests await the final debounced request before looking for its options.
  They still type through the real control, use the real debounce and select real
  accessible options; all identity and currency assertions remain.
- The App upcoming-filter test waits for its lazy import boundary before reading
  the destination controls, retaining real navigation and Back/focus assertions.
- The default worker cap is two. The existing 1s async lookup and 5s test defaults
  are unchanged, as are existing explicitly scoped longer tests.
- Both CI shards retain Vitest JSON timings and failures for seven days, alongside
  the normal console reporter. Shard selection and aggregate test gates remain.

This changes no application runtime behavior. Native macOS, browser visual QA and
manual accessibility acceptance were not performed. Hosted CI must still validate
the exact pushed head; a local pass does not prove all historical flakiness solved.
