import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { ContinuousBackupSection } from "./ContinuousBackupSection";

const api = vi.hoisted(() => ({ Status: vi.fn(), Configure: vi.fn(), TestConnection: vi.fn(), BackupNow: vi.fn(), RecoveryPoints: vi.fn(), InspectRestore: vi.fn(), ConfirmRestore: vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/continuousbackup", () => ({ Service: api }));
const initial = { enabled: false, accountID: "a".repeat(32), bucket: "test-bucket", credentialsConfigured: false, backupID: "owner", state: "disabled", streamID: "", lastAttempt: "", lastSuccessfulBackup: "", errorSummary: "", restoreState: "" };
const point = { streamID: "owner/stream", txID: "0000000000000001", capturedAt: "2026-09-30T12:00:00Z" };
const preview = { token: "candidate-token", currentHousehold: "Current family", currentCurrency: "USD", currentAccounts: 2, currentHoldings: 3, currentActivities: 4, backupHousehold: "Backup family", backupCurrency: "USD", backupAccounts: 5, backupHoldings: 6, backupActivities: 7 };
function renderSection(configured = false) {
  api.Status.mockResolvedValue({ ...initial, credentialsConfigured: configured });
  const client = createTestQueryClient({ retry: false });
  return render(<QueryClientProvider client={client}><ContinuousBackupSection /></QueryClientProvider>);
}
beforeEach(() => {
  Object.values(api).forEach((mock) => mock.mockReset());
  api.Configure.mockImplementation(async (input) => ({ ...initial, enabled: input.enabled, accountID: input.accountID, bucket: input.bucket, credentialsConfigured: !input.removeCredentials, state: input.enabled ? "preparing" : "disabled" }));
  api.TestConnection.mockResolvedValue(undefined);
  api.BackupNow.mockResolvedValue(undefined);
  api.RecoveryPoints.mockResolvedValue([point]);
  api.InspectRestore.mockResolvedValue(preview);
  api.ConfirmRestore.mockResolvedValue({ restartRequired: true });
});
describe("continuous backup settings", () => {
  it("saves a complete credential pair, clears typed secrets, and shows fixed masks", async () => {
    const user = userEvent.setup();
    renderSection();
    await user.type(await screen.findByLabelText("Access Key ID"), "new-access");
    expect(screen.getByRole("button", { name: "Save backup settings" })).toBeDisabled();
    await user.type(screen.getByLabelText("Secret Access Key"), "new-secret");
    await user.click(screen.getByLabelText("Enable continuous backup"));
    await user.click(screen.getByRole("button", { name: "Save backup settings" }));
    await waitFor(() => expect(api.Configure).toHaveBeenCalledWith({ enabled: true, accountID: initial.accountID, bucket: initial.bucket, accessKeyID: "new-access", secretAccessKey: "new-secret", removeCredentials: false }));
    await waitFor(() => expect(screen.getAllByText("••••••••")).toHaveLength(2));
    expect(screen.queryByDisplayValue("new-secret")).not.toBeInTheDocument();
    expect(screen.queryByDisplayValue("new-access")).not.toBeInTheDocument();
    await user.click(screen.getAllByRole("button", { name: "Replace" })[0]);
    expect(screen.getByLabelText("Access Key ID")).toHaveValue("");
  });
  it("preserves typed replacement inputs after failure and prevents repeated saves", async () => {
    const user = userEvent.setup();
    renderSection(true);
    await screen.findByText("Continuous backup");
    const replace = screen.getAllByRole("button", { name: "Replace" });
    await user.click(replace[0]);
    await user.click(replace[1]);
    await user.type(screen.getByLabelText("Access Key ID"), "replacement-access");
    await user.type(screen.getByLabelText("Secret Access Key"), "replacement-secret");
    let reject!: (reason: Error) => void;
    api.Configure.mockReturnValue(new Promise((_, fail) => { reject = fail; }));
    await user.click(screen.getByRole("button", { name: "Save backup settings" }));
    expect(screen.getByRole("button", { name: /Working|Saving|Please wait|Pending|…/ })).toBeDisabled();
    expect(api.Configure).toHaveBeenCalledOnce();
    reject(new Error("mock provider rejected operation"));
    await screen.findByRole("alert");
    expect(screen.getByLabelText("Access Key ID")).toHaveValue("replacement-access");
    expect(screen.getByLabelText("Secret Access Key")).toHaveValue("replacement-secret");
    expect(screen.getAllByText("••••••••")).toHaveLength(2);
    expect(screen.queryByText("mock provider rejected operation")).not.toBeInTheDocument();
  });
  it("keeps stored credentials on a nonsecret save and removes only the explicit pair", async () => {
    const user = userEvent.setup();
    renderSection(true);
    await user.clear(await screen.findByLabelText("Bucket name"));
    await user.type(screen.getByLabelText("Bucket name"), "new-bucket");
    await user.click(screen.getByRole("button", { name: "Save backup settings" }));
    await waitFor(() => expect(api.Configure).toHaveBeenCalledWith(expect.objectContaining({ bucket: "new-bucket", accessKeyID: "", secretAccessKey: "", removeCredentials: false })));
    await user.click(await screen.findByRole("button", { name: "Remove R2 credential pair" }));
    await waitFor(() => expect(api.Configure).toHaveBeenLastCalledWith(expect.objectContaining({ enabled: false, accessKeyID: "", secretAccessKey: "", removeCredentials: true })));
    await waitFor(() => expect(screen.queryByText("••••••••")).not.toBeInTheDocument());
  });
  it("does not turn a connection test into a successful backup", async () => {
    const user = userEvent.setup();
    renderSection(true);
    await user.click(await screen.findByRole("button", { name: "Test saved connection" }));
    await screen.findByText(/The bucket can be listed/);
    expect(screen.getByText("Last successful backup: None yet")).toBeVisible();
    expect(api.BackupNow).not.toHaveBeenCalled();
  });
  it("requires preview, acknowledgment and typed confirmation; cancel never installs", async () => {
    const user = userEvent.setup();
    renderSection(true);
    await user.click(await screen.findByRole("button", { name: "List recovery points" }));
    await user.selectOptions(await screen.findByLabelText("Recovery point"), `${point.streamID}:${point.txID}`);
    await user.click(screen.getByRole("button", { name: "Preview recovery" }));
    await screen.findByRole("alertdialog");
    expect(screen.getByText(/Household Backup family/)).toBeVisible();
    expect(screen.getByRole("button", { name: "Replace and quit" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(api.ConfirmRestore).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Preview recovery" }));
    await screen.findByRole("alertdialog");
    await user.click(screen.getByLabelText("I understand the current database will be replaced."));
    await user.type(screen.getByLabelText("Type RESTORE to confirm"), "WRONG");
    expect(screen.getByRole("button", { name: "Replace and quit" })).toBeDisabled();
    await user.clear(screen.getByLabelText("Type RESTORE to confirm"));
    await user.type(screen.getByLabelText("Type RESTORE to confirm"), "RESTORE");
    await user.click(screen.getByRole("button", { name: "Replace and quit" }));
    await waitFor(() => expect(api.ConfirmRestore).toHaveBeenCalledOnce());
    expect(api.ConfirmRestore).toHaveBeenCalledWith({ token: "candidate-token", confirmation: "RESTORE", acknowledged: true, restoreChrome: false, restoreFormat: false, restoreRouting: false });
  });
  it("treats empty or failed discovery as no restoration", async () => {
    const user = userEvent.setup();
    api.RecoveryPoints.mockResolvedValue([]);
    renderSection(true);
    await user.click(await screen.findByRole("button", { name: "List recovery points" }));
    await screen.findByText("No recovery points found. Nothing was restored.");
    expect(api.InspectRestore).not.toHaveBeenCalled();
    expect(api.ConfirmRestore).not.toHaveBeenCalled();
    api.RecoveryPoints.mockRejectedValue(new Error("offline"));
    await user.click(screen.getByRole("button", { name: "List recovery points" }));
    await screen.findByRole("alert");
    expect(api.ConfirmRestore).not.toHaveBeenCalled();
  });
});
