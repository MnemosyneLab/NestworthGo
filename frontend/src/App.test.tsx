import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
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
  it.each(SUPPORTED_LANGUAGES)("renders with zero real pages implemented, in %s, without runtime errors", async (language) => {
    await i18n.changeLanguage(language);
    render(
      <AppProviders>
        <App />
      </AppProviders>,
    );
    // The default landing page (decision #2 in
    // docs/migration/wails-v3-navigation-decisions.md) renders its
    // "coming soon" placeholder without throwing.
    expect(await screen.findByText(i18n.t("common.comingSoon"))).toBeInTheDocument();
    // Every nav item's label is a real, non-fallback translation (a
    // missing i18next key falls back to the raw key text, which would
    // never match a real nav item's rendered label).
    const nav = screen.getByRole("navigation", { name: "Main navigation" });
    expect(within(nav).getByText(i18n.t("nav.overview"))).toBeInTheDocument();
    expect(within(nav).getByText(i18n.t("nav.directory"))).toBeInTheDocument();
  });
});
