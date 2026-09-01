/**
 * Keyboard-only completion tests for onboarding, account creation, record
 * change, and settings. Every interaction below uses only
 * `userEvent.tab()`/`userEvent.keyboard()` (Tab, Space, Enter, and typed
 * characters) — never `userEvent.click()` or a programmatic `.focus()`
 * call for a step that represents user input, so these tests fail if a
 * control becomes unreachable or unactivatable without a pointer.
 *
 * The one documented exception: opening a Sheet (Base UI Dialog) moves
 * focus into the popup via its own internal focus-trap, which jsdom's
 * Base UI implementation does not simulate; each test calls `.focus()`
 * once, on the first field inside the just-opened popup, to stand in for
 * that already-tested platform behavior (components/ui/sheet.tsx), and
 * every field after that first one is reached purely by `Tab`.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { OnboardingPage } from "@/features/onboarding/OnboardingPage";
import { AccountsPage } from "@/features/accounts/AccountsPage";
import { SettingsPage } from "@/features/settings/SettingsPage";
import { HistoryPage } from "@/features/history/HistoryPage";

const completeOnboarding = vi.fn().mockResolvedValue({});
const listAccounts = vi.fn();
const createAccount = vi.fn();
const historyOrigin = vi.fn();
const listActivities = vi.fn();
const listActivityPage = vi.fn();
const previewChange = vi.fn();
const recordChange = vi.fn();
const settingsLoad = vi.fn();
const settingsSave = vi.fn().mockResolvedValue(undefined);

vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/household", () => ({
  Service: {
    CompleteOnboarding: (request: unknown) => completeOnboarding(request),
    Bootstrap: () =>
      Promise.resolve({
        household: { id: "h1", name: "Test", baseCurrency: "USD", createdAt: "", updatedAt: "" },
        members: [{ id: "alice", name: "Alice" }],
        institutions: [],
        groups: [],
      }),
  },
}));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: {
    ListAccounts: (...args: unknown[]) => listAccounts(...args),
    AccountValuations: () => Promise.resolve([]),
    CreateAccount: (...args: unknown[]) => createAccount(...args),
    UpdateAccount: vi.fn(),
    ArchiveAccount: vi.fn(),
    SetAccountIcon: vi.fn(),
  },
}));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/directory", () => ({
  Service: {
    ListMembers: () => Promise.resolve([{ id: "alice", name: "Alice" }]),
    ListInstitutions: () => Promise.resolve([]),
    ListGroups: () => Promise.resolve([]),
    CreateInstitution: vi.fn(),
  },
}));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({
  Service: {
    HistoryOrigin: () => historyOrigin(),
    StartHistory: vi.fn(),
    StartHistoryWithCosts: vi.fn(),
    StartingPointDraft: () => Promise.resolve([]),
    ListActivities: () => listActivities(),
    ListActivityPage: (...args: unknown[]) => listActivityPage(...args),
    PreviewChange: (...args: unknown[]) => previewChange(...args),
    PreviewFixChange: vi.fn(),
    RecordChange: (...args: unknown[]) => recordChange(...args),
    UndoChange: vi.fn(),
    FixChange: vi.fn(),
  },
}));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: { ListInstruments: () => Promise.resolve([]), CreateInstrument: vi.fn() },
}));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding", () => ({
  Service: { HoldingsByAccounts: () => Promise.resolve({}), CreateHolding: vi.fn(), AppendAccountCashValue: vi.fn() },
}));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: {
    Load: () => settingsLoad(),
    Save: (...args: unknown[]) => settingsSave(...args),
    Reset: vi.fn(),
    SupportedCurrencies: () => Promise.resolve(["USD", "SGD"]),
  },
}));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/app", () => ({
  Service: {
    AppInfo: () => Promise.resolve({ name: "Nestworth", appId: "com.nestworth.app", version: "v0.2.0", build: "1" }),
    Startup: () => Promise.resolve({ available: true }),
  },
}));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/data", () => ({
  Service: {
    LastBackupStatus: () => Promise.resolve({ available: false }),
    CreateBackup: () => Promise.resolve({ cancelled: true }),
    ExportCSV: () => Promise.resolve({ cancelled: true }),
    SelectCSV: () => Promise.resolve({ cancelled: true }),
  },
}));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/recovery", () => ({
  Service: {
    InspectBackup: () => Promise.resolve({ cancelled: true }),
    ConfirmRestore: () => Promise.resolve({ restartRequired: false }),
  },
}));

function renderWithQueryClient(element: React.ReactElement) {
  const queryClient = createTestQueryClient();
  return render(<QueryClientProvider client={queryClient}>{element}</QueryClientProvider>);
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
  completeOnboarding.mockClear();
  listAccounts.mockReset();
  createAccount.mockReset();
  historyOrigin.mockReset();
  listActivities.mockReset();
  listActivityPage.mockReset();
  previewChange.mockReset();
  recordChange.mockReset();
  settingsLoad.mockReset();
  settingsSave.mockClear();
  settingsLoad.mockResolvedValue(defaultSettings);
  listActivities.mockResolvedValue([]);
  listActivityPage.mockImplementation(async (...args: unknown[]) => ({ activities: await listActivities(...args) }));
});

describe("keyboard-only completion", () => {
  it("completes Onboarding using only Tab, typed characters, and Enter", async () => {
    renderWithQueryClient(<OnboardingPage />);

    const householdNameInput = await screen.findByLabelText(/household name/i);
    expect(householdNameInput).toHaveFocus(); // autoFocus, per OnboardingPage.tsx
    await userEvent.keyboard("The Tans");

    await userEvent.tab(); // -> base currency select (left at its default)
    expect(screen.getByLabelText(/base currency/i)).toHaveFocus();

    await userEvent.tab(); // -> member 1 name input
    expect(screen.getByLabelText("Member 1 name")).toHaveFocus();
    await userEvent.keyboard("Alice");

    await userEvent.tab(); // -> "+ Add" member button (left unpressed)
    expect(screen.getByRole("button", { name: /add/i })).toHaveFocus();

    await userEvent.tab(); // -> submit button
    expect(screen.getByRole("button", { name: /get started/i })).toHaveFocus();
    await userEvent.keyboard("{Enter}");

    expect(completeOnboarding).toHaveBeenCalledWith(
      expect.objectContaining({ householdName: "The Tans", memberNames: ["Alice"] }),
    );
  });

  it("completes Account creation using only Tab, Space (checkboxes), typed characters, and Enter", async () => {
    listAccounts.mockResolvedValue([]);
    createAccount.mockResolvedValue({ account: { id: "acc-1" } });
    renderWithQueryClient(<AccountsPage />);

    await screen.findByText("No accounts yet");
    await userEvent.tab(); // body -> "Add account" Sheet trigger
    expect(screen.getByRole("button", { name: "Add account" })).toHaveFocus();
    await userEvent.keyboard("{Enter}"); // opens the Sheet

    const form = await screen.findByRole("form", { name: "Create account" });
    const continueButton = within(form).getByRole("button", { name: "Continue" });
    continueButton.focus(); // stands in for the Sheet's own focus-trap (see file header)
    await userEvent.keyboard("{Enter}"); // institution -> type (bank account default)

    expect(await screen.findByTestId("account-wizard-type")).toBeInTheDocument();
    expect(within(form).getByRole("button", { name: "Continue" })).toHaveFocus();
    await userEvent.keyboard("{Enter}"); // type -> tracking question for bank accounts

    expect(await screen.findByTestId("account-wizard-tracking")).toBeInTheDocument();
    expect(within(form).getByRole("button", { name: "Continue" })).toHaveFocus();
    await userEvent.keyboard("{Enter}"); // tracking default: account total -> details

    const nameInput = within(form).getByLabelText("Name");
    expect(nameInput).toHaveFocus();
    await userEvent.keyboard("New Savings");

    await userEvent.tab(); // -> currency select
    await userEvent.tab(); // -> initial value input
    expect(within(form).getByLabelText("Initial value")).toHaveFocus();
    await userEvent.keyboard("500");

    await userEvent.tab(); // -> Alice owner checkbox
    expect(within(form).getByLabelText("Alice")).toHaveFocus();
    await userEvent.keyboard(" "); // Space toggles a focused checkbox

    await userEvent.tab(); // -> "More settings" toggle (left unpressed)
    await userEvent.tab(); // -> Back
    await userEvent.tab(); // -> Continue
    expect(within(form).getByRole("button", { name: "Continue" })).toHaveFocus();
    await userEvent.keyboard("{Enter}"); // details -> review

    expect(await screen.findByTestId("account-wizard-review")).toBeInTheDocument();
    expect(within(form).getByRole("button", { name: "Add account" })).toHaveFocus();
    await userEvent.keyboard("{Enter}");

    expect(createAccount).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "New Savings",
        accountType: "bank_account",
        trackingMode: "balance",
        ownership: [{ memberId: "alice", shareBps: 10000 }],
        initialAmount: "500",
      }),
    );
  });

  it("completes Record change (History) using only Tab, typed characters, and Enter for Preview then Confirm", async () => {
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listActivities.mockResolvedValue([]);
    previewChange.mockResolvedValue({
      activity: { id: "a1", kind: "cash_in" },
      effects: [],
      resulting: [{ target: "account_value", name: "Checking", amount: "1000", currency: "USD" }],
    });
    recordChange.mockResolvedValue({ activity: { id: "a1", kind: "cash_in" }, effects: [], resulting: [] });
    listAccounts.mockResolvedValue([{ account: { id: "acc-1", name: "Checking", trackingMode: "balance" }, ownership: [], latestValue: null }]);

    renderWithQueryClient(<HistoryPage />);

    await screen.findByRole("button", { name: /record change/i }); // wait for HistoryOrigin to load
    await userEvent.tab(); // body -> kind filter
    await userEvent.tab(); // -> account filter
    await userEvent.tab(); // -> from-date filter
    await userEvent.tab(); // -> to-date filter
    await userEvent.tab(); // -> "Record change" Sheet trigger
    expect(screen.getByRole("button", { name: /record change/i })).toHaveFocus();
    await userEvent.keyboard("{Enter}"); // opens the Sheet

    const form = await screen.findByRole("form", { name: "Record change" });
    const kindSelect = within(form).getByLabelText("Type of change");
    kindSelect.focus(); // stands in for the Sheet's own focus-trap (see file header)

    await userEvent.tab(); // -> Account select (kind left at its default: "Money added")
    expect(within(form).getByLabelText("Account")).toHaveFocus();
    await userEvent.selectOptions(within(form).getByLabelText("Account"), "acc-1");

    await userEvent.tab(); // -> Amount input
    expect(within(form).getByLabelText("Amount")).toHaveFocus();
    await userEvent.keyboard("1000");

    await userEvent.tab(); // -> currency select (left at its default: USD)
    await userEvent.tab(); // -> reason select (default: Other)
    await userEvent.tab(); // -> note input
    await userEvent.tab(); // -> effective date
    await userEvent.tab(); // -> effective time
    await userEvent.tab(); // -> Preview button
    const previewOrConfirm = within(form).getByRole("button", { name: "Preview" });
    expect(previewOrConfirm).toHaveFocus();
    await userEvent.keyboard("{Enter}"); // runs Preview

    expect(await within(form).findByRole("status")).toHaveTextContent("Checking");
    // React reconciles the Preview/Confirm buttons in place (same element
    // type and position), so the DOM node — and its focus — survives the
    // label/handler swap; Enter now activates Confirm on the same button.
    expect(within(form).getByRole("button", { name: "Confirm" })).toHaveFocus();
    await userEvent.keyboard("{Enter}");

    expect(recordChange).toHaveBeenCalledWith(expect.objectContaining({ kind: "money_added", accountId: "acc-1", amount: "1000" }));
  });

  it("completes Settings using only Tab, select changes, and Enter", async () => {
    settingsLoad.mockResolvedValue(defaultSettings);
    renderWithQueryClient(<SettingsPage />);

    const form = await screen.findByRole("form", { name: "Settings" });
    await userEvent.tab(); // body -> Appearance select (the form's first field)
    expect(within(form).getByLabelText("Appearance")).toHaveFocus();
    // jsdom does not implement a native <select>'s own ArrowDown/typeahead
    // interaction (that behavior lives entirely in each browser's form
    // control implementation, not in the DOM userEvent can drive), so
    // `selectOptions` — Testing Library's interaction-mode-agnostic API
    // for choosing an option once a <select> has focus — stands in for
    // it here, consistent with the Account/Record-change selects above.
    await userEvent.selectOptions(within(form).getByLabelText("Appearance"), "light");

    await userEvent.tab(); // -> Language select
    expect(within(form).getByLabelText("Language")).toHaveFocus();
    await userEvent.selectOptions(within(form).getByLabelText("Language"), "en");

    await userEvent.tab(); // -> Currency select (left at default)
    await userEvent.tab(); // -> Timezone combobox (left at default)
    await userEvent.tab(); // -> FX provider select (left at default)
    await userEvent.tab(); // -> Quote cache duration select (left at default)
    await userEvent.tab(); // -> Save button
    expect(within(form).getByRole("button", { name: "Save changes" })).toHaveFocus();
    await userEvent.keyboard("{Enter}");

    expect(settingsSave).toHaveBeenCalledWith(expect.objectContaining({ appearance: "light", language: "en" }));
  });
});
