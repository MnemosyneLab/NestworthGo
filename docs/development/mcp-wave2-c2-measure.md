# MCP Wave 2 C2 capture-cost measurement

Status: **CLOSED-as-measure-only**. The fold of `get_contribution_item`,
`get_return_day`, and `get_asset_driver_detail` into `get_read_item` is not
implemented.

Design section 4 says those three tools can become sections of the parent
capture only after the memory and TTL cost of freezing the drill-down is
measured. `list_contributions` already freezes each row's `components` and
`byAccount`. `analyze_period` does not freeze return days or asset-driver
rows. This note records that cost on synthetic books. No server behavior
changed.

## Verdict

Freezing every return day into the `analyze_period` capture exceeds
`readCaptureLimit` on a range the tool accepts today.

`boundedReadCache.store` rejects a capture whose `jsoncodec.Marshal` length is
greater than `readCaptureLimit` (`4 << 20` = 4,194,304 bytes) with
`too_large`. On the quiet 14-year book below, today's period capture is
1,004,169 bytes and the same capture with every return day and all 11 asset
drivers attached is 4,473,188 bytes. `analyze_period` would start failing for
that query.

Shorter windows fit. An active year of this 4-instrument book is 627,737
bytes (15.0% of the cap). The allowed analysis range is 36,600 days
(`maxAnalysisRangeDays` in `internal/application/analysis_civil_date.go`).
The measured 14-year and 18-year windows are inside that limit and over the
capture cap, so the fold is closed.

`list_contributions` already pays the contribution-item cost. On these books
that incremental JSON is 4,884 to 26,463 bytes. Pointing `get_read_item` at
those existing sections would add no capture bytes. Asset-driver sections
stayed at 6 to 14 rows, with zero residual rows. The cap failure is the
return-day series, and the requested change folds all three tools together.

## Environment

| Item | Value |
| --- | --- |
| Server source | `38cfd205dabdeb036a7332eeada30014cb49ce6c` |
| Capture path at `2b90efca` | `bounded_read.go`, `history_tools.go`, `attribution_tools.go`, `health_read_tools.go`, and `analysis_civil_date.go` are unchanged from `38cfd205` |
| Host | Linux x86_64, 4 CPUs, `Intel(R) Xeon(R) Processor`, Go 1.27.3 |
| Clock | `2026-01-02T12:00:00Z` |
| Household | `SYNTHETIC c2`, base currency USD, timezone UTC, member `Synthetic owner` |
| Quotes | manual only; the market-data registry was empty |

Cache constants used as the comparison:

| Constant | Value |
| --- | --- |
| `readTTL` | 5m0s |
| `readCaptureLimit` | 4,194,304 bytes |
| `readCacheLimit` | 16,777,216 bytes |
| `readCacheCount` | 8 |
| `readBuildTimeout` | 10s, around capture assembly |

The cache charges `len(jsoncodec.Marshal(capture))` after
`normalizeFrozenQuery`. That is the same length `store` assigns to `e.size`.
`get_read_item` and `get_read_page` do not move `expires`. A stored capture
stays until that 5-minute expiry or until eviction (8 entries, or the running
byte total would pass 16 MiB).

## Method

Opt-in test `TestMeasureC2ParentCaptureCost` and
`TestMeasureC2ParentCaptureExtremes` in
`internal/mcpserver/c2_capture_measure_test.go`. Ordinary `go test` skips
them. Re-run:

```text
NESTWORTH_C2_MEASURE=1 go test ./internal/mcpserver -run 'TestMeasureC2ParentCapture' -count=1 -timeout 30m -v
```

The period capture is built with `sectionedReadCapture` the way
`analyze_period` builds `income/rows`, `expenses/rows`, `assetChange/waterfall`,
`assetChange/groups`, `investmentReturns/points`, and
`investmentReturns/sources`. The folded capture then attaches each
`ReturnCalendar` cell (empty cursor, granularity `day`) as
`returnDays/<date>` with sections `composition`, `contributors`, and `issues`,
and each `AssetDriverDetail` for the 11 bucket keys as
`assetDrivers/<key>` with sections `byInstrument`, `byAccount`, and
`residualDetails`. Contribution captures use `CaptureContributions` and the
same per-item attachment `list_contributions` uses today. Rows-only size drops
`Details` and the child sections.

