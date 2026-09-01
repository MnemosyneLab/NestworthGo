import { describe, expect, it } from "vitest";
import { collapseChartCategories } from "./collapseCategories";

describe("collapseChartCategories", () => {
  it("keeps six or fewer positive categories", () => {
    const items = Array.from({ length: 6 }, (_, index) => ({
      key: `k${index}`,
      label: `k${index}`,
      amount: `${index + 1}`,
      shareBps: 1000,
    }));
    expect(collapseChartCategories(items)).toHaveLength(6);
  });

  it("merges the tail into other when there are more than six categories", () => {
    const items = Array.from({ length: 8 }, (_, index) => ({
      key: `k${index}`,
      label: `k${index}`,
      amount: "10",
      shareBps: 1000,
    }));
    const collapsed = collapseChartCategories(items);
    expect(collapsed).toHaveLength(6);
    expect(collapsed[5]).toEqual({ key: "other", label: "other", amount: "30", shareBps: 3000 });
  });

  it("hides non-positive amounts instead of treating them as zero slices", () => {
    const collapsed = collapseChartCategories([
      { key: "cash", label: "cash", amount: "100", shareBps: 10000 },
      { key: "empty", label: "empty", amount: "0", shareBps: 0 },
    ]);
    expect(collapsed).toEqual([{ key: "cash", label: "cash", amount: "100", shareBps: 10000 }]);
  });
});
