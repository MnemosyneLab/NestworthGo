import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AssetCompositionSankey } from "./AssetCompositionSankey";
import type { EChartProps } from "@/components/charts/EChart";
import type { SankeySeriesOption } from "echarts/charts";

const drawChart = vi.hoisted(() => vi.fn<(props: EChartProps) => void>());

vi.mock("@/components/charts/EChart", () => ({
  EChart: (props: EChartProps) => {
    drawChart(props);
    return <div role="img" aria-label={props.ariaLabel} />;
  },
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
        nodes: [{ id: "assets", parentId: "", kind: "total", key: "", amount: "1" }],
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

it("draws every real branch at compact widths and formats complete paths for interaction", async () => {
  const longAccount = "Synthetic cross-border investment and retirement account with a long name";
  const longInstitution = "Synthetic institution with a shared and very long business name";
  const cashPaths = Array.from({ length: 9 }, (_, index) => ({
    assetClass: "cash", institutionId: "inst-a", institutionName: longInstitution, accountId: `account-${index}`,
    accountName: longAccount, amount: index === 8 ? "900.123456" : "1000",
  }));
  const paths = [
    ...cashPaths,
    { assetClass: "cash", institutionId: "inst-b", institutionName: longInstitution, accountId: "account-tiny", accountName: "Small savings", amount: "0.002" },
    { ...cashPaths[8], assetClass: "stock", amount: "2000.25" },
    { assetClass: "bond", institutionId: "", institutionName: "", accountId: "account-unassigned", accountName: "Unassigned bonds", amount: "0.003" },
    { assetClass: "bond", institutionId: "inst-c", institutionName: "No institution", accountId: "account-named", accountName: "Named bonds", amount: "0.001" },
  ];
  const node = (id: string, parentId: string, kind: string, key: string, amount: string) => ({ id, parentId, kind, key, amount });
  const nodes = [
    node("assets", "", "total", "", "10900.379456"),
    node("cash", "assets", "assetClass", "cash", "8900.125456"),
    node("cash-a", "cash", "institution", "inst-a", "8900.123456"),
    ...cashPaths.map(path => node(`cash-${path.accountId}`, "cash-a", "account", path.accountId, path.amount)),
    node("cash-b", "cash", "institution", "inst-b", "0.002"),
    node("tiny", "cash-b", "account", "account-tiny", "0.002"),
    node("stock", "assets", "assetClass", "stock", "2000.25"),
    node("stock-a", "stock", "institution", "inst-a", "2000.25"),
    node("stock-account-8", "stock-a", "account", "account-8", "2000.25"),
    node("bond", "assets", "assetClass", "bond", "0.004"),
    node("bond-unassigned", "bond", "institution", "", "0.003"),
    node("unassigned", "bond-unassigned", "account", "account-unassigned", "0.003"),
    node("bond-c", "bond", "institution", "inst-c", "0.001"),
    node("named", "bond-c", "account", "account-named", "0.001"),
  ];
  const onOpenAccount = vi.fn();
  render(<AssetCompositionSankey composition={{ ...empty, positive: nodes[0].amount, paths, nodes }}
    assets={nodes[0].amount} currency="USD" onOpenAccount={onOpenAccount} />);

  const viewport = screen.getByRole("region", { name: "Scrollable allocation chart" });
  expect(viewport).toHaveAttribute("tabindex", "0");
  expect(within(viewport).getByRole("img")).toBeInTheDocument();
  const chart = drawChart.mock.lastCall![0];
  const series = chart.option.series as SankeySeriesOption;
  expect(chart.renderer).toBe("svg");
  expect(series.data?.map(item => item.name)).toEqual(nodes.map(item => item.id));
  expect(series.links?.map(link => [link.source, link.target])).toEqual(nodes.slice(1).map(item => [item.parentId, item.id]));
  expect(series.emphasis?.focus).toBe("trajectory");
  const tooltip = (chart.option.tooltip as { formatter: (params: unknown) => string }).formatter;
  const cashTooltip = tooltip({ dataType: "node", data: { assetNodeId: "cash-account-8" } });
  expect(cashTooltip).toContain(`Cash › ${longInstitution} (1) › ${longAccount} (9)`);
  expect(cashTooltip).toContain("US$900.12");
  expect(cashTooltip).not.toContain("900.123456");
  expect(tooltip({ dataType: "edge", data: { assetNodeId: "stock-account-8" } }))
    .toContain(`Stock › ${longInstitution} (1) › ${longAccount} (9)`);
  const table = within(screen.getByRole("table", { name: "View allocation details" }));
  expect(table.getAllByRole("row")).toHaveLength(paths.length + 1);
  expect(table.getAllByRole("row", { name: /US\$0\.00/ })).toHaveLength(3);
  expect(table.getByRole("button", { name: "Unassigned bonds" })).toHaveAccessibleDescription("Bond · No institution");
  expect(table.getByRole("button", { name: "Named bonds" })).toHaveAccessibleDescription("Bond · No institution (1)");
  expect(table.queryByText(/900\.123456|Other institutions|Other accounts/)).not.toBeInTheDocument();
  const [cash, stock] = table.getAllByRole("button", { name: `${longAccount} (9)` });
  expect(cash.closest("tr")).toHaveTextContent("US$900.12");
  expect(stock.closest("tr")).toHaveTextContent("US$2,000.25");
  expect(cash).toHaveAccessibleDescription(`Cash · ${longInstitution} (1)`);
  expect(stock).toHaveAccessibleDescription(`Stock · ${longInstitution} (1)`);
  stock.focus();
  await userEvent.setup().keyboard("{Enter}");
  chart.onSelectName?.("cash-account-8");
  expect(onOpenAccount.mock.calls).toEqual([["account-8"], ["account-8"]]);
});
