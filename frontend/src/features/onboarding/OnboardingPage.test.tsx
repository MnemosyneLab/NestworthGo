import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { selectValues } from "@/test/catalog";
import i18n from "@/i18n";
import { OnboardingPage } from "./OnboardingPage";

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
