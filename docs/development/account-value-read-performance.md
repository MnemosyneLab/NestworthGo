# Latest account value reads

## Scope and selection

The account list, account detail, general read snapshot and portfolio snapshot
share `loadLatestValues`. At baseline `b08dfb301b7da2310d0f78cdc23d832822f78bc4`,
it visits historical account values and checks each for a newer sibling with
`NOT EXISTS`. This repeats work as an account's observation history grows.

The replacement starts from the household's accounts and selects one value
per account with an indexed, ordered lookup. It reuses
`idx_account_values_latest` and the existing row scanner. No schema, dependency,
API, decimal conversion, transaction boundary or financial policy changes.

This was selected over frontend invalidation deduplication (async refetch
semantics need separate verification) and batching manual-market-data health
reads (broader repository and coverage changes). It provides a measurable
benefit with one localized production query change.

## Preserved behavior

- Select by `effective_at DESC`, then `created_at DESC`, then **rowid DESC**.
  A UUID is not an insertion-order tie-breaker, even though the existing index
  includes `id DESC`.
- Include baseline, event and replay projections exactly as before.
- Preserve archived-account handling and household scope; accounts with no
  observations still have no latest value.
- Preserve balance/manual-value modes and exact decimal parsing.

`latest_values_test.go` keeps the former SQL as an independent selection oracle.
Both versions use the same production scanner. Differential tests cover
backfilled histories, equal timestamps, empty accounts, archived accounts and
all projection kinds. They compare hydrated account lists and both snapshot
read models, and check account detail with IDs ordered opposite to insertion.
An `EXPLAIN QUERY PLAN` assertion checks use of the existing value index.

## Reproducible benchmark

```sh
go test ./internal/infrastructure/sqlite -run '^TestLatestValues' -count=1
go test ./internal/infrastructure/sqlite -run '^$' \
  -bench BenchmarkLatestAccountValues -benchmem -count=5 -benchtime=200ms
```

The fixture uses a temporary current-schema SQLite database, 20 valued accounts
(one archived), one empty account, reverse chronological inserts, and timestamp
ties. Setup is outside the timed loop. `legacy` and `indexed` execute against
the same database and include scanning and decimal parsing. The additional
`tied=true` case gives every observation the same effective and creation times.

Initial measurements on Linux amd64, Go 1.26.0, modernc SQLite 1.49.1,
Intel Xeon Platinum 8370C, GOMAXPROCS=5; medians of five 200 ms samples for
`tied=false`, measured before concurrent full-suite validation:

| Values per account | Previous query | Indexed query | Time reduction |
| --- | ---: | ---: | ---: |
| 1 | 0.202 ms | 0.191 ms | 5.2% |
| 365 | 11.840 ms | 0.238 ms | 98.0% |
| 3650 | 126.582 ms | 0.268 ms | 99.8% |

The indexed path uses about 17.4 KB and 767 allocations per call at all three
sizes; allocation reduction is not the main benefit. The smallest case shows
no observed regression, but its small timing difference is not a significant
product-level claim.

These are warm database microbenchmarks, not cold-disk or end-to-end UI timings.
The latest equal-timestamp group still needs insertion-order selection because
the index ends with UUID rather than rowid. If all timestamps are identical,
work therefore grows with that group; this change does not promise constant
work for arbitrary histories. It also retains the existing household-wide
hydration behavior for filtered lists and single-account reads.
