import { beforeEach, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AnalysisFilterBar } from "./AnalysisFilterBar";
import { useAnalysisStore } from "@/stores/analysis";

const { household, instruments } = vi.hoisted(() => ({ household: vi.fn(), instruments: vi.fn() }));
vi.mock("@/queries/household", () => ({ useBootstrap: () => household() }));
vi.mock("@/queries/settings", () => ({ useSettings: () => ({ data: { currency: "CNY" } }) }));
vi.mock("@/queries/history", () => ({ useHistoryOrigin: () => ({ data: { timezone: "UTC", startedAt: "2026-09-01T00:00:00Z" } }) }));
vi.mock("@/queries/accounts", () => ({ useAccounts: () => ({ data: [{ account: { id: "a1", name: "Brokerage" } }] }) }));
vi.mock("@/queries/investments", () => ({ useInstruments: () => instruments() }));
vi.mock("@/queries/directory", () => ({ useMembers: () => ({ data: [] }) }));
vi.mock("@/queries/catalog", () => ({ useCatalog: () => ({ data: { currencies: ["AUD", "CNY"], instrumentTypes: ["stock"] } }) }));

beforeEach(() => {
  useAnalysisStore.getState().reset();
  instruments.mockReturnValue({ data: [] });
  household.mockReturnValue({ data: { household: { baseCurrency: "AUD" } } });
});

it("uses household currency rather than the unrelated display preference", async () => {
  render(<AnalysisFilterBar session={useAnalysisStore.getState()} onChange={vi.fn()} onReset={vi.fn()} />);
  await userEvent.click(screen.getByText("More filters", { exact: false, selector: "summary" }));
  const valuation = screen.getByLabelText("Valuation");
  expect(valuation).toHaveTextContent("AUD");
  expect(valuation).not.toHaveTextContent("CNY");
});

it("does not invent a currency while household data is unavailable", async () => {
  household.mockReturnValue({ data: undefined });
  render(<AnalysisFilterBar session={useAnalysisStore.getState()} onChange={vi.fn()} onReset={vi.fn()} />);
  await userEvent.click(screen.getByText("More filters", { exact: false, selector: "summary" }));
  expect(screen.getByLabelText("Valuation")).not.toHaveTextContent(/CNY|USD/);
});

it("keeps selected filters and cash scope visible when collapsed, and preserves reset", async () => {
  useAnalysisStore.getState().setFilters({ includeCash: false, moreFilters: { accountId: "a1", currency: "AUD" } });
  const onReset = vi.fn();
  render(<AnalysisFilterBar session={useAnalysisStore.getState()} onChange={vi.fn()} onReset={onReset} />);
  const summary = screen.getByText("More filters", { exact: false, selector: "summary" });
  expect(summary).toHaveTextContent("Brokerage");
  expect(summary).toHaveTextContent("Exclude cash");
  expect(summary).toHaveTextContent("AUD");
  expect(summary.closest("details")).not.toHaveAttribute("open");
  await userEvent.click(screen.getByRole("button", { name: "Portfolio defaults" }));
  expect(onReset).toHaveBeenCalledOnce();
});

it("clears only extra filters while keeping the selected object and date", async () => {
  useAnalysisStore.getState().setFilters({ scope: "account", scopeId: "a1", from: "2026-09-02", to: "2026-09-10", moreFilters: { currency: "AUD" } });
  const clear = useAnalysisStore.getState().clearAdditionalFilters;
  const view = render(<AnalysisFilterBar session={useAnalysisStore.getState()} onChange={vi.fn()} onReset={vi.fn()} onClearAdditionalFilters={clear} />);
  await userEvent.click(screen.getByText("More filters", { exact: false, selector: "summary" }));
  await userEvent.click(screen.getByRole("button", { name: "Clear extras" }));
  expect(useAnalysisStore.getState()).toMatchObject({ scope: "account", scopeId: "a1", from: "2026-09-02", to: "2026-09-10", moreFilters: {} });
  view.rerender(<AnalysisFilterBar session={useAnalysisStore.getState()} onChange={vi.fn()} onReset={vi.fn()} onClearAdditionalFilters={clear} />);
  expect(screen.queryByRole("button", { name: "Clear extras" })).not.toBeInTheDocument();
});

