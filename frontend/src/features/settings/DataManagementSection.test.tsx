import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { DataManagementSection } from "./DataManagementSection";

const exportJSON = vi.fn();
const success = vi.fn();
const errorToast = vi.fn();
vi.mock("sonner", () => ({ toast: { success: (...args: unknown[]) => success(...args), error: (...args: unknown[]) => errorToast(...args) } }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/data", () => ({
  Service: {
    LastBackupStatus: () => Promise.resolve({ available: false }),
    CreateBackup: () => Promise.resolve({ cancelled: true }),
    ExportJSON: () => exportJSON(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/recovery", () => ({
  Service: {
    InspectBackup: () => Promise.resolve({ cancelled: true }),
    ConfirmRestore: () => Promise.resolve({ restartRequired: false }),
  },
}));
function renderSection() {
  return render(<QueryClientProvider client={createTestQueryClient({ retry: false })}><DataManagementSection /></QueryClientProvider>);
}
beforeEach(() => { exportJSON.mockReset(); success.mockReset(); errorToast.mockReset(); });
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
