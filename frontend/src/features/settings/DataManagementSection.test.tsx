import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { DataManagementSection } from "./DataManagementSection";

const lastBackupStatus = vi.fn();
const selectCSV = vi.fn();
const previewCSV = vi.fn();
const confirmCSV = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/data", () => ({
  Service: {
    LastBackupStatus: () => lastBackupStatus(),
    CreateBackup: () => Promise.resolve({ cancelled: true }),
    ExportCSV: () => Promise.resolve({ cancelled: true }),
    SelectCSV: (...args: unknown[]) => selectCSV(...args),
    PreviewCSV: (...args: unknown[]) => previewCSV(...args),
    ConfirmCSV: (...args: unknown[]) => confirmCSV(...args),
    CommitCSV: () => Promise.resolve({ createAccounts: 0, createHoldings: 0 }),
    DownloadCSVErrors: () => Promise.resolve({ cancelled: true }),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/recovery", () => ({
  Service: {
    InspectBackup: () => Promise.resolve({ cancelled: true }),
    ConfirmRestore: () => Promise.resolve({ restartRequired: false }),
  },
}));

function renderSection() {
  const queryClient = createTestQueryClient({ retry: false });
  return render(
    <QueryClientProvider client={queryClient}>
      <DataManagementSection />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  lastBackupStatus.mockReset();
  selectCSV.mockReset();
  previewCSV.mockReset();
  confirmCSV.mockReset();
  lastBackupStatus.mockResolvedValue({ available: false });
});

describe("DataManagementSection CSV mapping", () => {
  it("keeps Accounts mapping after a Holdings file is added", async () => {
    selectCSV
      .mockResolvedValueOnce({
        cancelled: false,
        token: "csv-1",
        profile: "accounts",
        delimiter: "comma",
        headers: ["account_name", "account_type", "balance_sheet_role", "tracking_mode", "currency", "current_value", "value_date", "ownership"],
      })
      .mockResolvedValueOnce({
        cancelled: false,
        token: "csv-1",
        profile: "holdings",
        delimiter: "comma",
        headers: ["account_name", "instrument_type", "instrument_name", "quantity", "quote_currency"],
      });
    renderSection();
    await userEvent.click(await screen.findByRole("button", { name: "Import / Export CSV" }));
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: /Import · Accounts/ }));
    expect(await within(dialog).findByLabelText("account_name")).toHaveValue("account_name");
    await userEvent.click(within(dialog).getByRole("button", { name: "Holdings" }));
    expect(await within(dialog).findByLabelText("instrument_name")).toHaveValue("instrument_name");
    await userEvent.selectOptions(within(dialog).getByLabelText("Column mapping"), "accounts");
    expect(within(dialog).getByLabelText("account_name")).toHaveValue("account_name");
    expect(within(dialog).queryByLabelText("instrument_name")).not.toBeInTheDocument();
  });
});
