import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { selectValues } from "@/test/catalog";
import i18n from "@/i18n";
import { OnboardingPage } from "./OnboardingPage";
import { localDateInTimeZone } from "@/features/history/historyStartDate";

const supportedCurrencies = vi.fn();
const inspectBackup = vi.fn();
const confirmRestore = vi.fn();
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/recovery", () => ({
  Service: { InspectBackup: () => inspectBackup(), ConfirmRestore: (request: unknown) => confirmRestore(request) },
}));
const completeOnboarding = vi.fn().mockResolvedValue({});
const saveSettings = vi.fn().mockResolvedValue(undefined);

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

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/household", () => ({
  Service: { CompleteOnboarding: (request: unknown) => completeOnboarding(request) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: {
    Load: () => Promise.resolve(defaultSettings),
    Save: (value: unknown) => saveSettings(value),
    SupportedCurrencies: () => supportedCurrencies(),
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
  supportedCurrencies.mockReset().mockResolvedValue(["USD", "SGD", "CNY"]);
  inspectBackup.mockReset().mockResolvedValue({ cancelled: true });
  confirmRestore.mockReset().mockResolvedValue({ restartRequired: true });
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

  it("passes an explicitly selected past History start and timezone into onboarding", async () => {
    renderPage();
    const form = screen.getByRole("form", { name: "Onboarding" });
    await userEvent.type(within(form).getByLabelText(/household name/i), "The Tans");
    await userEvent.type(within(form).getByLabelText("Member 1 name"), "Alice");
    await userEvent.click(within(form).getByRole("checkbox", { name: "Start recording from an earlier date" }));
    const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    const today = localDateInTimeZone(timezone)!;
    const selected = `${today.slice(0, 7)}-01`;
    await userEvent.click(within(form).getByLabelText("Start date"));
    const calendar = await screen.findByRole("grid");
    await userEvent.click(calendar.querySelector(`[data-day="${selected}"] button`) as HTMLElement);
    await userEvent.click(within(form).getByRole("button", { name: /get started/i }));

    expect(completeOnboarding).toHaveBeenCalledWith(expect.objectContaining({
      householdName: "The Tans",
      memberNames: ["Alice"],
      timezone,
      historyStartDate: selected,
    }));
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

it("restores a backup without creating a household first", async () => {
  inspectBackup.mockResolvedValue({ cancelled: false, token: "preview-token", backupHousehold: "Existing family", backupCurrency: "SGD", backupAccounts: 3 });
  renderPage();
  await userEvent.click(screen.getByRole("button", { name: "Restore from backup" }));
  const dialog = await screen.findByRole("alertdialog");
  expect(within(dialog).getByText(/Existing family/)).toBeInTheDocument();
  const confirm = within(dialog).getByRole("button", { name: "Restore and quit" });
  expect(confirm).toBeDisabled();
  await userEvent.click(within(dialog).getByLabelText("I want to use the data from this backup."));
  await userEvent.type(within(dialog).getByLabelText("Type RESTORE to confirm"), "RESTORE");
  await userEvent.click(confirm);
  await waitFor(() => expect(confirmRestore).toHaveBeenCalledWith(expect.objectContaining({ token: "preview-token", confirmation: "RESTORE", acknowledged: true })));
  expect(completeOnboarding).not.toHaveBeenCalled();
  expect(await screen.findByText("Nestworth is quitting. Reopen the app to use the restored database.")).toBeInTheDocument();
});

it("keeps onboarding available when backup selection is cancelled", async () => {
  renderPage();
  await userEvent.click(screen.getByRole("button", { name: "Restore from backup" }));
  await waitFor(() => expect(inspectBackup).toHaveBeenCalledOnce());
  expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: /get started/i })).toBeEnabled();
  expect(confirmRestore).not.toHaveBeenCalled();
});


it("keeps the displayed and submitted currency aligned after delayed options", async () => {
  let resolveCurrencies!: (value: string[]) => void;
  supportedCurrencies.mockImplementation(() => new Promise((resolve) => { resolveCurrencies = resolve; }));
  renderPage();
  await waitFor(() => expect(supportedCurrencies).toHaveBeenCalled());
  expect(screen.getByRole("button", { name: /get started/i })).toBeDisabled();
  await act(async () => resolveCurrencies(["AUD", "CNY", "USD"]));
  expect(screen.getByLabelText("Base currency")).toHaveValue("CNY");
  await userEvent.type(screen.getByLabelText(/household name/i), "Synthetic");
  await userEvent.type(screen.getByLabelText("Member 1 name"), "Owner");
  await userEvent.click(screen.getByRole("button", { name: /get started/i }));
  await waitFor(() => expect(completeOnboarding).toHaveBeenCalledWith(expect.objectContaining({ baseCurrency: "CNY" })));
});


it("submits an explicitly selected nondefault currency", async () => {
  renderPage();
  await screen.findByRole("option", { name: "USD" });
  await userEvent.selectOptions(screen.getByLabelText("Base currency"), "USD");
  await userEvent.type(screen.getByLabelText(/household name/i), "Synthetic");
  await userEvent.type(screen.getByLabelText("Member 1 name"), "Owner");
  await userEvent.click(screen.getByRole("button", { name: /get started/i }));
  await waitFor(() => expect(completeOnboarding).toHaveBeenCalledWith(expect.objectContaining({ baseCurrency: "USD" })));
});

it("does not create a household when currencies could not be loaded", async () => {
  supportedCurrencies.mockRejectedValue(new Error("offline"));
  renderPage();
  await waitFor(() => expect(supportedCurrencies).toHaveBeenCalled());
  expect(screen.getByRole("button", { name: /get started/i })).toBeDisabled();
  expect(completeOnboarding).not.toHaveBeenCalled();
});
