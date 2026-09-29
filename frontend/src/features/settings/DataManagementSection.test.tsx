import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { queryKeys } from "@/queries/keys";
import { DataManagementSection } from "./DataManagementSection";

const exportJSON = vi.fn();
const rebuildDerivedData = vi.fn();
const success = vi.fn();
const errorToast = vi.fn();
vi.mock("sonner", () => ({ toast: { success: (...args: unknown[]) => success(...args), error: (...args: unknown[]) => errorToast(...args) } }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/data", () => ({
  Service: {
    LastBackupStatus: () => Promise.resolve({ available: false }),
    CreateBackup: () => Promise.resolve({ cancelled: true }),
    ExportJSON: () => exportJSON(),
    RebuildDerivedData: () => rebuildDerivedData(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/recovery", () => ({
  Service: {
    InspectBackup: () => Promise.resolve({ cancelled: true }),
    ConfirmRestore: () => Promise.resolve({ restartRequired: false }),
  },
}));
function renderSection() {
  const client = createTestQueryClient({ retry: false });
  return { ...render(<QueryClientProvider client={client}><DataManagementSection /></QueryClientProvider>), client };
}
beforeEach(() => { exportJSON.mockReset(); rebuildDerivedData.mockReset(); success.mockReset(); errorToast.mockReset(); });
describe("Data export", () => {
  it("exports directly and prevents duplicate exports while saving", async () => {
    let finish!: (result: { fileName: string; cancelled: boolean }) => void;
    exportJSON.mockImplementation(() => new Promise((resolve) => { finish = resolve; }));
    renderSection();
    expect(screen.getByRole("button", { name: "Back up data" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Restore from backup" })).toBeInTheDocument();
    expect(screen.queryByText(/CSV/)).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Export data" }));
    expect(screen.getByRole("button", { name: "Exporting…" })).toBeDisabled();
    expect(exportJSON).toHaveBeenCalledTimes(1);
    await act(async () => finish({ cancelled: false, fileName: "Nestworth.nestworth.json" }));
    await waitFor(() => expect(success).toHaveBeenCalledWith("Exported to Nestworth.nestworth.json"));
    expect(screen.getByRole("button", { name: "Export data" })).toBeEnabled();
  });
  it("does not report success for a cancelled save", async () => {
    exportJSON.mockResolvedValue({ cancelled: true });
    renderSection();
    await userEvent.click(screen.getByRole("button", { name: "Export data" }));
    await waitFor(() => expect(exportJSON).toHaveBeenCalledTimes(1));
    expect(success).not.toHaveBeenCalled();
  });
  it("reports a failed export and permits retry", async () => {
    exportJSON.mockRejectedValue(new Error("Cannot save export"));
    renderSection();
    await userEvent.click(screen.getByRole("button", { name: "Export data" }));
    await waitFor(() => expect(errorToast).toHaveBeenCalled());
    expect(success).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Export data" })).toBeEnabled();
  });
});

describe("Derived data rebuild", () => {
  it("explains what is preserved, blocks duplicate requests, and shows the completed range and coverage", async () => {
    let finish!: (result: { from: string; to: string; snapshotDays: number; incompleteDays: number }) => void;
    rebuildDerivedData.mockImplementation(() => new Promise((resolve) => { finish = resolve; }));
    const { client } = renderSection();
    client.setQueryData(queryKeys.overview.all, { amount: "old" });
    client.setQueryData(queryKeys.history.all, { count: 1 });
    client.setQueryData(queryKeys.analysis.all, { return: "old" });

    expect(screen.getByText(/original transactions, prices, and the History start are preserved/i)).toBeInTheDocument();
    expect(screen.getByText(/will remain incomplete/i)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Recalculate all data" }));
    expect(screen.getByRole("button", { name: "Recalculating…" })).toBeDisabled();
    expect(screen.getByText(/Keep this window open/i)).toBeInTheDocument();
    expect(rebuildDerivedData).toHaveBeenCalledTimes(1);

    await act(async () => finish({ from: "2026-01-01", to: "2026-09-28", snapshotDays: 271, incompleteDays: 2 }));
    expect(await screen.findByText("Period: 2026-01-01 to 2026-09-28.")).toBeInTheDocument();
    expect(screen.getByText("271 snapshot days · 2 incomplete days")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Recalculate all data" })).toBeEnabled();
    for (const key of [queryKeys.overview.all, queryKeys.history.all, queryKeys.analysis.all]) {
      expect(client.getQueryState(key)?.isInvalidated).toBe(true);
    }
  });

  it("shows an error and permits retry", async () => {
    rebuildDerivedData.mockRejectedValueOnce(new Error(JSON.stringify({ code: "database_unavailable", message: "database unavailable" })))
      .mockResolvedValueOnce({ from: "2026-09-29", to: "2026-09-29", snapshotDays: 0, incompleteDays: 0 });
    renderSection();
    await userEvent.click(screen.getByRole("button", { name: "Recalculate all data" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("The local database could not be opened");
    await userEvent.click(screen.getByRole("button", { name: "Recalculate all data" }));
    expect(await screen.findByText("Period: 2026-09-29 to 2026-09-29.")).toBeInTheDocument();
    expect(rebuildDerivedData).toHaveBeenCalledTimes(2);
  });
});
