import { formatAmount } from "@/lib/money";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import type { HistoricalOverviewResult } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import i18n from "@/i18n";
import { createTestQueryClient } from "@/test/queryClient";
import { useHistoricalOverview } from "@/queries/historicalOverview";
import { HistoricalReconciliationView } from "./HistoricalReconciliation";

vi.mock("@wailsio/runtime", () => ({ CancelError: class extends Error {}, CancelledRejectionError: class extends Error {} }));
const { read } = vi.hoisted(() => ({ read: vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics", () => ({ Service: { HistoricalOverview: read } }));
function data(total = "-1.00012", supported: string | null = "-1.0001", residual: string | null = "-0.00002"): HistoricalOverviewResult {
  const state = { date: "2026-08-01", cutoffAt: "2026-08-01T23:59:59Z", current: false, currency: "CNY", complete: true, assets: "100", liabilities: "0", netWorth: "100", knownAssets: "100", knownLiabilities: "0", byClass: [], byCurrency: [] };
  return { left: state, right: { ...state, date: "2026-08-02" }, change: { assets: total, liabilities: "0", netWorth: total }, categories: [], rows: [], timezone: "UTC", originDate: "2026-08-01", lastClosedDate: "2026-08-02", capturedAt: "2026-08-03T12:00:00Z", inputGeneration: 1, resolverPolicy: "policy", reconciliation: { status: "partial", currency: "CNY", basis: "endpoint_bridge/1", total, supported, residual, contributions: [{ key: "fee", amount: "-1.0001" }], reasons: ["unexplained_movement"] } };
}

it.each(["en", "zh-CN", "zh-TW"])("shows exact amounts and keyboard-accessible localized explanation (%s)", async locale => {
  await i18n.changeLanguage(locale);
  render(<HistoricalReconciliationView data={data()} />);
  const section = screen.getByRole("region", { name: i18n.t("historicalReconciliation.title") });
  expect(section).toHaveTextContent(formatAmount("-1.00012", "CNY")); expect(section).toHaveTextContent(formatAmount("-0.00002", "CNY"));
  const disclosure = within(section).getByText(i18n.t("historicalReconciliation.explanation"));
  disclosure.focus(); await userEvent.keyboard("{Enter}");
  expect(within(section).getByRole("table")).toBeVisible();
  expect(within(section).getByText(i18n.t("historicalReconciliation.category.fee"))).toBeVisible();
  expect(within(section).getByText(i18n.t("historicalReconciliation.reason.unexplained_movement"))).toBeVisible();
  expect(section).not.toHaveTextContent("historicalReconciliation.");
  await userEvent.keyboard(" "); expect(disclosure).toHaveAttribute("aria-expanded", "false");
});
it("keeps unknown residual distinct from zero, and zero uncertainty visible", async () => {
  await i18n.changeLanguage("en");
  const unknown = data("1", null, null); unknown.reconciliation!.status = "unavailable"; unknown.reconciliation!.reasons = ["current_endpoint"]; unknown.reconciliation!.contributions = [];
  const view = render(<HistoricalReconciliationView data={unknown} />);
  expect(screen.getAllByText(i18n.t("historicalOverview.unknown"))).toHaveLength(3);
  expect(screen.getByRole("status")).toHaveTextContent("unavailable");
  const zero = data("0", "0", "0"); zero.reconciliation!.reasons = ["quantity_activity"];
  view.rerender(<HistoricalReconciliationView data={zero} />);
  expect(screen.queryByText(i18n.t("historicalOverview.unknown"))).not.toBeInTheDocument();
  expect(screen.getByRole("status")).toHaveTextContent("partial");
  await userEvent.click(screen.getByText("How these amounts reconcile"));
  expect(screen.getByText(i18n.t("historicalReconciliation.reason.quantity_activity"))).toBeVisible();
});
it("publishes the reconciliation from the final rapid date selection and ignores late replies", async () => {
  await i18n.changeLanguage("en"); read.mockReset();
  function deferred() {
    let resolve!: (value: HistoricalOverviewResult) => void;
    const promise = Object.assign(new Promise<HistoricalOverviewResult>(yes => { resolve = yes; }), { cancel: vi.fn() });
    return { resolve, promise };
  }
  const calls = Array.from({ length: 4 }, deferred); calls.forEach(call => read.mockReturnValueOnce(call.promise));
  const client = createTestQueryClient();
  function Harness({ date }: { date: string }) {
    const query = useHistoricalOverview(date, "2026-08-09", true);
    return query.data ? <HistoricalReconciliationView data={query.data} /> : <p>Loading fixture</p>;
  }
  const view = render(<QueryClientProvider client={client}><Harness date="2026-08-01" /></QueryClientProvider>);
  await waitFor(() => expect(read).toHaveBeenCalledTimes(1));
  for (let index = 1; index < calls.length; index++) {
    view.rerender(<QueryClientProvider client={client}><Harness date={`2026-08-0${index + 1}`} /></QueryClientProvider>);
    await waitFor(() => expect(read).toHaveBeenCalledTimes(index + 1));
    expect(calls[index - 1].promise.cancel).toHaveBeenCalledOnce();
  }
  await act(async () => calls[3].resolve(data("3", "2", "1")));
  await waitFor(() => expect(screen.getByRole("region")).toHaveTextContent(("+" + formatAmount("3", "CNY"))));
  await act(async () => calls.slice(0, 3).forEach(call => call.resolve(data("999", "999", "0"))));
  expect(screen.getByRole("region")).toHaveTextContent(("+" + formatAmount("3", "CNY"))); expect(screen.queryByText(/999/)).not.toBeInTheDocument();
  view.unmount(); client.clear();
});
