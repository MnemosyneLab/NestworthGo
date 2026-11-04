import { describe, expect, it } from "vitest";
import { destinationKey, revisitTarget, appendVisit, recordRecent } from "./navigationHistory";

describe("revisitable destinations", () => {
  it("separates one-time intents from account and activity destinations", () => {
    expect(revisitTarget({ page: "accounts", accountId: "synthetic-account", create: true, updateValue: true })).toEqual({ page: "accounts", accountId: "synthetic-account", filter: undefined });
    expect(revisitTarget({ page: "history", activityId: "synthetic-activity", filters: { accountId: "synthetic-account" } })).toEqual({ page: "history", filters: { accountId: "synthetic-account" } });
    expect(revisitTarget({ page: "available-funds", accountId: "synthetic-account", focus: { kind: "receipt", productId: "synthetic-product" } })).toEqual({ page: "available-funds", productId: undefined, focus: undefined });
  });
  it("deduplicates equivalent optional fields and normalizes aliases", () => {
    expect(destinationKey({ page: "history", filters: { from: "2026-10-01", accountId: "synthetic", to: undefined } })).toBe(destinationKey({ page: "history", filters: { accountId: "synthetic", from: "2026-10-01" } }));
    expect(destinationKey({ page: "reconciliation", accountId: "synthetic" })).toBe(destinationKey({ page: "tasks", tab: "reconciliation", accountId: "synthetic" }));
  });
  it("distinguishes object identity and effective semantic filters", () => {
    expect(destinationKey({ page: "accounts", accountId: "synthetic-a" })).not.toBe(destinationKey({ page: "accounts", accountId: "synthetic-b" }));
    expect(destinationKey({ page: "instrument-detail", instrumentId: "synthetic-a", from: "2026-10-01" })).not.toBe(destinationKey({ page: "instrument-detail", instrumentId: "synthetic-a", from: "2026-10-02" }));
  });
  it("copies targets so later caller mutation cannot rewrite history", () => {
    const target = { page: "history" as const, filters: { kinds: ["buy"] } };
    const saved = revisitTarget(target);
    target.filters.kinds.push("sell");
    expect(saved).toEqual({ page: "history", filters: { kinds: ["buy"] } });
  });
});


it("caps navigation without retaining the truncated forward branch", () => {
  let entries = [0];
  for (let id = 1; id <= 45; id++) entries = appendVisit(entries, entries.length - 1, entries[entries.length - 1], id);
  expect(entries).toHaveLength(40);
  expect(entries[0]).toBe(6);
  expect(entries[entries.length - 1]).toBe(45);
  expect(appendVisit(entries, 2, entries[2], 99)).toEqual([6, 7, 8, 99]);
});

it("keeps an independent bounded MRU after branch truncation and moves duplicates to the front", () => {
  type Item = { target: { page: "accounts"; accountId: string }; revision: number };
  let recent: Item[] = [];
  for (let id = 0; id < 25; id++) recent = recordRecent(recent, { target: { page: "accounts", accountId: `synthetic-${id}` }, revision: 1 });
  expect(recent).toHaveLength(20);
  const revisited: Item = { target: { page: "accounts", accountId: "synthetic-10" }, revision: 2 };
  recent = recordRecent(recent, revisited);
  expect(recent).toHaveLength(20);
  expect(recent[0]).toEqual(revisited);
  expect(recent.filter(item => item.target.accountId === "synthetic-10")).toHaveLength(1);
});
