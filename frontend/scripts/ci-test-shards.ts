import measured from "../ci-test-weights.json" with { type: "json" };

// Median file elapsed seconds from complete JSON reports in hosted runs
// 41406212751 and 41412429440 attempts 1/2; refresh from comparable runs.
const weights: Record<string, number> = measured;
const history = "src/features/history/HistoryPage.test.tsx";

// Non-History shards in run 41412429440 reported 1.8–3.3s per file for
// transform/setup/import/environment (median 2.92s). Use a rounded scheduling
// estimate; it is not a wall-time prediction for the two concurrent workers.
const fileSetupSeconds = 3;

/** Deterministic LPT assignment from hosted timings, including per-file setup.
 * History is one indivisible, expensive file: give it its own process budget.
 * All other files, including newly added/unmeasured ones, are assigned once. */
export function assignTestShards(files: string[], count: number): string[][] {
  if (!Number.isInteger(count) || count < 1) throw new Error("invalid shard count");
  const bins: string[][] = Array.from({ length: count }, () => []);
  const loads = bins.map(() => 0);
  const dedicated = count >= 4 && files.includes(history);
  if (dedicated) bins[0].push(history);
  const rest = files.filter(file => !dedicated || file !== history);
  const cost = (file: string) => (weights[file] ?? 0.25) + fileSetupSeconds;
  rest.sort((a, b) => cost(b) - cost(a) || (a < b ? -1 : a > b ? 1 : 0));
  for (const file of rest) {
    let target = dedicated ? 1 : 0;
    for (let index = target + 1; index < count; index++) {
      if (loads[index] < loads[target]) target = index;
    }
    bins[target].push(file);
    loads[target] += cost(file);
  }
  return bins;
}

