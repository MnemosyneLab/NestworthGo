import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AssetCompositionSankey } from "./AssetCompositionSankey";

vi.mock("@/components/charts/EChart", () => ({
  EChart: ({ ariaLabel }: { ariaLabel: string }) => <div role="img" aria-label={ariaLabel} />,
}));

const empty = { positive: "0", negative: "0", zeroCount: 0, missingCount: 0, paths: [], nodes: [] };

describe("Asset composition Sankey states", () => {
  it("distinguishes unavailable values from a known zero", () => {
    const { rerender } = render(<AssetCompositionSankey composition={{ ...empty, missingCount: 1 }} assets="0" currency="JPY" />);
    expect(screen.getByText("Asset values are unavailable")).toBeInTheDocument();
    expect(screen.queryByText(/JP¥/)).toBeNull();

    rerender(<AssetCompositionSankey composition={{ ...empty, zeroCount: 1 }} assets="0" currency="JPY" />);
    expect(screen.getByText("No positive asset values to chart")).toBeInTheDocument();
    expect(screen.getByText("JP¥0")).toBeInTheDocument();
    expect(screen.getByText("1 zero-value component has no band.")).toBeInTheDocument();
  });

  it("shows the signed offset even with no positive ribbons", () => {
    render(<AssetCompositionSankey composition={{ ...empty, negative: "-3", missingCount: 1 }} assets="-3" currency="USD" />);
    expect(screen.getByText("Positive assets")).toBeInTheDocument();
    expect(screen.getByText("US$0.00")).toBeInTheDocument();
    expect(screen.getByText("Negative components").nextElementSibling).toHaveTextContent("-US$3.00");
    expect(screen.getByText("Known asset subtotal").nextElementSibling).toHaveTextContent("-US$3.00");
    expect(screen.queryByRole("img", { name: "Asset composition Sankey" })).toBeNull();
  });

  it("unmounts the canvas when Overview is hidden for account navigation", () => {
    vi.stubGlobal("ResizeObserver", class {
      constructor(private notify: (entries: { contentRect: { width: number } }[]) => void) {}
      observe() { this.notify([{ contentRect: { width: 900 } }]); }
      disconnect() {}
    });
    try {
      const composition = {
        ...empty, positive: "1",
        paths: [{ assetClass: "cash", institutionId: "", institutionName: "", accountId: "synthetic", accountName: "Savings", amount: "1" }],
      };
      const { rerender } = render(<AssetCompositionSankey composition={composition} assets="1" currency="USD" />);
      expect(screen.getByRole("img", { name: "Asset composition Sankey" })).toBeInTheDocument();
      rerender(<AssetCompositionSankey composition={composition} assets="1" currency="USD" isActive={false} />);
      expect(screen.queryByRole("img", { name: "Asset composition Sankey" })).toBeNull();
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
