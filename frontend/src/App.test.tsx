import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import App from "./App";
import { AppProviders, queryClient } from "./app/providers";
import i18n, { SUPPORTED_LANGUAGES } from "./i18n";
import { useUiStore } from "./stores/ui";

const { startup, settingsLoad, settingsSave } = vi.hoisted(() => ({
  startup: vi.fn().mockResolvedValue({ available: true }),
  settingsLoad: vi.fn(),
  settingsSave: vi.fn(),
}));

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

// The generated Wails binding calls Call.ByID under the hood, which tries
// to reach the Wails runtime bridge that does not exist in jsdom.
vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/app", () => ({
  Service: {
    AppInfo: () => Promise.resolve({ name: "Nestworth", appId: "com.nestworth.app", version: "v0.2.0", build: "1" }),
    Startup: () => startup(),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: {
    Load: () => settingsLoad(),
    Save: (...args: unknown[]) => settingsSave(...args),
    Reset: vi.fn(),
    SupportedCurrencies: () => Promise.resolve(["USD"]),
  },
}));

// This smoke test only exercises the shell + placeholder pages, so
// Bootstrap resolves with an existing Household (skipping Onboarding);
// OnboardingPage has its own dedicated tests.
vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/household", () => ({
  Service: {
    Bootstrap: () =>
      Promise.resolve({
        household: { id: "h1", name: "Test", baseCurrency: "USD", createdAt: "", updatedAt: "" },
        members: [],
        institutions: [],
        groups: [],
      }),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio", () => ({
  Service: {
    Overview: () =>
      Promise.resolve({
        currency: "USD",
        accountCount: 1,
        complete: true,
        missingInputs: [],
        assets: "0",
        liabilities: "0",
        netWorth: "0",
        byCategory: [],
        byMember: [],
        byInstitution: [],
        byGroup: [],
      }),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({
  Service: {
    HistoryOrigin: () => Promise.resolve({ id: "origin-1", timezone: "UTC" }),
    ListActivities: () => Promise.resolve([]),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: {
    ListAccounts: () => Promise.resolve([]),
    AccountValuations: () => Promise.resolve([]),
  },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: { ListInstruments: () => Promise.resolve([]) },
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding", () => ({
  Service: {},
}));

vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({
  Service: {},
}));

beforeEach(() => {
  queryClient.clear();
  startup.mockReset();
  startup.mockResolvedValue({ available: true });
  settingsLoad.mockReset();
  settingsLoad.mockResolvedValue({ ...defaultSettings });
  settingsSave.mockReset();
  settingsSave.mockResolvedValue(undefined);
  useUiStore.setState({ appearance: "system" });
  window.matchMedia =
    window.matchMedia ||
    (vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })) as unknown as typeof window.matchMedia);
});

describe("App shell smoke test", () => {
  it("hydrates saved appearance and language before rendering the workspace", async () => {
    settingsLoad.mockResolvedValue({ ...defaultSettings, appearance: "dark", language: "zh-CN" });
    render(
      <AppProviders>
        <App />
      </AppProviders>,
    );

    expect(await screen.findByTestId("overview-net-worth")).toBeInTheDocument();
    await waitFor(() => expect(document.documentElement.classList.contains("dark")).toBe(true));
    expect(i18n.language).toBe("zh-CN");
  });

  it("persists a header language change with the complete settings value", async () => {
    render(
      <AppProviders>
        <App />
      </AppProviders>,
    );
    await screen.findByTestId("overview-net-worth");

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Language" }), "zh-CN");
    await waitFor(() =>
      expect(settingsSave).toHaveBeenCalledWith({
        ...defaultSettings,
        language: "zh-CN",
      }),
    );
  });

  it("rolls back a failed header save", async () => {
    settingsSave.mockRejectedValue(new Error("save failed"));
    render(
      <AppProviders>
        <App />
      </AppProviders>,
    );
    await screen.findByTestId("overview-net-worth");

    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Language" }), "zh-CN");
    await waitFor(() => expect(i18n.language).toBe("en"));
    expect(screen.getByRole("combobox", { name: "Language" })).toHaveValue("en");
  });

  it.each(SUPPORTED_LANGUAGES)("renders the app shell in %s without runtime errors", async (language) => {
    await i18n.changeLanguage(language);
    render(
      <AppProviders>
        <App />
      </AppProviders>,
    );
    // Overview is the default landing page and renders its net worth figure
    // without throwing.
    expect(await screen.findByTestId("overview-net-worth")).toBeInTheDocument();
    // Every nav item's label is a real, non-fallback translation (a
    // missing i18next key falls back to the raw key text, which would
    // never match a real nav item's rendered label).
    const nav = screen.getByRole("navigation", { name: i18n.t("ui.navigation.main") });
    expect(within(nav).getByRole("button", { name: i18n.t("nav.overview") })).toBeInTheDocument();
    expect(within(nav).getByRole("button", { name: i18n.t("nav.directory") })).toBeInTheDocument();

    // Settings is a real surface and remains reachable from the grouped nav.
    await userEvent.click(within(nav).getByRole("button", { name: i18n.t("nav.settings") }));
    expect(await screen.findByRole("form", { name: i18n.t("settings.formLabel") })).toBeInTheDocument();
    expect(screen.getByLabelText(i18n.t("about.title"))).toBeInTheDocument();
  });

  it("renders the blocked-startup page when the database is unavailable", async () => {
    await i18n.changeLanguage("en");
    startup.mockResolvedValue({ available: false, code: "unavailable", field: "database" });
    render(
      <AppProviders>
        <App />
      </AppProviders>,
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(i18n.t("startup.blockedTitle"));
    expect(screen.queryByTestId("overview-net-worth")).not.toBeInTheDocument();
  });
});