`ReturnCalendar` and `AssetDriverDetail` run after the period projections, so
their times are memoized projections. The naive column calls `ReturnDay` once
per day after that memo. Windows longer than 366 days record 31 of those
calls. Timings are one shot in this process. They are not a percentile.

HeapAlloc is read after `runtime.GC`, then the folded capture is marshaled
and decoded again, then GC runs once more. The delta keeps both the JSON
buffer and the decoded value. VmRSS is the whole process, including SQLite
and the analysis memo.

## Books

All books have two brokerage accounts and four manual USD stocks. Each
account receives 100000 USD and buys 10 shares of each stock for 400 USD plus
a 1 USD fee.

| Book | What was recorded |
| --- | --- |
| Quiet carried-price | History start 2016-01-01. Cash and buys on 2016-01-04. Quotes 40 on 2016-01-04 and 55 on 2025-12-31. One 4-share sale on 2020-06-15 and one 12 USD dividend on 2024-03-15, both on the first stock of the first account. |
| Active December | The quiet book, plus one quote per stock on every day 2025-12-01 through 2025-12-31. Price is `40 + day index`. |
| Active year | History start 2025-01-01. Cash and buys on 2025-01-02. One quote per stock on every day 2025-01-02 through 2025-12-31. Price is `40 + (day index mod 50)`. |
| Long quiet | History start 2000-01-01. Cash, buys, and a quote of 40 on 2012-01-04 (14-year window) or 2008-01-04 (18-year window). A second quote of 40 on 2025-12-31. No sale and no dividend. |

## Results

Bytes are the cache charge. `period_now` is today's `analyze_period` capture.
`folded` adds every return day and all 11 drivers. `over_cap` is
`folded > 4194304`.

| Window | Book | Days | period_now | folded | incremental | folded / cap | over_cap |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| 2025-12-01..2025-12-31 | quiet | 31 | 7,606 | 35,703 | 28,097 | 0.008512 | false |
| 2025-10-01..2025-12-31 | quiet | 92 | 19,562 | 88,956 | 69,394 | 0.021209 | false |
| 2025-01-01..2025-12-31 | quiet | 365 | 73,073 | 327,288 | 254,215 | 0.078032 | false |
| 2021-01-01..2025-12-31 | quiet | 1,826 | 359,878 | 1,603,723 | 1,243,845 | 0.382357 | false |
| 2016-01-01..2025-12-31 | quiet | 3,653 | 718,682 | 3,201,409 | 2,482,727 | 0.763275 | false |
| 2025-12-01..2025-12-31 | active December | 31 | 8,602 | 59,626 | 51,024 | 0.014216 | false |
| 2025-01-02..2025-12-31 | active year | 364 | 86,117 | 627,737 | 541,620 | 0.149664 | false |
| 2012-01-01..2025-12-31 | long quiet | 5,114 | 1,004,169 | 4,473,188 | 3,469,019 | 1.066491 | true |
| 2008-01-01..2025-12-31 | long quiet | 6,575 | 1,290,525 | 5,748,641 | 4,458,116 | 1.370583 | true |

Day shape and driver rows from the same runs:

| Window | Middle day bytes | Middle contributors | Middle composition | Max day bytes | Max contributors | Day section rows | Driver section rows |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| quiet month | 540 | 0 | 0 | 1,339 | 4 | 5 | 6 |
| quiet year | 540 | 0 | 0 | 1,339 | 4 | 5 | 6 |
| quiet 10-year | 540 | 0 | 0 | 1,339 | 4 | 15 | 14 |
| active December | 1,331 | 4 | 1 | 1,331 | 4 | 150 | 6 |
| active year | 1,331 | 4 | 1 | 1,345 | 4 | 1,820 | 12 |
| quiet 14-year | 540 | 0 | 0 | 1,319 | 4 | 5 | 6 |
| quiet 18-year | 540 | 0 | 0 | 1,319 | 4 | 5 | 6 |

Quiet middle days carry a price and have an `ok` status with no composition
and no contributors. Active days have four instrument contributors and one
composition line. The 14-year over-cap capture is mostly those 540-byte
carried days. Populated days are larger: the active year stores 1,820 day
section rows and 627,737 folded bytes in 364 days.

