import { expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NavigationContext } from "@/app/NavigationContext";
import { NetWorthTrendCard } from "./NetWorthTrendCard";
vi.mock("@/queries/history", () => ({ useHistoryOrigin: () => ({ data: { startedAt: "2026-01-01", timezone: "UTC" } }) }));
vi.mock("@/queries/settings", () => ({ useSettings: () => ({ data: { timezone: "UTC" } }) }));
vi.mock("@/queries/analytics", () => ({ useNetWorthTrend: () => ({ data: {
  currency: "USD", startDate: "2026-01-01", endDate: "2026-01-03", summaryReason: "missing_boundary", complete: false,
  points: [{ localDate: "2026-01-01", complete: false }, { localDate: "2026-01-02", complete: true, netWorth: { amount: "100", currency: "USD" } }, { localDate: "2026-01-03", complete: true, netWorth: { amount: "150", currency: "USD" } }],
} }) }));
vi.mock("@/components/charts/EChart", () => ({ EChart: ({ dataTableColumns, dataTableRows }: { dataTableColumns: string[]; dataTableRows: string[][] }) => <table><thead><tr>{dataTableColumns.map(column => <th key={column}>{column}</th>)}</tr></thead><tbody>{dataTableRows.map((row, index) => <tr key={index}>{row.map((cell, column) => <td key={column}>{cell}</td>)}</tr>)}</tbody></table> }));
it("does not invent a change from remaining points and links the missing date", async () => {
  const open = vi.fn();
  render(<NavigationContext.Provider value={{ open, openHealth: vi.fn() }}><NetWorthTrendCard /></NavigationContext.Provider>);
  expect(screen.getByText("Net worth change: Not available")).toBeInTheDocument();
  expect(screen.queryByText("$50.00")).not.toBeInTheDocument();
  await userEvent.click(screen.getByText("1 days have incomplete data"));
  await userEvent.click(screen.getByRole("button", { name: "2026-01-01 · View gap" }));
  expect(open).toHaveBeenCalledWith({ page: "data-health", focus: { rangeStart: "2026-01-01", rangeEnd: "2026-01-01" } });
});

it("shows dates and monetary values alongside coverage in the data table", () => {
  render(<NetWorthTrendCard />);
  const table = within(screen.getByRole("table"));
  expect(table.getAllByRole("columnheader").map(cell => cell.textContent)).toEqual(["Date", "Net worth", "Assets", "Liabilities", "Data coverage"]);
  expect(table.getAllByRole("row")[1]).toHaveTextContent("2026-01-01");
  expect(table.getAllByRole("row")[2]).toHaveTextContent("$100.00");
  expect(table.getAllByRole("row")[3]).toHaveTextContent("$150.00");
});
