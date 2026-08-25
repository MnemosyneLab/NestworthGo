import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import App from "./App";
import { AppProviders } from "./app/providers";
import i18n, { SUPPORTED_LANGUAGES } from "./i18n";

// The generated Wails binding calls Call.ByID under the hood, which tries
// to reach the Wails runtime bridge that does not exist in jsdom. Mock the
// service so the smoke test exercises real app-shell rendering without a
// real backend, matching this phase's "zero real pages yet" scope.
vi.mock("../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/app", () => ({
  Service: {
    AppInfo: () => Promise.resolve({ name: "Nestworth", appId: "com.nestworth.app", version: "v0.1.4", build: "1" }),
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
        accountCount: 0,
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

beforeEach(() => {
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
  it.each(SUPPORTED_LANGUAGES)("renders the app shell in %s without runtime errors", async (language) => {
    await i18n.changeLanguage(language);
    render(
      <AppProviders>
        <App />
      </AppProviders>,
    );
    // The default landing page (decision #2 in
    // docs/migration/wails-v3-navigation-decisions.md) is Overview,
    // implemented in Phase 4; it renders its net worth figure without
    // throwing.
    expect(await screen.findByTestId("overview-net-worth")).toBeInTheDocument();
    // Every nav item's label is a real, non-fallback translation (a
    // missing i18next key falls back to the raw key text, which would
    // never match a real nav item's rendered label).
    const nav = screen.getByRole("navigation", { name: "Main navigation" });
    expect(within(nav).getByText(i18n.t("nav.overview"))).toBeInTheDocument();
    expect(within(nav).getByText(i18n.t("nav.directory"))).toBeInTheDocument();

    // Every other nav destination (Phase 5's remaining work) still
    // renders its "coming soon" placeholder without throwing.
    await userEvent.click(within(nav).getByText(i18n.t("nav.settings")));
    expect(await screen.findByText(i18n.t("common.comingSoon"))).toBeInTheDocument();
  });
});
