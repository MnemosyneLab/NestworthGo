import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { useUiStore } from "@/stores/ui";
import { SettingsPage } from "./SettingsPage";

const load = vi.fn();
const save = vi.fn().mockResolvedValue(undefined);
const reset = vi.fn();
const historyOrigin = vi.fn();
const tiingoKeyStatus = vi.fn();
const saveTiingoAPIKey = vi.fn();
const deleteTiingoAPIKey = vi.fn();
const workerTokenStatus = vi.fn();
const saveWorkerAPIToken = vi.fn();
const deleteWorkerAPIToken = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: {
    Load: () => load(),
    Save: (...args: unknown[]) => save(...args),
    Reset: () => reset(),
    SupportedCurrencies: () => Promise.resolve(["USD", "SGD"]),
    FXProviders: () => Promise.resolve(["frankfurter"]),
    TiingoKeyStatus: () => tiingoKeyStatus(),
    SaveTiingoAPIKey: (...args: unknown[]) => saveTiingoAPIKey(...args),
    DeleteTiingoAPIKey: () => deleteTiingoAPIKey(),
    WorkerTokenStatus: () => workerTokenStatus(),
    SaveWorkerAPIToken: (...args: unknown[]) => saveWorkerAPIToken(...args),
    DeleteWorkerAPIToken: () => deleteWorkerAPIToken(),
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
    ExportCSV: () => Promise.resolve({ cancelled: true }),
    SelectCSV: () => Promise.resolve({ cancelled: true }),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/recovery", () => ({
  Service: {
    InspectBackup: () => Promise.resolve({ cancelled: true }),
    ConfirmRestore: () => Promise.resolve({ restartRequired: false }),
  },
}));

function renderPage() {
  const queryClient = createTestQueryClient({ retry: false });
  return render(
    <QueryClientProvider client={queryClient}>
      <SettingsPage />
    </QueryClientProvider>,
  );
}

const defaultSettings = {
  schema_version: 1,
  appearance: "system",
  accent: "nestworth",
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
  workerBaseURL: "",
};

beforeEach(() => {
  load.mockReset();
  save.mockClear();
  reset.mockReset();
  historyOrigin.mockReset();
  tiingoKeyStatus.mockReset();
  saveTiingoAPIKey.mockReset();
  deleteTiingoAPIKey.mockReset();
  workerTokenStatus.mockReset();
  saveWorkerAPIToken.mockReset();
  deleteWorkerAPIToken.mockReset();
  load.mockResolvedValue(defaultSettings);
  historyOrigin.mockResolvedValue(null);
  tiingoKeyStatus.mockResolvedValue({ configured: false });
  saveTiingoAPIKey.mockResolvedValue({ configured: true });
  deleteTiingoAPIKey.mockResolvedValue({ configured: false });
  workerTokenStatus.mockResolvedValue({ configured: false });
  saveWorkerAPIToken.mockResolvedValue({ configured: true });
  deleteWorkerAPIToken.mockResolvedValue({ configured: false });
});

describe("SettingsPage", () => {
  it("saves the edited appearance and language", async () => {
    renderPage();
    await screen.findByRole("form", { name: "Settings" });
    await userEvent.selectOptions(screen.getByLabelText("Appearance"), "dark");
    await userEvent.selectOptions(screen.getByLabelText("Language"), "zh-CN");
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(save).toHaveBeenCalledWith(expect.objectContaining({ appearance: "dark", language: "zh-CN" }));
  });

  it("restores defaults when Reset is clicked", async () => {
    reset.mockResolvedValue({ ...defaultSettings, appearance: "system" });
    renderPage();
    await screen.findByRole("form", { name: "Settings" });
    await userEvent.click(screen.getByRole("button", { name: "Restore defaults" }));
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Restore defaults" }));
    expect(reset).toHaveBeenCalled();
  });

  it("does not reset when the confirmation is canceled", async () => {
    renderPage();
    await screen.findByRole("form", { name: "Settings" });
    await userEvent.click(screen.getByRole("button", { name: "Restore defaults" }));
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    expect(reset).not.toHaveBeenCalled();
  });

  it("renders the Data management actions", async () => {
    renderPage();
    expect(await screen.findByRole("heading", { name: "Data management" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Back up data" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Restore from backup" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Import / Export CSV" })).toBeInTheDocument();
  });

  it("renders appearance and language options from the catalog only", async () => {
    const { TEST_CATALOG, selectValues } = await import("@/test/catalog");
    renderPage();
    await screen.findByRole("form", { name: "Settings" });
    expect(selectValues(screen.getByLabelText("Appearance"))).toEqual(TEST_CATALOG.appearances);
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

  it("saves the Worker URL and token through their separate settings controls", async () => {
    renderPage();
    await screen.findByRole("form", { name: "Settings" });
    await userEvent.type(screen.getByLabelText("Nestworth Worker URL"), "https://worker.example");
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));
    expect(save).toHaveBeenCalledWith(expect.objectContaining({ workerBaseURL: "https://worker.example" }));

    await userEvent.type(screen.getByLabelText("Nestworth Worker token"), "worker-secret");
    await userEvent.click(screen.getByRole("button", { name: "Save token" }));
    expect(saveWorkerAPIToken).toHaveBeenCalledWith("worker-secret");
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
  const form = await screen.findByRole("form", { name: "Settings" });
  const currency = within(form).getByLabelText("Household base currency");
  await waitFor(() => expect(currency).toHaveValue("SGD"));
  expect(currency).toHaveAttribute("readonly");
  const url = form.querySelector<HTMLInputElement>("#settings-worker-url")!;
  const saveButton = within(form).getByRole("button", { name: "Save changes" });
  expect(url.compareDocumentPosition(saveButton) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  await userEvent.type(url, "https://quotes.example.com");
  await userEvent.click(saveButton);
  expect(save).toHaveBeenCalledWith(expect.objectContaining({ workerBaseURL: "https://quotes.example.com", currency: "USD" }));
  expect(saveTiingoAPIKey).not.toHaveBeenCalled();
  expect(saveWorkerAPIToken).not.toHaveBeenCalled();
});

it("applies the default accent when restoring preferences", async () => {
  useUiStore.setState({ accent: "ocean" });
  load.mockResolvedValue({ ...defaultSettings, accent: "ocean" });
  reset.mockResolvedValue(defaultSettings);
  renderPage();
  await screen.findByRole("form", { name: "Settings" });
  await userEvent.click(screen.getByRole("button", { name: "Restore defaults" }));
  const dialog = await screen.findByRole("alertdialog");
  await userEvent.click(within(dialog).getByRole("button", { name: "Restore defaults" }));
  await waitFor(() => expect(useUiStore.getState().accent).toBe("nestworth"));
});