it("uses the same day and week presets for return analysis", async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-08T12:00:00Z"));
  try {
    const onChange = vi.fn();
    render(<AnalysisFilterBar session={useAnalysisStore.getState()} returnPresets onChange={onChange} onReset={vi.fn()} />);
    await userEvent.click(screen.getByRole("button", { name: "Last week" }));
    expect(onChange).toHaveBeenCalledWith({ from: "2026-10-01", to: "2026-10-07" });
    expect(screen.getByRole("button", { name: "1D" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "YTD" })).not.toBeInTheDocument();
  } finally { vi.useRealTimers(); }
});

it("defaults to asset trend and resets filters without navigating away", () => {
  expect(useAnalysisStore.getState().assetTab).toBe("trend");
  useAnalysisStore.getState().setAssetView({ tab: "categories" });
  useAnalysisStore.getState().setReturnView({ tab: "trend", cursor: "2026-08" });
  useAnalysisStore.getState().setFilters({ scope: "account", scopeId: "a1", from: "2026-08-01", includeCash: false });
  useAnalysisStore.getState().resetFilters();
  expect(useAnalysisStore.getState()).toMatchObject({ assetTab: "categories", returnTab: "trend", returnCursor: "2026-08", scope: "portfolio", scopeId: "", from: "", includeCash: true });
});

it("shows the effective date range when no custom dates were selected", () => {
 render(<AnalysisFilterBar session={useAnalysisStore.getState()} resolvedRange={{ from: "2026-09-02", to: "2026-09-10" }} onChange={vi.fn()} onReset={vi.fn()} />);
 expect(screen.getByRole("button", { name: "From" })).not.toHaveTextContent("Select");
 expect(screen.getByRole("button", { name: "To" })).not.toHaveTextContent("Select");
});


it.each(["scope", "filter"] as const)("keeps full instrument identity searchable with compact %s choices", async (mode) => {
  const instrument = {
    id: "synthetic-instrument", shortName: "Family fund", symbol: "ABC",
    name: "Synthetic exceptionally long legal investment name", isin: "TEST-ISIN-123",
    marketCode: "TEST-MARKET", quoteCurrency: "USD", quantityUnit: "troy_oz",
    providerKey: "synthetic-provider", providerSymbol: "provider-code-xyz",
  };
  instruments.mockReturnValue({ data: [instrument, { id: "other", shortName: "Other", symbol: "XYZ" }] });
  const onChange = vi.fn();
  const session = { ...useAnalysisStore.getState(), scope: mode === "scope" ? "instrument" as const : "portfolio" as const };
  const view = render(<AnalysisFilterBar session={session} onChange={onChange} onReset={vi.fn()} />);
  const user = userEvent.setup();
  if (mode === "filter") await user.click(screen.getByText("More filters", { exact: false, selector: "summary" }));
  const input = screen.getByRole("combobox", { name: mode === "scope" ? "Select scope" : "Instrument" });
  for (const query of [instrument.name, "abc", "family fund", instrument.isin, instrument.providerKey, instrument.providerSymbol, instrument.marketCode, "usd", instrument.quantityUnit, instrument.id]) {
    await user.clear(input);
    await user.type(input, query);
    const option = screen.getByRole("option", { name: "Family fund · ABC" });
    expect(option).toHaveTextContent(/^Family fund · ABC$/);
    expect(within(screen.getByRole("listbox")).getAllByRole("option")).toHaveLength(1);
    await user.keyboard("{Enter}");
    expect(onChange).toHaveBeenLastCalledWith(mode === "scope" ? { scopeId: instrument.id } : { moreFilters: { instrumentId: instrument.id } });
  }
  view.rerender(<AnalysisFilterBar session={{ ...session, ...(mode === "scope" ? { scopeId: instrument.id } : { moreFilters: { instrumentId: instrument.id } }) }} onChange={onChange} onReset={vi.fn()} />);
  expect(input).toHaveValue("Family fund · ABC");
});
