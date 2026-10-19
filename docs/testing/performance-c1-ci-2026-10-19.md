# PR114 CI failure classification and controlled comparison

Run [39214731606](https://github.com/MnemosyneLab/NestworthGo/actions/runs/39214731606), reviewed head `0cfd711d9d61c37d5a28559e647c05b07e5f5806`, tested GitHub merge ref `e932b04064f61a4aad161a10d52d3e44972633d4` against base `63fc5e1c16f835f56d13869375f3755f4bfe0767`. This is preserved failure evidence, not a canonical successful run. Parent subsequently merged C1 as `176bfb0d215d06058c331f444f8a66cafc712e14`, including the intervening PR115/116 fixes. No merged history was rewritten.

## Exact failures

| Job | Result | Evidence |
| --- | --- | --- |
| [test117935864863](https://github.com/MnemosyneLab/NestworthGo/actions/runs/39214731606/job/117935864863) | `TestHistoricalOverviewDenseEntrySingleAndComparison/series-endpoint-equivalence` rejected at10.00s with `local trend exceeded its computation deadline`; full test91.06s | Normal mandatory success/equality assertion failed. No wrong amount, race report or partial financial result was reported. |
| [application4/4 117935865439](https://github.com/MnemosyneLab/NestworthGo/actions/runs/39214731606/job/117935865439) | Cumulative package timeout600.036s | Cancellation evidence test completed380.20s; dense overview124.26s; reconciliation coverage33.51s, plus other tests. Test active at alarm was only7s old. |
| [application2/4 117935865504](https://github.com/MnemosyneLab/NestworthGo/actions/runs/39214731606/job/117935865504) | Cumulative package timeout600.036s | Fact-limit fixture142.19s; overview cancellation115.91s; snapshot list59.59s; backup lifecycle48.75s; C1 oracle27.12s, plus other tests. Active tests at alarm were3–46s old. |
| [infrastructure117935865511](https://github.com/MnemosyneLab/NestworthGo/actions/runs/39214731606/job/117935865511) | SQLite package timeout600.038s | New bounded-retention test98.16s and pruning-failure test81.23s contribute materially. Alarm occurred22s into a later v15 upgrade/reopen test. |

No data-race detector finding or financial correctness assertion preceded the three race timeouts. Completed heavy tests and active-test ages support cumulative exhaustion; they do not establish a deadlock. The race aggregate correctly failed because required children failed. All four decoded job logs were read through the authorized GitHub connector after direct CLI archive downloads failed; [compact extracted failures/timings](performance-c1-ci-2026-10-19/) retain the exact observations without copying large setup/dependency/stack logs.

## Controlled same-environment base/current checks

Production/test code was compiled into separate executables **before** measurement from base63fc5e1 and failed head0cfd711. No production or fixture changes were made to either build. Runs used the identical existing test functions/flags/fixtures, one process at a time, Go1.26.0, modernc SQLite3.53.0, Linux x86_64, Xeon Platinum8573C, GOMAXPROCS4 (container quota4 CPUs). No concurrent test/build/race/profile process ran during these measurements. UUIDs are generated independently by the unchanged fixtures; these are comparable workload checks, not identical-byte database clones. Each result below is **one sample**, not a distribution, p95, or controlled cold-cache estimate. This container is not the GitHub runner hardware.

| Unchanged test | Base63fc5e1 | C1 head0cfd711 |
| --- | ---: | ---: |
| Dense overview total, normal | 30.21s | 36.15s |
| Dense single day | 5.31s | 5.13s |
| Dense comparison | 5.56s | 5.90s |
| Dense series endpoint equality, original10s deadline | 5.59s | 6.39s |
| Full dense cancellation test, race/parallel4/timeout10m | 278.42s | 408.23s |
| Actual cancellation after10ms scheduling | 10.324ms | 11.250ms |

Both normal builds completed the full unchanged endpoint equality/correctness assertions below10s. The CI deadline failure did not reproduce in this controlled pair; one pair is not proof of canonical CI success or a production p95. It remains a retained CI timing failure, not a waived test.

The 278.42s/408.23s figures are Go's full test elapsed times, including fixture construction, GC, the cancellation call and cleanup. Only the approximately10ms call is separately timed after setup. The write/setup attribution below is inferred from that full-test difference and the separately measured call; no isolated fixture-setup timer was installed.

The race cancellation test constructs the same100-account,9000-backfill,25000-activity dense financial database before scheduling cancellation. Source tracking added129.81s (46.6%) to this write-heavy fixture's total, while cancellation itself stayed near10ms. This is material **write/setup overhead**, consistent with the already reported C1 primitive/batch write measurements. It is not a stuck cancellation path or evidence that financial validation should be removed. The per-row transactional revision/journal and bounded pruning are active in fixtures exactly as in product writes. Those costs remain visible; sharding does not make them disappear. No individual test needed a timeout increase in the controlled comparison.

## Coordination and next step

No workflow/shard script was modified by this investigation. Recommended splitting the existing deterministic application mapping from4 to8: the completed cancellation test maps to part3/8, dense overview to7/8. Splitting SQLite into at least2 test-name parts separates the98s retention test (part0/2) and81s failure test (part1/2). Preserve the existing10m process budget, race instrumentation, complete Test/Example/Fuzz coverage, and all-shards-required aggregate. Parent/Luna owns independent [CI-only PR117](https://github.com/MnemosyneLab/NestworthGo/pull/117), which implements that separation; its coverage/merge review is outside this core branch.

No new reproducible financial correctness defect was found in C1. A blanket trigger/verification rewrite was not justified by these results: the accepted journaling cost is explicit, and one unreplicated deadline rejection is insufficient evidence for changing financial logic. C2 work proceeds independently on merged main, preserving all full source rechecks and the full recompute oracle. It makes no claim to remove C1's write cost. New canonical CI results remain necessary; isolated success is not substituted for them.
