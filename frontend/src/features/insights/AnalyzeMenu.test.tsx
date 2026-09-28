import { beforeEach, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NavigationContext } from "@/app/NavigationContext";
import { useAnalysisStore } from "@/stores/analysis";
import { AnalyzeMenu } from "./AnalyzeMenu";
vi.mock("@/queries/history", () => ({ useHistoryOrigin: () => ({ data: { startedAt: "2026-01-01T00:00:00Z", timezone: "UTC" } }) }));
beforeEach(() => useAnalysisStore.getState().reset());
it.each([
  { accountId: "A", instrumentId: "NVDA", scope: "instrument", scopeId: "NVDA", filters: { accountId: "A" }, cash: false },
  { accountId: undefined, instrumentId: "NVDA", scope: "instrument", scopeId: "NVDA", filters: {}, cash: false },
  { accountId: "A", instrumentId: undefined, scope: "account", scopeId: "A", filters: {}, cash: true },
])("opens the intended scope from $accountId / $instrumentId", async ({ accountId, instrumentId, scope, scopeId, filters, cash }) => {
  const open = vi.fn();
  useAnalysisStore.getState().setFilters({ from: "2026-02-01", to: "2026-02-28", moreFilters: { accountId: "B", currency: "EUR" } });
  render(<NavigationContext.Provider value={{ open, openHealth: vi.fn() }}><AnalyzeMenu accountId={accountId} instrumentId={instrumentId} label="NVDA" /></NavigationContext.Provider>);
  await userEvent.click(screen.getByRole("button", { name: "Actions for NVDA" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: "View return" }));
  expect(open).toHaveBeenCalledWith({ page: "return-analysis", tab: "trend", analysis: { scope, scopeId, includeCash: cash, valuation: "base", from: "2026-02-01", to: "2026-02-28", moreFilters: filters, returnType: "total_return" } });
});

it("offers direct account links without a selection control", async () => {
  const open = vi.fn();
  render(<NavigationContext.Provider value={{ open, openHealth: vi.fn() }}><AnalyzeMenu inline accountId="A" label="Bank" /></NavigationContext.Provider>);
  expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", {name: "Asset changes"}));
  expect(open).toHaveBeenCalledWith(expect.objectContaining({page: "asset-changes", tab: "drivers", analysis: expect.objectContaining({scope: "account", scopeId: "A", includeCash: true})}));
});

it("supports keyboard opening, Escape and focus return", async () => {
  const open = vi.fn();
  render(<NavigationContext.Provider value={{ open, openHealth: vi.fn() }}><AnalyzeMenu accountId="A" instrumentId="NVDA" label="NVDA" /></NavigationContext.Provider>);
  const trigger = screen.getByRole("button", {name: "Actions for NVDA"});
  trigger.focus();
  await userEvent.keyboard("{ArrowDown}");
  await waitFor(() => expect(screen.getByRole("menuitem", {name: "View return"})).toHaveFocus());
  await userEvent.keyboard("{Escape}");
  await waitFor(() => expect(trigger).toHaveFocus());
  expect(open).not.toHaveBeenCalled();
});
