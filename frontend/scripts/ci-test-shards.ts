import measured from "../ci-test-weights.json" with { type: "json" };

const weights: Record<string, number> = measured;
const history = "src/features/history/HistoryPage.test.tsx";

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
  const cost = (file: string) => (weights[file] ?? 0.25) + 1.25;
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

