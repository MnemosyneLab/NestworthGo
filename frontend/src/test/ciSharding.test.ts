import { expect, it } from "vitest";
import { assignTestShards } from "../../scripts/ci-test-shards.ts";

function testFiles(): string[] {
  return Object.keys(import.meta.glob("/src/**/*.test.{ts,tsx}")).map(file => file.slice(1));
}

it("partitions every discovered test exactly once, including unmeasured additions", () => {
  const files = [...testFiles(), "src/new-unmeasured.test.ts"];
  for (const count of [1, 2, 4, 6]) {
    const shards = assignTestShards(files, count);
    expect(shards).toHaveLength(count);
    expect(shards.flat().sort()).toEqual([...files].sort());
    expect(new Set(shards.flat()).size).toBe(files.length);
    expect(shards.every(shard => shard.length > 0)).toBe(true);
    expect(assignTestShards([...files].reverse(), count)).toEqual(shards);
  }
});

it("isolates the indivisible History workflow while keeping all other contracts", () => {
  const files = testFiles();
  for (const count of [4, 6]) {
    const shards = assignTestShards(files, count);
    expect(shards[0]).toEqual(["src/features/history/HistoryPage.test.tsx"]);
    expect(shards.slice(1).flat()).toContain("src/App.test.tsx");
    expect(shards.slice(1).flat()).toContain("src/test/keyboardOnly.test.tsx");
  }
});
