import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { settingsQueryKey } from "@/queries/settings";
import { HistoricalOverviewPage } from "./HistoricalOverviewPage";

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({ Service: { HistoryOrigin: vi.fn().mockResolvedValue({ startedAt: "2026-08-01T12:00:00Z", timezone: "Pacific/Kiritimati" }) } }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics", () => ({ Service: { HistoricalOverview: vi.fn().mockImplementation((date: string) => Promise.resolve({
  timezone: "Pacific/Kiritimati", originDate: "2026-08-02", lastClosedDate: "2026-10-12", rows: [], categories: [],
  left: { date, currency: "CNY", complete: true, assets: "0", liabilities: "0", netWorth: "0", knownAssets: "0", knownLiabilities: "0", byClass: [], byCurrency: [] },
})), HistoricalOverviewSeries: vi.fn().mockResolvedValue(null) } }));

it("uses the real shared picker for A/B and trend endpoints, keeps clear invalid and restores keyboard focus", async () => {
  const client = createTestQueryClient();
  client.setQueryData(settingsQueryKey, { weekStart: "monday", dateFormat: "iso" });
  const user = userEvent.setup();
  const { container } = render(<QueryClientProvider client={client}><HistoricalOverviewPage initialDate="2026-08-02" onExit={() => {}} /></QueryClientProvider>);
  const baseline = await screen.findByLabelText("Historical date");
  await waitFor(() => expect(baseline).toHaveFocus());
  expect(baseline.tagName).toBe("BUTTON");
  await user.keyboard("{Enter}");
  let grid = await screen.findByRole("grid");
  expect(grid.querySelector('[data-day="2026-08-01"] button')).toBeDisabled();
  await user.click(grid.querySelector('[data-day="2026-08-03"] button')!);
  expect(baseline).toHaveTextContent("2026-08-03");
  await waitFor(() => expect(baseline).toHaveFocus());
  await user.keyboard("{Enter}{Escape}");
  await waitFor(() => expect(screen.queryByRole("grid")).not.toBeInTheDocument());
  expect(baseline).toHaveFocus();
  await user.click(screen.getByRole("button", { name: "Compare dates" }));
  const comparison = screen.getByLabelText("Comparison date");
  expect(comparison.tagName).toBe("BUTTON");
  await user.click(comparison);
  grid = await screen.findByRole("grid");
  expect(grid).toBeInTheDocument();
  await user.keyboard("{Escape}");
  await user.click(screen.getByRole("tab", { name: "Time trend" }));
  expect(screen.getByLabelText("Trend A · First closed date").tagName).toBe("BUTTON");
  expect(screen.getByLabelText("Trend B · Last closed date").tagName).toBe("BUTTON");
  await user.click(baseline);
  await user.click(screen.getByRole("button", { name: "Clear selection" }));
  expect(baseline).toHaveAttribute("aria-invalid", "true");
  expect(container.querySelector('input[type="date"]')).toBeNull();
});
