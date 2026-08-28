import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { SettingsPage } from "./SettingsPage";

const load = vi.fn();
const save = vi.fn().mockResolvedValue(undefined);
const reset = vi.fn();
const historyOrigin = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: {
    Load: () => load(),
    Save: (...args: unknown[]) => save(...args),
    Reset: () => reset(),
    SupportedCurrencies: () => Promise.resolve(["USD", "SGD"]),
    FXProviders: () => Promise.resolve(["frankfurter"]),
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
  week_start: "monday",
  date_format: "iso",
  time_format: "24h",
  currency: "USD",
  decimal_separator: ".",
  grouping_separator: ",",
  decimal_places: 2,
  window_width: 1100,
  window_height: 720,
  fx_provider: "frankfurter",
};

beforeEach(() => {
  load.mockReset();
  save.mockClear();
  reset.mockReset();
  historyOrigin.mockReset();
  load.mockResolvedValue(defaultSettings);
  historyOrigin.mockResolvedValue(null);
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

  it("renders the About section from AppService.AppInfo", async () => {
    renderPage();
    expect(await screen.findByLabelText("About Nestworth")).toBeInTheDocument();
    expect(await screen.findByText(/Version v0\.2\.0/)).toBeInTheDocument();
    expect(screen.getByText(/Build 1/)).toBeInTheDocument();
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

    expect(save).toHaveBeenCalledWith(expect.objectContaining({ timezone: "Asia/Shanghai", fx_provider: "frankfurter" }));
  });

  it("notes when Settings presentation timezone differs from History Origin", async () => {
    const presentation = Intl.DateTimeFormat().resolvedOptions().timeZone;
    const origin = presentation === "UTC" ? "Asia/Tokyo" : "UTC";
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: origin });
    renderPage();
    expect(await screen.findByText(new RegExp(`History was started in ${origin}`))).toBeInTheDocument();
  });
});
