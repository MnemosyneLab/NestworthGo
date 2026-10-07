import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within, waitFor, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { useUiStore } from "@/stores/ui";
import { tabbable } from "tabbable";
import i18n from "@/i18n";
import { SettingsPage } from "./SettingsPage";

const load = vi.fn();
const save = vi.fn().mockResolvedValue(undefined);
const reset = vi.fn();
const historyOrigin = vi.fn();
const tiingoKeyStatus = vi.fn();
const saveTiingoAPIKey = vi.fn();
const deleteTiingoAPIKey = vi.fn();
const inspectBackup = vi.fn();
const confirmRestore = vi.fn();
const cloud = vi.hoisted(() => ({ Status: vi.fn(), RetentionStatus: vi.fn(), Configure: vi.fn(), RecoveryPointPage: vi.fn(), InspectRestore: vi.fn(), ConfirmRestore: vi.fn(), BackupNow: vi.fn() }));
const agent = vi.hoisted(() => ({ Status: vi.fn(), RecentOperations: vi.fn(), Connection: vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/continuousbackup", () => ({ Service: cloud }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/agent", () => ({ Service: agent }));

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: {
    Load: () => load(),
    Save: (...args: unknown[]) => save(...args),
    Reset: () => reset(),
    SupportedCurrencies: () => Promise.resolve(["USD", "SGD"]),
    FXProviders: () => Promise.resolve(["frankfurter"]),
    CoinGeckoKeyStatus: () => Promise.resolve({ configured: false }),
    SaveCoinGeckoAPIKey: () => Promise.resolve({ configured: true }),
    DeleteCoinGeckoAPIKey: () => Promise.resolve({ configured: false }),
    TiingoKeyStatus: () => tiingoKeyStatus(),
    SaveTiingoAPIKey: (...args: unknown[]) => saveTiingoAPIKey(...args),
    DeleteTiingoAPIKey: () => deleteTiingoAPIKey(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/app", () => ({
  Service: {
    AppInfo: () => Promise.resolve({ name: "Nestworth", appId: "com.nestworth.app", version: "v0.2.0", build: "1" }),
    Startup: () => Promise.resolve({ available: true }),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({
  Service: {
    HistoryOrigin: () => historyOrigin(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/data", () => ({
  Service: {
    LastBackupStatus: () => Promise.resolve({ available: false }),
    CreateBackup: () => Promise.resolve({ cancelled: true }),
    ExportJSON: () => Promise.resolve({ cancelled: true }),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/recovery", () => ({
  Service: {
    InspectBackup: () => inspectBackup(),
    ConfirmRestore: (...args: unknown[]) => confirmRestore(...args),
  },
}));

function renderPage(queryClient = createTestQueryClient({ retry: false })) {
  return render(
    <QueryClientProvider client={queryClient}>
      <SettingsPage />
    </QueryClientProvider>,
  );
}

const defaultSettings = {
  schema_version: 1,
  appearance: "system",
  accent: "indigo",
  language: "en",
  timezone: "system",
  weekStart: "monday",
  dateFormat: "iso",
  timeFormat: "24h",
  currency: "USD",
  decimalSeparator: ".",
  groupingSeparator: ",",
  decimalPlaces: 2,
  windowWidth: 1100,
  windowHeight: 720,
  fxProvider: "frankfurter",
};

beforeEach(() => {
  load.mockReset();
  save.mockReset();
  save.mockImplementation(async (value) => { load.mockResolvedValue(value); });
  reset.mockReset();
  historyOrigin.mockReset();
  tiingoKeyStatus.mockReset();
  saveTiingoAPIKey.mockReset();
  deleteTiingoAPIKey.mockReset();
  load.mockResolvedValue(defaultSettings);
  inspectBackup.mockReset().mockResolvedValue({ cancelled: true });
  confirmRestore.mockReset().mockResolvedValue({ restartRequired: false });
  Object.values(cloud).forEach((mock) => mock.mockReset());
  cloud.Status.mockResolvedValue({ enabled: false, accountID: "test", bucket: "test-bucket", credentialsConfigured: false, backupID: "owner", state: "disabled" });
  cloud.RetentionStatus.mockResolvedValue({ enabled: false, days: 30, scannedBytes: 0, eligibleBytes: 0 });
  agent.Status.mockReset().mockResolvedValue({ running: true, mode: "read_only", endpoint: "http://127.0.0.1:1234/mcp", instanceId: "test" });
  agent.RecentOperations.mockReset().mockResolvedValue([]);
  agent.Connection.mockReset().mockResolvedValue({ json: '{"token":"synthetic-secret"}' });
  historyOrigin.mockResolvedValue(null);
  tiingoKeyStatus.mockResolvedValue({ configured: false });
  saveTiingoAPIKey.mockResolvedValue({ configured: true });
  deleteTiingoAPIKey.mockResolvedValue({ configured: false });
});

describe("SettingsPage", () => {
  it("saves the edited appearance and language", async () => {
    renderPage();
    await screen.findByRole("form", { name: "Settings" });
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Appearance" }), "dark");
    await userEvent.selectOptions(screen.getByLabelText("Language"), "zh-CN");
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(save).toHaveBeenCalledWith(expect.objectContaining({ appearance: "dark", language: "zh-CN" }));
  });

  it("restores defaults when Reset is clicked", async () => {
    reset.mockResolvedValue({ ...defaultSettings, appearance: "system" });
    renderPage();
    await screen.findByRole("form", { name: "Settings" });
    await userEvent.click(screen.getByRole("button", { name: "Restore all preference defaults" }));
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Restore all preference defaults" }));
    expect(reset).toHaveBeenCalled();
  });

  it("does not reset when the confirmation is canceled", async () => {
    renderPage();
    await screen.findByRole("form", { name: "Settings" });
    await userEvent.click(screen.getByRole("button", { name: "Restore all preference defaults" }));
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(reset).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByRole("button", { name: "Restore all preference defaults" })).toHaveFocus());
  });

  it("renders the Data management actions", async () => {
    renderPage();
    await screen.findByRole("form", { name: "Settings" });
    await userEvent.click(screen.getByRole("tab", { name: "Data" }));
    expect(await screen.findByRole("heading", { name: "Data management" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Back up data" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Restore from backup" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Export data" })).toBeInTheDocument();
  });

  it("renders appearance and language options from the catalog only", async () => {
    const { TEST_CATALOG, selectValues } = await import("@/test/catalog");
    renderPage();
    await screen.findByRole("form", { name: "Settings" });
    expect(selectValues(screen.getByRole("combobox", { name: "Appearance" }))).toEqual(TEST_CATALOG.appearances);
    expect(selectValues(screen.getByLabelText("Language"))).toEqual(TEST_CATALOG.languages);
  });

  it("saves the Settings timezone and FX provider fields", async () => {
    renderPage();
    await screen.findByRole("form", { name: "Settings" });
    const timezone = screen.getByRole("combobox", { name: "Timezone" });
    await userEvent.clear(timezone);
    await userEvent.type(timezone, "Asia/Shanghai");
    await userEvent.keyboard("{Enter}");
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(save).toHaveBeenCalledWith(expect.objectContaining({ timezone: "Asia/Shanghai", fxProvider: "frankfurter" }));
  });

  it("notes when Settings presentation timezone differs from History Origin", async () => {
    const presentation = Intl.DateTimeFormat().resolvedOptions().timeZone;
    const origin = presentation === "UTC" ? "Asia/Tokyo" : "UTC";
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: origin });
    renderPage();
    expect(await screen.findByText(new RegExp(`History was started in ${origin}`))).toBeInTheDocument();
  });
});


vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/household", () => ({
  Service: { Bootstrap: () => Promise.resolve({ household: { id: "h1", baseCurrency: "SGD" } }) },
}));

it("shows the actual household currency read-only and saves all general fields together", async () => {
  renderPage();
  await screen.findByRole("form", { name: "Settings" });
  const currency = screen.getByLabelText("Household base currency");
  await waitFor(() => expect(currency).toHaveValue("SGD"));
  expect(currency).toHaveAttribute("readonly");
  await userEvent.selectOptions(screen.getByRole("combobox", { name: "Appearance" }), "dark");
  const saveButton = screen.getByRole("button", { name: "Save changes" });
  await userEvent.click(saveButton);
  expect(save).toHaveBeenCalledWith(expect.objectContaining({ appearance: "dark", currency: "USD" }));
  expect(saveTiingoAPIKey).not.toHaveBeenCalled();
});

it("applies the default accent when restoring preferences", async () => {
  useUiStore.setState({ accent: "mint" });
  load.mockResolvedValue({ ...defaultSettings, accent: "mint" });
  reset.mockResolvedValue(defaultSettings);
  renderPage();
  await screen.findByRole("form", { name: "Settings" });
  await userEvent.click(screen.getByRole("button", { name: "Restore all preference defaults" }));
  const dialog = await screen.findByRole("alertdialog");
  await userEvent.click(within(dialog).getByRole("button", { name: "Restore all preference defaults" }));
  await waitFor(() => expect(useUiStore.getState().accent).toBe("indigo"));
});


it("groups provider credentials with their connection settings and keeps diagnostics separate", async () => {
  renderPage();
  await screen.findByRole("form", { name: "Settings" });
  await userEvent.click(screen.getByRole("tab", { name: "Market data" }));
  const market = screen.getByRole("region", { name: "Market data & connections" });
  await userEvent.click(screen.getByRole("tab", { name: "Diagnostics" }));
  const diagnostics = screen.getByRole("region", { name: "Diagnostics" });
  expect(within(diagnostics).getByRole("combobox")).toHaveAttribute("form", "settings-preferences");
  expect(market).not.toContainElement(within(diagnostics).getByRole("combobox"));
  expect(document.querySelector("form form")).toBeNull();
});



async function selectTab(name: string) { await userEvent.click(screen.getByRole("tab", { name })); }
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

it("preserves one preference draft across tabs and saves from About", async () => {
  renderPage();
  await screen.findByRole("combobox", { name: "Appearance" });
  await userEvent.selectOptions(screen.getByRole("combobox", { name: "Appearance" }), "dark");
  await selectTab("Market data");
  await userEvent.selectOptions(screen.getByLabelText("Quote cache duration"), "3h");
  await selectTab("Diagnostics");
  await userEvent.selectOptions(screen.getByLabelText("Application file logging"), "debug");
  await selectTab("Appearance");
  expect(screen.getByRole("combobox", { name: "Appearance" })).toHaveValue("dark");
  expect(save).not.toHaveBeenCalled();
  await selectTab("About");
  await userEvent.click(screen.getByRole("button", { name: "Save changes" }));
  expect(save).toHaveBeenCalledOnce();
  expect(save).toHaveBeenCalledWith(expect.objectContaining({ appearance: "dark", quoteCacheTTL: "3h", logLevel: "debug" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Save changes" })).toBeDisabled());
});

it("discards and resets cross-tab preferences without saving first or clearing secret drafts", async () => {
  renderPage();
  await screen.findByRole("combobox", { name: "Appearance" });
  await userEvent.selectOptions(screen.getByRole("combobox", { name: "Appearance" }), "dark");
  await selectTab("Market data");
  await userEvent.type(screen.getByLabelText("Tiingo API key"), "synthetic-key");
  await userEvent.selectOptions(screen.getByLabelText("Quote cache duration"), "1h");
  await userEvent.click(screen.getByRole("button", { name: "Discard changes" }));
  expect(screen.getByLabelText("Quote cache duration")).toHaveValue("12h");
  expect(screen.getByLabelText("Tiingo API key")).toHaveValue("synthetic-key");
  await userEvent.selectOptions(screen.getByLabelText("Quote cache duration"), "3h");
  reset.mockResolvedValue(defaultSettings);
  await userEvent.click(screen.getByRole("button", { name: "Restore all preference defaults" }));
  const dialog = await screen.findByRole("alertdialog");
  expect(within(dialog).getByText(/Unsaved preference drafts across all tabs/)).toBeVisible();
  await userEvent.click(within(dialog).getByRole("button", { name: "Restore all preference defaults" }));
  await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
  expect(reset).toHaveBeenCalledOnce();
  expect(save).not.toHaveBeenCalled();
  expect(screen.getByLabelText("Quote cache duration")).toHaveValue("12h");
  expect(screen.getByLabelText("Tiingo API key")).toHaveValue("synthetic-key");
  await selectTab("Appearance");
  expect(screen.getByRole("combobox", { name: "Appearance" })).toHaveValue("system");
});

it("remasks new provider and R2 inputs on tab exit without discarding them", async () => {
  renderPage();
  await screen.findByRole("combobox", { name: "Appearance" });
  await selectTab("Market data");
  const provider = screen.getByLabelText("Tiingo API key");
  await userEvent.type(provider, "synthetic-key");
  await userEvent.click(screen.getAllByRole("button", { name: "Show new input" })[1]);
  expect(provider).toHaveAttribute("type", "text");
  await selectTab("Data");
  await userEvent.type(await screen.findByLabelText("Access Key ID"), "synthetic-access");
  await userEvent.type(screen.getByLabelText("Secret Access Key"), "synthetic-secret");
  await userEvent.click(screen.getAllByRole("button", { name: "Show new input" })[1]);
  await selectTab("About");
  await selectTab("Data");
  expect(screen.getByLabelText("Secret Access Key")).toHaveValue("synthetic-secret");
  expect(screen.getByLabelText("Secret Access Key")).toHaveAttribute("type", "password");
  await selectTab("Market data");
  expect(screen.getByLabelText("Tiingo API key")).toHaveValue("synthetic-key");
  expect(screen.getByLabelText("Tiingo API key")).toHaveAttribute("type", "password");
});

it("collapses MCP configuration while retaining the permission draft", async () => {
  renderPage();
  await screen.findByRole("combobox", { name: "Appearance" });
  await selectTab("AI · MCP");
  await userEvent.selectOptions(await screen.findByLabelText("Agent permissions"), "directory_write");
  await userEvent.click(screen.getByRole("button", { name: "Show connection configuration" }));
  expect(await screen.findByRole("textbox", { name: "MCP connection configuration" })).toHaveValue('{"token":"synthetic-secret"}');
  await selectTab("About");
  await selectTab("AI · MCP");
  expect(screen.queryByRole("textbox", { name: "MCP connection configuration" })).not.toBeInTheDocument();
  expect(screen.getByLabelText("Agent permissions")).toHaveValue("directory_write");
});

it("locks switching through async local inspection, resets preview on cancel, and returns focus", async () => {
  const result = deferred<{ token: string }>();
  inspectBackup.mockReturnValue(result.promise);
  renderPage();
  await screen.findByRole("combobox", { name: "Appearance" });
  await selectTab("Data");
  const trigger = screen.getByRole("button", { name: "Restore from backup" });
  await userEvent.dblClick(trigger);
  expect(inspectBackup).toHaveBeenCalledOnce();
  expect(screen.getByRole("tab", { name: "About" })).toHaveAttribute("aria-disabled", "true");
  await selectTab("About");
  expect(screen.getByRole("tab", { name: "Data" })).toHaveAttribute("aria-selected", "true");
  await act(async () => result.resolve({ token: "synthetic-preview" }));
  const dialog = await screen.findByRole("alertdialog");
  await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(trigger).toHaveFocus());
  expect(confirmRestore).not.toHaveBeenCalled();
  await selectTab("About");
  expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
});

it("locks cloud inspect through its deferred result and cancels to the preview trigger", async () => {
  const point = { streamID: "synthetic-stream", txID: "1", capturedAt: "2026-10-01T12:00:00Z" };
  cloud.Status.mockResolvedValue({ enabled: true, accountID: "test", bucket: "test-bucket", credentialsConfigured: true, backupID: "owner", state: "idle" });
  cloud.RecoveryPointPage.mockResolvedValue({ points: [point], nextCursor: "" });
  const result = deferred<{ token: string }>();
  cloud.InspectRestore.mockReturnValue(result.promise);
  renderPage();
  await screen.findByRole("combobox", { name: "Appearance" });
  await selectTab("Data");
  await userEvent.click(await screen.findByRole("button", { name: "List recovery points" }));
  await userEvent.selectOptions(await screen.findByLabelText("Recovery point"), "synthetic-stream:1");
  const trigger = screen.getByRole("button", { name: "Preview recovery" });
  await userEvent.dblClick(trigger);
  expect(cloud.InspectRestore).toHaveBeenCalledOnce();
  expect(screen.getByRole("tab", { name: "About" })).toHaveAttribute("aria-disabled", "true");
  await act(async () => result.resolve({ token: "synthetic-preview" }));
  const dialog = await screen.findByRole("alertdialog");
  await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(trigger).toHaveFocus());
  await selectTab("About");
  expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  expect(cloud.ConfirmRestore).not.toHaveBeenCalled();
});

it("keeps in-progress R2 saves across switches and Enter belongs to its independent form", async () => {
  const result = deferred<object>();
  cloud.Configure.mockReturnValue(result.promise);
  renderPage();
  await screen.findByRole("combobox", { name: "Appearance" });
  await userEvent.selectOptions(screen.getByRole("combobox", { name: "Appearance" }), "dark");
  await selectTab("Data");
  await userEvent.type(await screen.findByLabelText("Bucket name"), "-changed{Enter}");
  expect(cloud.Configure).toHaveBeenCalledOnce();
  expect(save).not.toHaveBeenCalled();
  await selectTab("About");
  await selectTab("Data");
  expect(screen.getByLabelText("Bucket name")).toBeDisabled();
  expect(cloud.Configure).toHaveBeenCalledOnce();
  await act(async () => result.resolve({ enabled: false, accountID: "test", bucket: "test-bucket-changed", credentialsConfigured: false, backupID: "owner", state: "disabled" }));
  await waitFor(() => expect(screen.getByLabelText("Bucket name")).toBeEnabled());
});

it("retains ordinary backup status when switching away and returning", async () => {
  const result = deferred<void>();
  cloud.Status.mockResolvedValue({ enabled: true, accountID: "test", bucket: "test-bucket", credentialsConfigured: true, backupID: "owner", state: "idle" });
  cloud.BackupNow.mockReturnValue(result.promise);
  renderPage();
  await screen.findByRole("combobox", { name: "Appearance" });
  await selectTab("Data");
  await userEvent.dblClick(await screen.findByRole("button", { name: "Back up now" }));
  expect(cloud.BackupNow).toHaveBeenCalledOnce();
  await selectTab("About");
  await selectTab("Data");
  expect(screen.getByRole("button", { name: "Back up now" })).toBeDisabled();
  await act(async () => result.resolve());
  await waitFor(() => expect(screen.getByRole("button", { name: "Back up now" })).toBeEnabled());
});

it("prevents duplicate preference requests and disables draft edits while saving", async () => {
  const result = deferred<void>();
  save.mockReturnValue(result.promise);
  renderPage();
  await screen.findByRole("combobox", { name: "Appearance" });
  await userEvent.selectOptions(screen.getByRole("combobox", { name: "Appearance" }), "dark");
  await userEvent.dblClick(screen.getByRole("button", { name: "Save changes" }));
  expect(save).toHaveBeenCalledOnce();
  expect(screen.getByRole("combobox", { name: "Appearance" })).toBeDisabled();
  await selectTab("Market data");
  expect(screen.getByLabelText("Quote cache duration")).toBeDisabled();
  await act(async () => result.resolve());
});

it.each(["en", "zh-CN", "zh-TW"])("has six accessible tabs and hidden panels cannot receive keyboard focus in %s", async (locale) => {
  await i18n.changeLanguage(locale);
  renderPage();
  await screen.findByRole("form");
  expect(screen.getAllByRole("tab")).toHaveLength(6);
  expect(screen.getAllByRole("tabpanel")).toHaveLength(1);
  const panels = screen.getAllByRole("tabpanel", { hidden: true });
  for (const panel of panels) {
    if (panel.hidden) expect(tabbable(panel)).toHaveLength(0);
  }
  const footer = document.querySelector("footer")!;
  expect(panels.every((panel) => !panel.contains(footer))).toBe(true);
  expect(footer.className).not.toMatch(/sticky|fixed|absolute/);
  expect(document.querySelector("form form")).toBeNull();
});


it("pauses hidden MCP, backup and retention polling, then refreshes on return", async () => {
  renderPage();
  await screen.findByRole("combobox", { name: "Appearance" });
  await selectTab("AI · MCP");
  await screen.findByLabelText("Agent permissions");
  await selectTab("Data");
  await screen.findByLabelText("Bucket name");
  await waitFor(() => expect(cloud.RetentionStatus).toHaveBeenCalled());
  await selectTab("About");
  const counts = [agent.Status.mock.calls.length, agent.RecentOperations.mock.calls.length, cloud.Status.mock.calls.length, cloud.RetentionStatus.mock.calls.length];
  vi.useFakeTimers();
  try { await act(async () => { await vi.advanceTimersByTimeAsync(11000); }); }
  finally { vi.useRealTimers(); }
  expect([agent.Status.mock.calls.length, agent.RecentOperations.mock.calls.length, cloud.Status.mock.calls.length, cloud.RetentionStatus.mock.calls.length]).toEqual(counts);
  await selectTab("AI · MCP");
  await waitFor(() => expect(agent.Status.mock.calls.length).toBeGreaterThan(counts[0]));
  await selectTab("Data");
  await waitFor(() => expect(cloud.Status.mock.calls.length).toBeGreaterThan(counts[2]));
});


it("retains cloud recovery selection and unapplied retention days across tabs", async () => {
  cloud.Status.mockResolvedValue({ enabled: true, accountID: "test", bucket: "test-bucket", credentialsConfigured: true, backupID: "owner", state: "idle" });
  cloud.RecoveryPointPage.mockResolvedValue({ points: [{ streamID: "synthetic-stream", txID: "1", capturedAt: "2026-10-01T12:00:00Z" }], nextCursor: "" });
  renderPage();
  await screen.findByRole("combobox", { name: "Appearance" });
  await selectTab("Data");
  await userEvent.selectOptions(await screen.findByLabelText("Keep sealed history for"), "90");
  await userEvent.click(screen.getByRole("button", { name: "List recovery points" }));
  await userEvent.selectOptions(await screen.findByLabelText("Recovery point"), "synthetic-stream:1");
  await selectTab("About");
  await selectTab("Data");
  expect(screen.getByLabelText("Recovery point")).toHaveValue("synthetic-stream:1");
  expect(screen.getByLabelText("Keep sealed history for")).toHaveValue("90");
  expect(cloud.InspectRestore).not.toHaveBeenCalled();
});

it("can return to Data when a delayed retention response reports background cleanup while hidden", async () => {
  const result = deferred<object>();
  const running = { enabled: true, running: true, days: 30, scannedBytes: 0, eligibleBytes: 0 };
  cloud.RetentionStatus.mockReturnValueOnce(result.promise).mockResolvedValue(running);
  renderPage();
  await screen.findByRole("combobox", { name: "Appearance" });
  await selectTab("Data");
  await waitFor(() => expect(cloud.RetentionStatus).toHaveBeenCalledOnce());
  await selectTab("About");
  await act(async () => result.resolve(running));
  await waitFor(() => expect(document.querySelector("#history-retention-days")).toBeDisabled());
  expect(screen.getByRole("tab", { name: "Data" })).not.toHaveAttribute("aria-disabled", "true");
  expect(screen.getByRole("tab", { name: "About" })).toHaveAttribute("aria-selected", "true");
  await selectTab("Data");
  expect(await screen.findByRole("button", { name: "Stop cleanup" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "Preview cleanup" })).toBeDisabled();
  expect(screen.getByLabelText("Bucket name")).toBeDisabled();
  await waitFor(() => expect(cloud.RetentionStatus).toHaveBeenCalledTimes(2));
  await selectTab("About");
  expect(screen.getByRole("tab", { name: "About" })).toHaveAttribute("aria-selected", "true");
});

it("can navigate to Data when reopening Settings with cached background cleanup", async () => {
  const queryClient = createTestQueryClient({ retry: false });
  const running = { enabled: true, running: true, days: 30, scannedBytes: 0, eligibleBytes: 0 };
  queryClient.setQueryData(["continuousBackup"], { enabled: false, accountID: "test", bucket: "test-bucket", credentialsConfigured: false, backupID: "owner", state: "disabled" });
  queryClient.setQueryData(["continuousBackup", "retention", "test/test-bucket/owner"], running);
  cloud.RetentionStatus.mockResolvedValue(running);
  renderPage(queryClient);
  await screen.findByRole("combobox", { name: "Appearance" });
  await waitFor(() => expect(document.querySelector("#history-retention-days")).toBeDisabled());
  expect(cloud.RetentionStatus).not.toHaveBeenCalled();
  expect(screen.getByRole("tab", { name: "Data" })).not.toHaveAttribute("aria-disabled", "true");
  await selectTab("Data");
  expect(await screen.findByRole("button", { name: "Stop cleanup" })).toBeEnabled();
  await waitFor(() => expect(cloud.RetentionStatus).toHaveBeenCalledOnce());
  await selectTab("About");
  expect(screen.getByRole("tab", { name: "About" })).toHaveAttribute("aria-selected", "true");
});

it("locks retention confirmation but can cancel it when background cleanup starts", async () => {
  const queryClient = createTestQueryClient({ retry: false });
  cloud.Status.mockResolvedValue({ enabled: true, accountID: "test", bucket: "test-bucket", credentialsConfigured: true, backupID: "owner", state: "idle" });
  renderPage(queryClient);
  await screen.findByRole("combobox", { name: "Appearance" });
  await selectTab("Data");
  const trigger = await screen.findByLabelText("Enable history cleanup");
  const aboutTab = screen.getByRole("tab", { name: "About" });
  const dataTab = screen.getByRole("tab", { name: "Data" });
  await userEvent.click(trigger);
  const dialog = await screen.findByRole("alertdialog");
  expect(aboutTab).toHaveAttribute("aria-disabled", "true");
  await userEvent.click(aboutTab);
  expect(dataTab).toHaveAttribute("aria-selected", "true");
  await act(async () => { queryClient.setQueryData(["continuousBackup", "retention", "test/test-bucket/owner"], { enabled: false, running: true, days: 30, scannedBytes: 0, eligibleBytes: 0 }); });
  await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(document.querySelector("#history-retention-title")).toHaveFocus());
  expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  await selectTab("About");
  expect(screen.getByRole("tab", { name: "About" })).toHaveAttribute("aria-selected", "true");
});
