import { beforeEach, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AnalysisFilterBar } from "./AnalysisFilterBar";
import { useAnalysisStore } from "@/stores/analysis";

const { household } = vi.hoisted(() => ({ household: vi.fn() }));
vi.mock("@/queries/household", () => ({ useBootstrap: () => household() }));
vi.mock("@/queries/settings", () => ({ useSettings: () => ({ data: { currency: "CNY" } }) }));
vi.mock("@/queries/history", () => ({ useHistoryOrigin: () => ({ data: { timezone: "UTC", startedAt: "2026-09-01T00:00:00Z" } }) }));
vi.mock("@/queries/accounts", () => ({ useAccounts: () => ({ data: [{ account: { id: "a1", name: "Brokerage" } }] }) }));
vi.mock("@/queries/investments", () => ({ useInstruments: () => ({ data: [] }) }));
vi.mock("@/queries/directory", () => ({ useMembers: () => ({ data: [] }) }));
vi.mock("@/queries/catalog", () => ({ useCatalog: () => ({ data: { currencies: ["AUD", "CNY"], instrumentTypes: ["stock"] } }) }));

beforeEach(() => {
  useAnalysisStore.getState().reset();
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
  await userEvent.click(screen.getByRole("button", { name: "Reset" }));
  expect(onReset).toHaveBeenCalledOnce();
});
