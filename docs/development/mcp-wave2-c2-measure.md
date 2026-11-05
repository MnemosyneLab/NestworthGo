# MCP Wave 2 C2 drill-down fold

Status: **implemented**. `get_contribution_item`, `get_return_day`, and
`get_asset_driver_detail` are no longer registered. The same evidence is
`get_read_item` on the parent capture.

## Design

`list_contributions` already froze each row's `components` and `byAccount`.
The fold points `get_read_item` at that capture (`section=rows`, `id` = the
row key). That adds no capture bytes.

`analyze_period` now builds the return calendar and all 11 asset-driver
details on the same write fence as income, expenses, asset change, and the
return trend. `get_read_item` then projects one day or one driver:

| Item | `section` | `id` | Child pages |
| --- | --- | --- | --- |
| Return day | `returnDays` | `YYYY-MM-DD` inside the capture range | `composition`, `contributors`, `issues` |
| Asset driver | `assetDrivers` | bucket key (`external_flow` through `residual`) | `byInstrument`, `byAccount`, `residualDetails` |

Those values are not stored in the marshaled capture. `boundedReadCache.store`
charges `len(jsoncodec.Marshal(capture))` against `readCaptureLimit`
(4,194,304 bytes). The day series and driver DTOs sit on an unexported
pointer that JSON skips, and `store` puts that pointer back after its
decode. `get_read_item` and `get_read_page` project one section from that
pointer. They do not refresh snapshots and they do not move `expires`.

The pointer is dropped when the capture expires (5 minutes) or is evicted
(8 captures, or the marshaled bytes would pass 16 MiB). The cache byte
counter still counts only the marshaled parent. A client can open any day
the calendar froze, including days past the point where stuffing every day
into the JSON would have crossed 4 MiB.

`readCaptureLimit` was not raised. Putting every return day into the parent
JSON is still over the cap; that shape is not what `analyze_period` stores.

## Re-measure

Same opt-in test as the original measurement,
`TestMeasureC2ParentCaptureExtremes` in
`internal/mcpserver/c2_capture_measure_test.go`. Ordinary `go test` skips it.

```text
NESTWORTH_C2_MEASURE=1 go test ./internal/mcpserver -run 'TestMeasureC2ParentCaptureExtremes' -count=1 -timeout 30m -v
```

`production_json_bytes` is the cache charge of `periodReadCapture`, the
function `analyze_period` stores. `naive_folded_bytes` still attaches every
return day and all 11 drivers to that JSON, which is the shape that was
rejected. The test fails if the production charge exceeds `readCaptureLimit`.

Host: Linux x86_64, Go 1.27.3, clock `2026-01-02T12:00:00Z`, household
`SYNTHETIC c2`. Books match the original note. This run passed.

| Window | Days | production JSON | naive folded | production over cap | naive over cap |
| --- | ---: | ---: | ---: | --- | --- |
| active year 2025-01-02..2025-12-31 | 364 | 86,117 | 627,737 | false | false |
| quiet 14-year 2012-01-01..2025-12-31 | 5,114 | 1,004,169 | 4,473,188 | false | true |
| quiet 18-year 2008-01-01..2025-12-31 | 6,575 | 1,290,525 | 5,748,641 | false | true |

The 14-year and 18-year production charges match today's period body (the
previous `period_now` column). The naive fold is still 4,473,188 and
5,748,641 bytes. `analyze_period` does not fail `too_large` on those windows.

Extra work on the analyze fence, after the period projections, was small next
to the projection itself: 14-year calendar 238 ms and 11 drivers 52 ms on a
12.2 s period build; 18-year calendar 370 ms and drivers 48 ms on a 16.5 s
build. Active-year calendar plus drivers was 19 ms on a 1.58 s build.

The retained drill-down is the projected day and driver values, not a second
JSON copy. This run did not isolate the Go heap of that pointer. The naive
incremental (3,469,019 bytes at 14 years, 4,458,116 at 18 years) is the
encoded size that is no longer charged. Process VmRSS during the 14-year
case was 371,528 KiB and during the 18-year case 449,852 KiB, including
SQLite and the analysis working set. Up to 8 captures can hold a drill-down
until the 5-minute TTL. That is outside `readCacheLimit`, which still counts
marshaled bytes only.

## Original measurement

The first pass, at `38cfd205`, froze every return day into the capture JSON
and stopped there. On the quiet 14-year book the period body was 1,004,169
bytes and the same capture with every day and all 11 drivers was 4,473,188
bytes. Shorter windows fit. Contribution detail was already in
`list_contributions` (about 5 KiB to 26 KiB on these books). Asset-driver
sections were 6 to 14 rows. The cap failure was the day series.

| Window | Book | Days | period body | naive folded | over cap |
| --- | --- | ---: | ---: | ---: | --- |
| 2025-12-01..2025-12-31 | quiet | 31 | 7,606 | 35,703 | false |
| 2025-10-01..2025-12-31 | quiet | 92 | 19,562 | 88,956 | false |
| 2025-01-01..2025-12-31 | quiet | 365 | 73,073 | 327,288 | false |
| 2021-01-01..2025-12-31 | quiet | 1,826 | 359,878 | 1,603,723 | false |
| 2016-01-01..2025-12-31 | quiet | 3,653 | 718,682 | 3,201,409 | false |
| 2025-12-01..2025-12-31 | active December | 31 | 8,602 | 59,626 | false |
| 2025-01-02..2025-12-31 | active year | 364 | 86,117 | 627,737 | false |
| 2012-01-01..2025-12-31 | long quiet | 5,114 | 1,004,169 | 4,473,188 | true |
| 2008-01-01..2025-12-31 | long quiet | 6,575 | 1,290,525 | 5,748,641 | true |

`readTTL` is 5 minutes, `readCaptureLimit` is 4,194,304 bytes,
`readCacheLimit` is 16,777,216 bytes, and `readCacheCount` is 8.
