import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import i18n from "@/i18n";
import { HistoryRetentionSection } from "./HistoryRetentionSection";

const api = vi.hoisted(() => ({ RetentionStatus: vi.fn(), ConfigureRetention: vi.fn(), PreviewRetention: vi.fn(), ExecuteRetention: vi.fn(), CancelCleanup: vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/continuousbackup", () => ({ Service: api }));
const initial = { enabled: false, running: false, days: 30, scannedAt: "", scannedBytes: 0, eligibleBytes: 0, lastAttempt: "", lastCleanup: "", result: "", errorSummary: "" };
const plan = { token: "one-use-token", scannedAt: "2026-10-01T12:00:00Z", scannedBytes: 900, eligibleBytes: 100, ready: true, items: [{ streamID: "old", sealedAt: "", bytes: 100, eligible: true, reason: "expired-sealed" }, { streamID: "current", sealedAt: "", bytes: 800, eligible: false, reason: "current" }] };
const busy = vi.fn();
function mount(enabled = false, running = false) {
  api.RetentionStatus.mockResolvedValue({ ...initial, enabled, running });
  const client = createTestQueryClient({ retry: false });
  return render(<QueryClientProvider client={client}><HistoryRetentionSection scope="test-target" disabled={false} onBusyChange={busy} /></QueryClientProvider>);
}
beforeEach(() => {
  Object.values(api).forEach((fn) => fn.mockReset());
  busy.mockReset();
  api.ConfigureRetention.mockImplementation(async (input) => { const result = { ...initial, enabled: input.enabled, days: input.days }; api.RetentionStatus.mockResolvedValue(result); return result; });
  api.PreviewRetention.mockResolvedValue(plan);
  api.ExecuteRetention.mockResolvedValue({ ...initial, enabled: true, result: "completed" });
  api.CancelCleanup.mockResolvedValue(undefined);
});
async function showPreview(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: "Preview cleanup" }));
  await screen.findByRole("list", { name: "Deletion candidates and protected streams" });
}
describe("history retention", () => {
  it("shows and cancels backend background work without a local pending mutation", async () => {
    const user = userEvent.setup(); mount(true, true);
    api.CancelCleanup.mockImplementation(async () => { api.RetentionStatus.mockResolvedValue({ ...initial, enabled: true, result: "stopped" }); });
    await screen.findByRole("button", { name: "Stop cleanup" });
    expect(screen.getByRole("button", { name: "Preview cleanup" })).toBeDisabled();
    expect(api.ExecuteRetention).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Stop cleanup" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Stop cleanup" })).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Preview cleanup" })).toBeEnabled();
    expect(api.CancelCleanup).toHaveBeenCalledOnce();
  });
  it("recovers Stop after navigation remount during manual cleanup", async () => {
    const user = userEvent.setup();
    let serverStatus = { ...initial, enabled: true };
    api.RetentionStatus.mockImplementation(async () => serverStatus);
    let reject!: (error: Error) => void;
    api.ExecuteRetention.mockImplementation(() => {
      serverStatus = { ...serverStatus, running: true, result: "running" };
      return new Promise((_, fail) => { reject = fail; });
    });
    const client = createTestQueryClient({ retry: false });
    const section = () => <QueryClientProvider client={client}><HistoryRetentionSection scope="test-target" disabled={false} onBusyChange={busy} /></QueryClientProvider>;
    const first = render(section());
    await showPreview(user);
    await user.click(screen.getByRole("button", { name: "Clean up now" }));
    await user.click(screen.getByLabelText("I understand older backup history will be permanently deleted."));
    await user.click(screen.getByRole("button", { name: "Permanently delete eligible history" }));
    expect(api.ExecuteRetention).toHaveBeenCalledOnce();
    first.unmount();
    render(section());
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    await screen.findByRole("button", { name: "Stop cleanup" });
    expect(screen.getByRole("button", { name: "Preview cleanup" })).toBeDisabled();
    api.CancelCleanup.mockImplementation(async () => {
      serverStatus = { ...serverStatus, running: false, result: "stopped" };
      reject(new Error("canceled"));
    });
    await user.click(screen.getByRole("button", { name: "Stop cleanup" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Stop cleanup" })).not.toBeInTheDocument());
    expect(api.ExecuteRetention).toHaveBeenCalledOnce();
    expect(api.CancelCleanup).toHaveBeenCalledOnce();
  });
  it("starts off, requires warning acknowledgment, and cancellation never enables cleanup", async () => {
    const user = userEvent.setup(); mount();
    const toggle = await screen.findByLabelText("Enable history cleanup");
    expect(toggle).not.toBeChecked();
    await user.click(toggle);
    const dialog = await screen.findByRole("alertdialog");
    expect(within(dialog).getByText(/Deletion is permanent/)).toBeVisible();
    expect(within(dialog).getByRole("button", { name: "Enable cleanup" })).toBeDisabled();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(api.ConfigureRetention).not.toHaveBeenCalled();
    await user.click(toggle);
    await user.click(screen.getByLabelText("I understand older backup history will be permanently deleted."));
    await user.click(screen.getByRole("button", { name: "Enable cleanup" }));
    await waitFor(() => expect(api.ConfigureRetention).toHaveBeenCalledWith({ enabled: true, days: 30, acknowledged: true }));
    await waitFor(() => expect(toggle).toBeChecked());
  });
  it("shows protected reasons and rechecks confirmation on every manual run", async () => {
    const user = userEvent.setup(); mount(true); await showPreview(user);
    expect(screen.getByText(/Protected: current stream/)).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Clean up now" }));
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(api.ExecuteRetention).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Clean up now" }));
    expect(screen.getByRole("button", { name: "Permanently delete eligible history" })).toBeDisabled();
    await user.click(screen.getByLabelText("I understand older backup history will be permanently deleted."));
    let resolve!: (value: unknown) => void;
    api.ExecuteRetention.mockReturnValue(new Promise((done) => { resolve = done; }));
    await user.dblClick(screen.getByRole("button", { name: "Permanently delete eligible history" }));
    expect(api.ExecuteRetention).toHaveBeenCalledOnce();
    expect(api.ExecuteRetention).toHaveBeenCalledWith({ token: plan.token, acknowledged: true });
    await act(async () => resolve({ ...initial, enabled: true }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Clean up now" })).toBeDisabled();
  });
  it("drops failed/stale execution tokens, hides provider messages, and requires a fresh preview", async () => {
    const user = userEvent.setup(); mount(true); await showPreview(user);
    api.ExecuteRetention.mockRejectedValue(new Error("provider-secret-response"));
    await user.click(screen.getByRole("button", { name: "Clean up now" }));
    await user.click(screen.getByLabelText("I understand older backup history will be permanently deleted."));
    await user.click(screen.getByRole("button", { name: "Permanently delete eligible history" }));
    await screen.findByRole("alert");
    expect(screen.queryByText("provider-secret-response")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Clean up now" })).toBeDisabled();
    await showPreview(user);
    expect(screen.getByRole("button", { name: "Clean up now" })).toBeEnabled();
  });
  it("cancels an interrupted preview and permits a later retry", async () => {
    const user = userEvent.setup(); mount(true);
    let reject!: (error: Error) => void;
    api.PreviewRetention.mockReturnValue(new Promise((_, fail) => { reject = fail; }));
    api.CancelCleanup.mockImplementation(async () => reject(new Error("canceled")));
    await user.click(await screen.findByRole("button", { name: "Preview cleanup" }));
    await user.click(screen.getByRole("button", { name: "Stop cleanup" }));
    await screen.findByRole("alert");
    expect(api.CancelCleanup).toHaveBeenCalledOnce();
    api.PreviewRetention.mockResolvedValue(plan);
    await showPreview(user);
    expect(api.PreviewRetention).toHaveBeenCalledTimes(2);
  });
  it("can stop manual deletion from its confirmation dialog and cannot reuse the token", async () => {
    const user = userEvent.setup(); mount(true); await showPreview(user);
    let reject!: (error: Error) => void;
    api.ExecuteRetention.mockReturnValue(new Promise((_, fail) => { reject = fail; }));
    api.CancelCleanup.mockImplementation(async () => reject(new Error("canceled")));
    await user.click(screen.getByRole("button", { name: "Clean up now" }));
    await user.click(screen.getByLabelText("I understand older backup history will be permanently deleted."));
    await user.click(screen.getByRole("button", { name: "Permanently delete eligible history" }));
    await user.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "Stop cleanup" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
    expect(api.CancelCleanup).toHaveBeenCalledOnce();
    expect(screen.getByRole("button", { name: "Clean up now" })).toBeDisabled();
    await showPreview(user);
    expect(screen.getByRole("button", { name: "Clean up now" })).toBeEnabled();
  });
  it("invalidates the displayed preview when the period changes and cannot delete without survivors", async () => {
    const user = userEvent.setup(); mount(true); await showPreview(user);
    await user.selectOptions(screen.getByLabelText("Keep sealed history for"), "90");
    expect(screen.queryByRole("list")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Clean up now" })).toBeDisabled();
    api.PreviewRetention.mockResolvedValue({ ...plan, ready: false, eligibleBytes: 0, items: [] });
    await user.click(screen.getByRole("button", { name: "Preview cleanup" }));
    await screen.findByText("Cleanup is skipped until two sealed streams can be restored and verified.");
    expect(screen.getByRole("button", { name: "Clean up now" })).toBeDisabled();
  });
  it("supports keyboard-only activation and both Chinese translations", async () => {
    const user = userEvent.setup(); mount();
    const toggle = await screen.findByLabelText("Enable history cleanup");
    toggle.focus(); await user.keyboard(" ");
    await screen.findByRole("alertdialog");
    const acknowledgment = screen.getByLabelText("I understand older backup history will be permanently deleted.");
    acknowledgment.focus(); await user.keyboard(" ");
    const confirm = screen.getByRole("button", { name: "Enable cleanup" });
    confirm.focus(); await user.keyboard("{Enter}");
    await waitFor(() => expect(api.ConfigureRetention).toHaveBeenCalledOnce());
    await act(async () => { await i18n.changeLanguage("zh-CN"); });
    expect(screen.getByRole("heading", { name: "历史保留" })).toBeVisible();
    expect(screen.getByLabelText("启用历史清理")).toBeVisible();
    await act(async () => { await i18n.changeLanguage("zh-TW"); });
    expect(screen.getByRole("heading", { name: "歷史保留" })).toBeVisible();
    expect(screen.getByLabelText("啟用歷史清理")).toBeVisible();
  });
});
