import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { CompositionChart } from "@/components/charts/CompositionChart";
import { applyChartPolicy } from "@/components/charts/EChart";
import { SignedBarChart } from "@/components/charts/SignedBarChart";
import { TrendChart, trendLegendLabel } from "@/components/charts/TrendChart";
import { escapeChartText, joinTooltipLines, prefersReducedMotion } from "@/components/charts/chartTheme";

describe("chartTheme helpers", () => {
  it("escapes dynamic HTML before it can enter a tooltip", () => {
    expect(escapeChartText(`Alice <img src=x onerror=alert(1)>`)).toBe(
      "Alice &lt;img src=x onerror=alert(1)&gt;",
    );
    expect(joinTooltipLines(["Alice <b>x</b>", "$1.00"])).toBe("Alice &lt;b&gt;x&lt;/b&gt;<br/>$1.00");
  });
});

describe("chart interactions", () => {
  it("highlights the Other category on hover and click", async () => {
    const items = Array.from({ length: 8 }, (_, index) => ({
      key: `k${index}`,
      label: `Category ${index + 1}`,
      amount: "10",
      shareBps: 1250,
    }));
    render(
      <CompositionChart
        title="Allocation"
        items={items}
        currency="USD"
        centerValue="80"
        centerCaption="Valued subtotal"
        ariaLabel="Allocation"
        summary="Allocation"
      />,
    );

    const other = screen.getByTestId("composition-slice-other");
    expect(other).toHaveTextContent("Other");
    await userEvent.hover(other);
    expect(other).toHaveAttribute("aria-pressed", "true");
    await userEvent.unhover(other);
    expect(other).toHaveAttribute("aria-pressed", "false");
    await userEvent.click(other);
    expect(other).toHaveAttribute("aria-pressed", "true");
  });

  it("draws a same-day single-point trend instead of hiding it as empty", () => {
    render(
      <TrendChart
        ariaLabel="Wealth"
        summary="Wealth"
        dates={["2026-08-15"]}
        series={[{ key: "netWorth", name: "Net worth", color: "#5b5bd6", values: ["100"] }]}
        currency="USD"
        emptyTitle="Not enough history to draw a trend yet."
      />,
    );
    expect(screen.queryByText("Not enough history to draw a trend yet.")).not.toBeInTheDocument();
    expect(screen.getByText("2026-08-15")).toBeInTheDocument();
  });

  it("puts source and delay status in the quote tooltip and legend", () => {
    const series = {
      key: "rate",
      name: "Rate",
      color: "#5b5bd6",
      values: ["0.14", "0.15"],
      pointMeta: [
        { sourceLabel: "Manual", delayed: false },
        { sourceLabel: "Provider · frankfurter", delayed: true },
      ],
    };
    expect(trendLegendLabel("Rate", series, "Mixed sources", "Mixed delay", "Delayed")).toBe(
      "Rate · Mixed sources · Mixed delay",
    );
    expect(joinTooltipLines([
      "2026-08-21",
      "Rate: 0.15",
      "Source: Provider · frankfurter",
      "Delayed",
    ])).toContain("Provider · frankfurter");
    expect(joinTooltipLines(["2026-08-21", "Rate: 0.15", "Source: Provider · frankfurter", "Delayed"])).toContain("<br/>");

    render(
      <TrendChart
        ariaLabel="Rate"
        summary="Rate"
        dates={["2026-08-20", "2026-08-21"]}
        series={[series]}
        currency=""
        emptyTitle="empty"
        extraTableColumns={["Quoted at", "Rate", "Source", "Delayed"]}
        extraTableRows={[
          ["2026-08-20", "0.14", "Manual", "Not delayed"],
          ["2026-08-21", "0.15", "Provider · frankfurter", "Delayed"],
        ]}
      />,
    );
    expect(screen.getByText("Provider · frankfurter")).toBeInTheDocument();
    expect(screen.getAllByText("Delayed").length).toBeGreaterThan(0);
  });

  it("renders member names through escaped HTML tooltip text", () => {
    const option = applyChartPolicy({
      tooltip: {
        trigger: "item",
        formatter: () => joinTooltipLines([`Alice <img src=x onerror=alert(1)>`, "$25.00"]),
      },
    });
    expect(option.tooltip).toEqual(expect.objectContaining({
      renderMode: "html",
      appendTo: "body",
      backgroundColor: expect.any(String),
      extraCssText: expect.stringContaining("z-index: 10000"),
    }));
    expect(joinTooltipLines([`Alice <img src=x onerror=alert(1)>`, "$25.00"])).toBe(
      "Alice &lt;img src=x onerror=alert(1)&gt;<br/>$25.00",
    );

    render(
      <SignedBarChart
        ariaLabel="Gain"
        summary="Gain"
        items={[{ key: "m1", label: `Alice <img src=x onerror=alert(1)>`, amount: "25", currency: "USD" }]}
        emptyLabel="empty"
      />,
    );
    expect(screen.getAllByText(`Alice <img src=x onerror=alert(1)>`).length).toBeGreaterThan(0);
  });

  it("disables animation when the user prefers reduced motion", () => {
    const original = window.matchMedia;
    window.matchMedia = ((query: string) => ({
      matches: query.includes("prefers-reduced-motion"),
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    })) as typeof window.matchMedia;
    expect(prefersReducedMotion()).toBe(true);
    expect(applyChartPolicy({ series: [] }).animation).toBe(false);
    window.matchMedia = original;
  });
});
