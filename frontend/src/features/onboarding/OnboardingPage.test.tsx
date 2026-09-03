import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { selectValues } from "@/test/catalog";
import i18n from "@/i18n";
import { OnboardingPage } from "./OnboardingPage";

const completeOnboarding = vi.fn().mockResolvedValue({});
const saveSettings = vi.fn().mockResolvedValue(undefined);

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
};

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/household", () => ({
  Service: { CompleteOnboarding: (request: unknown) => completeOnboarding(request) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: {
    Load: () => Promise.resolve(defaultSettings),
    Save: (value: unknown) => saveSettings(value),
    SupportedCurrencies: () => Promise.resolve(["USD", "SGD", "CNY"]),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});

function renderPage() {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <OnboardingPage />
    </QueryClientProvider>,
  );
}

beforeEach(async () => {
  completeOnboarding.mockClear();
  saveSettings.mockClear();
  await i18n.changeLanguage("en");
});

describe("OnboardingPage", () => {
  it("blocks submission and shows a validation error when the household name is empty", async () => {
    renderPage();
    await userEvent.click(screen.getByRole("button", { name: /get started/i }));
    expect(await screen.findAllByRole("alert")).not.toHaveLength(0);
    expect(completeOnboarding).not.toHaveBeenCalled();
  });

  it("submits householdName, baseCurrency, and every member name on success", async () => {
    const onCompleted = vi.fn();
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <OnboardingPage onCompleted={onCompleted} />
      </QueryClientProvider>,
    );
    const form = screen.getByRole("form", { name: "Onboarding" });
    await userEvent.type(within(form).getByLabelText(/household name/i), "The Tans");
    await userEvent.type(within(form).getByLabelText("Member 1 name"), "Alice");
    await userEvent.click(within(form).getByRole("button", { name: /add/i }));
    await userEvent.type(within(form).getByLabelText("Member 2 name"), "Bob");
    await userEvent.click(within(form).getByRole("button", { name: /get started/i }));

    expect(completeOnboarding).toHaveBeenCalledWith(
      expect.objectContaining({ householdName: "The Tans", memberNames: ["Alice", "Bob"] }),
    );
    await waitFor(() => expect(onCompleted).toHaveBeenCalledTimes(1));
  });

  it("ignores an extra empty member row on submit", async () => {
    renderPage();
    const form = screen.getByRole("form", { name: "Onboarding" });
    await userEvent.type(within(form).getByLabelText(/household name/i), "The Tans");
    await userEvent.type(within(form).getByLabelText("Member 1 name"), "Alice");
    await userEvent.click(within(form).getByRole("button", { name: /add/i }));
    expect(within(form).getByLabelText("Member 2 name")).toBeInTheDocument();
    await userEvent.click(within(form).getByRole("button", { name: /get started/i }));

    await waitFor(() =>
      expect(completeOnboarding).toHaveBeenCalledWith(
        expect.objectContaining({ householdName: "The Tans", memberNames: ["Alice"] }),
      ),
    );
  });

  it("allows removing a member row once more than one exists", async () => {
    renderPage();
    await userEvent.type(screen.getByLabelText("Member 1 name"), "Alice");
    await userEvent.click(screen.getByRole("button", { name: /add/i }));
    expect(screen.getAllByRole("button", { name: /remove member/i })).toHaveLength(2);
    await userEvent.click(screen.getAllByRole("button", { name: /remove member/i })[0]);
    expect(screen.queryAllByRole("button", { name: /remove member/i })).toHaveLength(0);
  });

  it("lets the user switch language before creating a household", async () => {
    const { TEST_CATALOG } = await import("@/test/catalog");
    renderPage();
    const languageSelect = await screen.findByLabelText("Language");
    await waitFor(() => expect(selectValues(languageSelect)).toEqual(TEST_CATALOG.languages));

    await userEvent.selectOptions(languageSelect, "zh-CN");
    expect(await screen.findByRole("heading", { level: 1, name: "开始建立家庭账本" })).toBeInTheDocument();
    await waitFor(() => expect(saveSettings).toHaveBeenCalledWith(expect.objectContaining({ language: "zh-CN" })));
  });
});
