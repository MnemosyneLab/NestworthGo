import { expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NavigationContext } from "@/app/NavigationContext";
import { NetWorthTrendCard } from "./NetWorthTrendCard";
vi.mock("@/queries/analytics", () => ({ useNetWorthTrend: () => ({ data: {
  currency: "USD", startDate: "2026-01-01", endDate: "2026-01-03", summaryReason: "missing_boundary", complete: false,
  points: [{ localDate: "2026-01-01", complete: false }, { localDate: "2026-01-02", complete: true, netWorth: { amount: "100", currency: "USD" } }, { localDate: "2026-01-03", complete: true, netWorth: { amount: "150", currency: "USD" } }],
} }) }));
vi.mock("@/components/charts/TrendChart", () => ({ TrendChart: () => <div>Trend</div> }));
it("does not invent a change from remaining points and links the missing date", async () => {
  const open = vi.fn();
  render(<NavigationContext.Provider value={{ open, openHealth: vi.fn() }}><NetWorthTrendCard /></NavigationContext.Provider>);
  expect(screen.getByText("Net worth change: Not available")).toBeInTheDocument();
  expect(screen.queryByText("$50.00")).not.toBeInTheDocument();
  await userEvent.click(screen.getByText("1 days have incomplete data"));
  await userEvent.click(screen.getByRole("button", { name: "2026-01-01 · View gap" }));
  expect(open).toHaveBeenCalledWith({ page: "data-health", focus: { rangeStart: "2026-01-01", rangeEnd: "2026-01-01" } });
});