Calendar status was `ok` and `available=true` on every window. Asset-change
status was `ok`. Residual section rows were 0.

Contribution captures (`total_return`, `groupBy=instrument`) already include
item detail:

| Window | Items | With detail | Rows only | Detail already stored | Build |
| --- | ---: | ---: | ---: | ---: | --- |
| quiet month | 5 | 6,646 | 1,762 | 4,884 | 904.9µs |
| quiet quarter | 5 | 6,646 | 1,762 | 4,884 | 4.602ms |
| quiet year | 5 | 6,668 | 1,774 | 4,894 | 16.695ms |
| quiet 5-year | 5 | 6,788 | 1,791 | 4,997 | 144.6ms |
| quiet 10-year | 5 | 7,278 | 1,868 | 5,410 | 591.0ms |
| active December | 5 | 10,071 | 3,474 | 6,597 | 2.111ms |
| active year | 5 | 49,469 | 23,006 | 26,463 | 23.413ms |
| quiet 14-year | 5 | 6,787 | 1,834 | 4,953 | 1.043s |
| quiet 18-year | 5 | 6,787 | 1,834 | 4,953 | 1.628s |

## Time and retention

One-shot wall time. Period build is the four projections `analyze_period`
already runs, and it sits outside `readBuildTimeout`. Calendar and driver
times are the extra projections used to freeze drill-down.

| Window | Period build | Calendar | 11 drivers | Naive ReturnDay |
| --- | --- | --- | --- | --- |
| quiet month | 95.2ms | 484µs | 236µs | 31/31 in 6.67ms |
| quiet quarter | 203.0ms | 3.06ms | 565µs | 92/92 in 69.8ms |
| quiet year | 782.1ms | 6.44ms | 2.00ms | 365/365 in 719ms |
| quiet 5-year | 3.648s | 68.6ms | 15.0ms | 31/1826 in 401ms |
| quiet 10-year | 7.829s | 149ms | 25.8ms | 31/3653 in 759ms |
| active December | 106.4ms | 948µs | 1.07ms | 31/31 in 9.00ms |
| active year | 1.513s | 8.12ms | 10.3ms | 364/364 in 1.266s |
| quiet 14-year | 12.222s | 250ms | 43.0ms | 31/5114 in 1.097s |
| quiet 18-year | 17.431s | 413ms | 68.6ms | 31/6575 in 1.686s |

Quiet-month bytes matched an earlier run whose period build was 107.2ms.
The quiet-month row above is the later run, which also counted per-day sizes.
The quiet quarter was measured only in the earlier run.

Retention after the second decode:

| Window | HeapAlloc before | HeapAlloc after | Delta | VmRSS KiB |
| --- | ---: | ---: | ---: | ---: |
| quiet month | 4,128,888 | 4,611,480 | 482,592 | 43,184 |
| quiet year | 12,366,496 | 14,911,680 | 2,545,184 | 70,344 |
| quiet 10-year | 106,450,536 | 122,806,464 | 16,355,928 | 332,152 |
| active year | 5,211,104 | 6,171,616 | 960,512 | 62,776 |
| quiet 14-year | 27,619,288 | 29,344,824 | 1,725,536 | 393,100 |
| quiet 18-year | 33,840,840 | 36,728,480 | 2,887,640 | 613,924 |

The 10-year quiet delta of 16,355,928 bytes is the reading where the 3,201,409-byte
buffer and the decoded capture were both still referenced and the process had
not yet dropped a larger analysis working set. The 14-year delta is smaller
than its JSON charge because GC reclaimed other memory between the two reads.
VmRSS at 14 years was 393,100 KiB and at 18 years was 613,924 KiB for the
whole process.

TTL stays 5 minutes. The 10-year quiet capture would hold 3,201,409 cache
bytes, 19.1% of the 16 MiB cache, until expiry. The 14-year capture is not
stored.

## What was not changed

`get_contribution_item`, `get_return_day`, and `get_asset_driver_detail`
stay registered. `readOutputKind`, discovery snapshots, skill text, and
`skills/nestworth/VERSION` stay as they were. Verification is this one Linux
process.
