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
    const collapsed = collapseChartCategories(items);
    expect(collapsed.map((item) => item.key)).toEqual(["k5", "k4", "k3", "k2", "k1", "k0"]);
  });

  it("merges the smallest tail into other when there are more than six categories", () => {
    const items = Array.from({ length: 8 }, (_, index) => ({
      key: `k${index}`,
      label: `k${index}`,
      amount: `${index + 1}`,
      shareBps: 1000,
    }));
    const collapsed = collapseChartCategories(items);
    expect(collapsed.map((item) => item.key)).toEqual(["k7", "k6", "k5", "k4", "k3", "other"]);
    expect(collapsed[5]).toEqual({ key: "other", label: "other", amount: "6", shareBps: 3000 });
  });

  it("hides non-positive amounts instead of treating them as zero slices", () => {
    const collapsed = collapseChartCategories([
      { key: "cash", label: "cash", amount: "100", shareBps: 10000 },
      { key: "empty", label: "empty", amount: "0", shareBps: 0 },
    ]);
    expect(collapsed).toEqual([{ key: "cash", label: "cash", amount: "100", shareBps: 10000 }]);
  });
});
