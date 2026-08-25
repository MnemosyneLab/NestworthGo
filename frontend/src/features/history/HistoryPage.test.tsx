import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { HistoryPage } from "./HistoryPage";

const historyOrigin = vi.fn();
const startHistory = vi.fn();
const listActivities = vi.fn();
const previewChange = vi.fn();
const recordChange = vi.fn();
const undoChange = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({
  Service: {
    HistoryOrigin: () => historyOrigin(),
    StartHistory: (...args: unknown[]) => startHistory(...args),
    ListActivities: () => listActivities(),
    PreviewChange: (...args: unknown[]) => previewChange(...args),
    RecordChange: (...args: unknown[]) => recordChange(...args),
    UndoChange: (...args: unknown[]) => undoChange(...args),
    FixChange: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: {
    ListAccounts: () =>
      Promise.resolve([
        { account: { id: "acc-1", name: "Checking", trackingMode: "balance" }, ownership: [], latestValue: null },
      ]),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: { ListInstruments: () => Promise.resolve([]) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding", () => ({
  Service: { HoldingsByAccounts: () => Promise.resolve({}) },
}));

function renderPage() {
  const queryClient = new QueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <HistoryPage />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  historyOrigin.mockReset();
  startHistory.mockReset();
  listActivities.mockReset();
  previewChange.mockReset();
  recordChange.mockReset();
  undoChange.mockReset();
});

describe("HistoryPage", () => {
  it("prompts to Start History when none exists yet", async () => {
    historyOrigin.mockResolvedValue(null);
    renderPage();
    expect(await screen.findByRole("heading", { name: "Start History" })).toBeInTheDocument();
  });

  it("starts history with the given timezone", async () => {
    historyOrigin.mockResolvedValue(null);
    startHistory.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    renderPage();
    const timezoneInput = await screen.findByLabelText("Timezone");
    await userEvent.clear(timezoneInput);
    await userEvent.type(timezoneInput, "UTC");
    await userEvent.click(screen.getByRole("button", { name: "Start History" }));
    expect(startHistory).toHaveBeenCalledWith("UTC");
  });

  it("previews then records a money_added change", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listActivities.mockResolvedValue([]);
    previewChange.mockResolvedValue({
      activity: { id: "a1", kind: "cash_in", effects: [] },
      effects: [],
      resulting: [{ target: "account_value", name: "Checking", amount: "1000", currency: "USD" }],
    });
    recordChange.mockResolvedValue({ activity: { id: "a1", kind: "cash_in" }, effects: [], resulting: [] });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /record change/i }));
    const form = await screen.findByRole("form", { name: "Record change" });

    await userEvent.selectOptions(within(form).getByLabelText("Account"), "acc-1");
    await userEvent.type(within(form).getByLabelText("Amount"), "1000");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));

    expect(previewChange).toHaveBeenCalledWith(expect.objectContaining({ kind: "money_added", accountId: "acc-1", amount: "1000" }));
    expect(await within(form).findByRole("status")).toHaveTextContent("Checking");

    await userEvent.click(within(form).getByRole("button", { name: "Confirm" }));
    expect(recordChange).toHaveBeenCalledWith(expect.objectContaining({ kind: "money_added", accountId: "acc-1", amount: "1000" }));
  });

  it("lists activities and undoes one after confirmation", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listActivities.mockResolvedValue([{ id: "a1", kind: "cash_in", effectiveLocalDate: "2026-01-01" }]);
    undoChange.mockResolvedValue({ activity: { id: "a2", kind: "reversal" }, effects: [], resulting: [] });

    renderPage();
    const list = await screen.findByTestId("activity-list");
    await userEvent.click(await within(list).findByRole("button", { name: "Undo" }));
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Undo" }));
    expect(undoChange).toHaveBeenCalledWith("a1");
  });
});
