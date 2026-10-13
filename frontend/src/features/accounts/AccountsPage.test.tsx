import { describe, expect, it, vi, beforeEach } from "vitest";
import { act, fireEvent, render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { queryKeys } from "@/queries/keys";
import { AccountsPage } from "./AccountsPage";
import i18n from "@/i18n";
import { localDateInTimeZone } from "@/features/history/historyStartDate";

const listAccounts = vi.fn();
const accountValuations = vi.fn();
const createAccount = vi.fn();
const updateAccount = vi.fn();
const archiveAccount = vi.fn().mockResolvedValue(undefined);
const historyOrigin = vi.fn();
const startHistory = vi.fn();
const startHistoryOnDate = vi.fn();
const listInstruments = vi.fn();
const createInstrument = vi.fn();
const holdingsByAccounts = vi.fn();
const createHolding = vi.fn();
const appendCash = vi.fn();
const previewCash = vi.fn();
const confirmCash = vi.fn();
const appendValue = vi.fn();
const previewChange = vi.fn();
const recordChange = vi.fn();
const settingsLoad = vi.fn();
const currentInstrumentQuote = vi.fn();
const listInstitutions = vi.fn();
const listGroups = vi.fn();
const listAccountCashValues = vi.fn();
const historyMutationAllowed = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: {
    ListAccounts: (...args: unknown[]) => listAccounts(...args),
    AccountValuations: (...args: unknown[]) => accountValuations(...args),
    CreateAccount: (...args: unknown[]) => createAccount(...args),
    UpdateAccount: (...args: unknown[]) => updateAccount(...args),
    ArchiveAccount: (...args: unknown[]) => archiveAccount(...args),
    AppendAccountValue: (...args: unknown[]) => appendValue(...args),
    ListAccountCashValues: (...args: unknown[]) => listAccountCashValues(...args),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/directory", () => ({
  Service: {
    ListMembers: () => Promise.resolve([{ id: "alice", name: "Alice" }, { id: "bob", name: "Bob" }]),
    ListInstitutions: (...args: unknown[]) => listInstitutions(...args),
    ListGroups: (...args: unknown[]) => listGroups(...args),
    CreateInstitution: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/household", () => ({
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
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: {
    Load: () => settingsLoad(),
    SupportedCurrencies: () => Promise.resolve(["USD", "SGD", "CNY"]),
    FXProviders: () => Promise.resolve(["frankfurter"]),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({
  Service: {
    HistoryOrigin: () => historyOrigin(),
    StartHistory: (...args: unknown[]) => startHistory(...args),
    StartHistoryOnDate: (...args: unknown[]) => startHistoryOnDate(...args),
    StartHistoryWithCosts: vi.fn(),
    StartingPointDraft: () => Promise.resolve([]),
    PreviewChange: (...args: unknown[]) => previewChange(...args),
    PreviewFixChange: vi.fn(),
    RecordChange: (...args: unknown[]) => recordChange(...args),
    HistoryMutationAllowed: () => historyMutationAllowed(),
    UndoChange: vi.fn(),
    FixChange: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: {
    ListInstruments: () => listInstruments(),
    CreateInstrument: (...args: unknown[]) => createInstrument(...args),
    ArchiveInstrument: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding", () => ({
  Service: {
    HoldingsByAccounts: (...args: unknown[]) => holdingsByAccounts(...args),
    ListAccountCashValues: (...args: unknown[]) => listAccountCashValues(...args),
    CreateHolding: (...args: unknown[]) => createHolding(...args),
    PreviewCashBalance: (...args: unknown[]) => previewCash(...args),
    ConfirmCashBalance: (...args: unknown[]) => confirmCash(...args),
    AppendAccountCashValue: (...args: unknown[]) => appendCash(...args),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({
  Service: {
    CurrentInstrumentQuote: (...args: unknown[]) => currentInstrumentQuote(...args),
    CurrentFXQuote: () => Promise.resolve(null),
    ListFXPreferences: () => Promise.resolve([]),
    SetFXPreference: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata", () => ({
  Service: {
    SearchInstruments: vi.fn().mockResolvedValue([]),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({
  Service: {
    Overview: () => Promise.resolve({ asOf: "", localDate: "2026-09-20", timezone: "UTC", baseCurrency: "USD", assumptions: [], buckets: [], sources: [], unresolvedReservations: [] }),
    ListProducts: () => Promise.resolve([]),
    Product: vi.fn(),
    ListOperations: vi.fn(),
    PreviewProductOperation: vi.fn(),
    RecordProductOperation: vi.fn(),
  },
}));

function renderPage() {
  const queryClient = createTestQueryClient();
  const rendered = render(
    <QueryClientProvider client={queryClient}>
      <AccountsPage />
    </QueryClientProvider>,
  );
  return { ...rendered, queryClient };
}

const emptyAccount = {
  account: {
    id: "acc-1",
    name: "Checking",
    accountType: "bank_account",
    balanceSheetRole: "asset",
    trackingMode: "balance",
    defaultCurrency: "USD",
    sortOrder: 0,
    includeInNetWorth: true,
    includeInPortfolio: false,
    includeInLiquidAssets: false,
    createdAt: "",
    updatedAt: "",
    institutionId: "cmb",
  },
  ownership: [{ memberId: "alice", shareBps: 10000 }],
  latestValue: {
    id: "v1",
    accountId: "acc-1",
    valueKind: "balance",
    amount: { amount: "1000", currency: "USD" },
    effectiveAt: "",
    createdAt: "",
  },
  institutionName: "China Merchants Bank",
};

const brokerageAccount = {
  account: {
    ...emptyAccount.account,
    id: "brk-1",
    name: "MooMoo",
    accountType: "brokerage",
    trackingMode: "holdings",
    includeInPortfolio: true,
    institutionId: "cmb",
  },
  ownership: emptyAccount.ownership,
  institutionName: "China Merchants Bank",
};

async function continueWizard(form: HTMLElement, times: number) {
  for (let index = 0; index < times; index += 1) {
    await userEvent.click(within(form).getByRole("button", { name: "Continue" }));
  }
}

beforeEach(() => {
  listAccounts.mockReset();
  accountValuations.mockReset();
  createAccount.mockReset();
  updateAccount.mockReset();
  archiveAccount.mockClear();
  historyOrigin.mockReset();
  startHistory.mockReset();
  startHistoryOnDate.mockReset();
  listInstruments.mockReset();
  createInstrument.mockReset();
  holdingsByAccounts.mockReset();
  createHolding.mockReset();
  previewCash.mockReset();
  previewCash.mockImplementation(async (_account, amount, currency, reason) => ({ planId: "cash-plan", current: "800", target: amount, delta: "100", currency, effectiveAt: "2026-10-11T12:00:00Z", reason }));
  confirmCash.mockReset(); confirmCash.mockResolvedValue({});
  appendCash.mockReset();
  appendValue.mockReset();
  previewChange.mockReset();
  recordChange.mockReset();
  settingsLoad.mockReset();
  currentInstrumentQuote.mockReset();
  listAccountCashValues.mockReset();
  historyMutationAllowed.mockReset();
  listInstitutions.mockReset();
  listGroups.mockReset();
  listAccounts.mockResolvedValue([emptyAccount]);
  accountValuations.mockResolvedValue([
    {
      account: emptyAccount.account,
      ownership: emptyAccount.ownership,
      complete: true,
      components: [],
      missingInputs: [],
      baseValue: { amount: "1000", currency: "USD" },
    },
  ]);
  createAccount.mockResolvedValue(emptyAccount);
  updateAccount.mockResolvedValue(emptyAccount);
  historyOrigin.mockResolvedValue(null);
  startHistory.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
  startHistoryOnDate.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
  settingsLoad.mockResolvedValue({ timezone: "system", fxProvider: "frankfurter" });
  listInstruments.mockResolvedValue([]);
  holdingsByAccounts.mockResolvedValue({});
  createHolding.mockResolvedValue({ id: "h1", accountId: "brk-1", instrumentId: "i1", quantity: "1" });
  appendCash.mockResolvedValue(undefined);
  appendValue.mockResolvedValue(undefined);
  currentInstrumentQuote.mockResolvedValue(null);
  listAccountCashValues.mockResolvedValue([]);
  historyMutationAllowed.mockResolvedValue({});
  listInstitutions.mockResolvedValue([
    { id: "cmb", name: "China Merchants Bank", iconKey: "bank", institutionType: "bank", sortOrder: 0, archivedAt: null },
  ]);
  listGroups.mockResolvedValue([]);
});

describe("AccountsPage", () => {
  it("renders the empty state when there are no accounts", async () => {
    listAccounts.mockResolvedValue([]);
    renderPage();
    expect(await screen.findByText("No accounts yet")).toBeInTheDocument();
  });

  it("lists existing accounts with their current value, grouped by institution", async () => {
    renderPage();
    expect(await screen.findByText("Checking")).toBeInTheDocument();
    expect(screen.getByText("China Merchants Bank")).toBeInTheDocument();
    expect(screen.getAllByText("$1,000.00").length).toBeGreaterThan(0);
    expect(screen.getByText("Bank account")).toBeInTheDocument();
    expect(screen.getByText("Complete")).toBeInTheDocument();
  });

  it("lists larger institution groups and accounts first", async () => {
    const smaller = {
      account: { ...emptyAccount.account, id: "acc-small", name: "Small Checking", institutionId: "alpha" },
      ownership: emptyAccount.ownership,
      latestValue: emptyAccount.latestValue,
      institutionName: "Alpha Bank",
    };
    const larger = {
      account: { ...emptyAccount.account, id: "acc-large", name: "Large Checking", institutionId: "zeta" },
      ownership: emptyAccount.ownership,
      latestValue: emptyAccount.latestValue,
      institutionName: "Zeta Bank",
    };
    const extra = {
      account: { ...emptyAccount.account, id: "acc-extra", name: "Extra Checking", institutionId: "zeta" },
      ownership: emptyAccount.ownership,
      latestValue: emptyAccount.latestValue,
      institutionName: "Zeta Bank",
    };
    listAccounts.mockResolvedValue([smaller, extra, larger]);
    accountValuations.mockResolvedValue([
      { account: smaller.account, ownership: smaller.ownership, complete: true, components: [], missingInputs: [], baseValue: { amount: "100", currency: "USD" } },
      { account: extra.account, ownership: extra.ownership, complete: true, components: [], missingInputs: [], baseValue: { amount: "200", currency: "USD" } },
      { account: larger.account, ownership: larger.ownership, complete: true, components: [], missingInputs: [], baseValue: { amount: "800", currency: "USD" } },
    ]);
    renderPage();
    const headings = (await screen.findAllByRole("heading", { level: 2 })).map((node) => node.textContent);
    expect(headings[0]).toBe("Zeta Bank");
    expect(headings[1]).toBe("Alpha Bank");
    const zetaSection = screen.getByRole("heading", { name: "Zeta Bank" }).closest("section");
    expect(zetaSection?.textContent?.indexOf("Large Checking")).toBeLessThan(zetaSection?.textContent?.indexOf("Extra Checking") ?? 0);
  });

  it("does not label a still-loading valuation as complete", async () => {
    accountValuations.mockReturnValue(new Promise(() => {}));
    renderPage();
    expect(await screen.findByText("Checking")).toBeInTheDocument();
    expect(screen.getByText("Loading")).toBeInTheDocument();
    expect(screen.queryByText("Complete")).not.toBeInTheDocument();
    expect(screen.getByText("$1,000.00")).toBeInTheDocument();
  });

  it("does not label a missing valuation as complete", async () => {
    accountValuations.mockResolvedValue([]);
    renderPage();
    expect(await screen.findByText("Checking")).toBeInTheDocument();
    expect(screen.getByText("$1,000.00")).toBeInTheDocument();
    expect(screen.queryByText("Pending conversion")).not.toBeInTheDocument();
    expect(screen.getAllByText("No current value").length).toBeGreaterThan(0);
    expect(screen.queryByText("Complete")).not.toBeInTheDocument();
  });

  it("shows computed valuation for holdings accounts without a stored latestValue", async () => {
    listAccounts.mockResolvedValue([brokerageAccount]);
    accountValuations.mockResolvedValue([
      {
        account: brokerageAccount.account,
        ownership: brokerageAccount.ownership,
        complete: true,
        components: [{ nativeCurrency: "USD", nativeAmount: "3542.10", available: true }],
        missingInputs: [],
        baseValue: { amount: "35706.45", currency: "CNY" },
      },
    ]);
    renderPage();
    expect(await screen.findByText("MooMoo")).toBeInTheDocument();
    expect(screen.getByText(/35,706\.45/)).toBeInTheDocument();
    expect(screen.queryByText(/3,542\.10/)).not.toBeInTheDocument();
  });

  it("creates a bank account through the wizard with equal ownership", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await continueWizard(form, 2);
    expect(within(form).getByText("How should this account be recorded?")).toBeInTheDocument();
    expect(within(form).getByText("Keep one account balance")).toBeInTheDocument();
    await continueWizard(form, 1);
    expect(within(form).getByText("Recording: Keep one account balance")).toBeInTheDocument();
    await userEvent.type(within(form).getByLabelText("Name"), "New Savings");
    await userEvent.click(within(form).getByLabelText("Alice"));
    await userEvent.click(within(form).getByLabelText("Bob"));
    await userEvent.type(within(form).getByLabelText("Initial value"), "500");
    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("button", { name: "Add account" }));

    expect(createAccount).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "New Savings",
        iconKey: "",
        accountType: "bank_account",
        trackingMode: "balance",
        ownership: [
          { memberId: "alice", shareBps: 5000 },
          { memberId: "bob", shareBps: 5000 },
        ],
        initialAmount: "500",
      }),
    );
  });

  it("selects and clears an optional opening date in the account wizard", async () => {
    const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone, startedAt: "2020-01-01T00:00:00Z" });
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await continueWizard(form, 3);
    await userEvent.type(within(form).getByLabelText("Name"), "Dated Savings");
    await userEvent.click(within(form).getByLabelText("Alice"));

    const openedOn = within(form).getByLabelText("Opened on");
    expect(openedOn).toHaveTextContent("Select a date");
    const today = localDateInTimeZone(timezone)!;
    const selected = `${today.slice(0, 7)}-01`;
    await userEvent.click(openedOn);
    let calendar = await screen.findByRole("grid");
    await userEvent.click(calendar.querySelector(`[data-day="${selected}"] button`) as HTMLElement);
    expect(openedOn).toHaveTextContent(selected);
    await userEvent.click(openedOn);
    await userEvent.click(screen.getByRole("button", { name: "Clear selection" }));
    expect(openedOn).toHaveTextContent("Select a date");
    await userEvent.click(openedOn);
    calendar = await screen.findByRole("grid");
    await userEvent.click(calendar.querySelector(`[data-day="${selected}"] button`) as HTMLElement);

    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("button", { name: "Add account" }));
    expect(createAccount).toHaveBeenCalledWith(expect.objectContaining({ name: "Dated Savings", openedOn: selected }));
  });

  it("preselects the lowest-order active directory entries and submits both IDs", async () => {
    listInstitutions.mockResolvedValue([
      { id: "later-bank", name: "Later Bank", iconKey: "bank", institutionType: "bank", sortOrder: 20, archivedAt: null },
      { id: "primary-bank", name: "Primary Bank", iconKey: "bank", institutionType: "bank", sortOrder: 1, archivedAt: null },
      { id: "archived-bank", name: "Archived Bank", iconKey: "bank", institutionType: "bank", sortOrder: -1, archivedAt: "2026-01-01T00:00:00Z" },
    ]);
    listGroups.mockResolvedValue([
      { id: "later-group", name: "Later group", iconKey: "folder", sortOrder: 20, archivedAt: null },
      { id: "primary-group", name: "Primary group", iconKey: "folder", sortOrder: 1, archivedAt: null },
      { id: "archived-group", name: "Archived group", iconKey: "folder", sortOrder: -1, archivedAt: "2026-01-01T00:00:00Z" },
    ]);

    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await waitFor(() => expect(within(form).getByRole("button", { name: "Primary Bank" })).toHaveAttribute("aria-selected", "true"));
    await continueWizard(form, 3);

    await userEvent.click(within(form).getByRole("button", { name: "More settings" }));
    expect(form.querySelector("#wizard-group")).toHaveTextContent("Primary group");
    expect(within(form).queryByText("Include in liquid assets")).not.toBeInTheDocument();
    await userEvent.type(within(form).getByLabelText("Name"), "Primary account");
    await userEvent.click(within(form).getByLabelText("Alice"));
    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("button", { name: "Add account" }));

    await waitFor(() =>
      expect(createAccount).toHaveBeenCalledWith(
        expect.objectContaining({ institutionId: "primary-bank", groupId: "primary-group" }),
      ),
    );
  });

  it("keeps a manually selected institution when directory data refreshes and renames it", async () => {
    const initialInstitutions = [
      { id: "primary-bank", name: "Primary Bank", iconKey: "bank", institutionType: "bank", sortOrder: 1, archivedAt: null },
      { id: "later-bank", name: "Later Bank", iconKey: "bank", institutionType: "bank", sortOrder: 20, archivedAt: null },
    ];
    listInstitutions.mockResolvedValue(initialInstitutions);
    const { queryClient } = renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await waitFor(() => expect(within(form).getByRole("button", { name: "Primary Bank" })).toHaveAttribute("aria-selected", "true"));
    await userEvent.click(within(form).getByRole("button", { name: "Later Bank" }));

    listInstitutions.mockResolvedValue([
      { ...initialInstitutions[0], sortOrder: -10 },
      { ...initialInstitutions[1], name: "Renamed Later Bank", sortOrder: 20 },
    ]);
    await queryClient.refetchQueries({ queryKey: queryKeys.directory.institutions(false) });

    await waitFor(() => expect(within(form).getByRole("button", { name: "Renamed Later Bank" })).toHaveAttribute("aria-selected", "true"));
  });

  it("disables Continue and does not submit when no owner is checked", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await continueWizard(form, 3);
    await userEvent.type(within(form).getByLabelText("Name"), "No Owner");
    await userEvent.type(within(form).getByLabelText("Initial value"), "100");
    const continueButton = within(form).getByRole("button", { name: "Continue" });
    expect(continueButton).toBeDisabled();
    await userEvent.click(continueButton);
    expect(within(form).queryByText("Review and create")).not.toBeInTheDocument();
    expect(createAccount).not.toHaveBeenCalled();
  });

  it("assigns 100% when a single owner is checked", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await continueWizard(form, 3);
    await userEvent.type(within(form).getByLabelText("Name"), "Alice Only");
    await userEvent.click(within(form).getByLabelText("Alice"));
    await userEvent.type(within(form).getByLabelText("Initial value"), "100");
    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("button", { name: "Add account" }));
    expect(createAccount).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "Alice Only",
        ownership: [{ memberId: "alice", shareBps: 10000 }],
      }),
    );
  });

  it("creates an account with explicit ownership percentages", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await continueWizard(form, 3);
    await userEvent.type(within(form).getByLabelText("Name"), "Joint");
    await userEvent.click(within(form).getByLabelText("Alice"));
    await userEvent.click(within(form).getByLabelText("Bob"));
    await userEvent.click(within(form).getByText(/optionally enter a percentage for each selected owner/i));
    await userEvent.type(within(form).getByLabelText("Alice ownership percentage"), "70");
    await userEvent.type(within(form).getByLabelText("Bob ownership percentage"), "30");
    await userEvent.type(within(form).getByLabelText("Initial value"), "100");
    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("button", { name: "Add account" }));

    expect(createAccount).toHaveBeenCalledWith(
      expect.objectContaining({
        ownership: [
          { memberId: "alice", shareBps: 7000 },
          { memberId: "bob", shareBps: 3000 },
        ],
      }),
    );
  });

  it("shows brokerage tracking options and defaults to cash and holdings", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("button", { name: "Brokerage" }));
    await continueWizard(form, 1);
    expect(within(form).getByText("How should this account be recorded?")).toBeInTheDocument();
    expect(within(form).getByRole("radio", { name: /Record cash and holdings separately/ })).toBeChecked();
    expect(within(form).getByRole("radio", { name: /Keep one account value/ })).not.toBeChecked();
    await continueWizard(form, 1);
    expect(within(form).getByText("Recording: Record cash and holdings separately")).toBeInTheDocument();
    expect(within(form).queryByLabelText("Balance sheet role")).not.toBeInTheDocument();
    await userEvent.click(within(form).getByRole("button", { name: "More settings" }));
    expect(within(form).queryByLabelText("Include in portfolio")).not.toBeInTheDocument();
    await userEvent.type(within(form).getByLabelText("Name"), "Brokerage");
    await userEvent.click(within(form).getByLabelText("Alice"));
    await continueWizard(form, 1);
    expect(within(form).getByText("Record cash and holdings separately")).toBeInTheDocument();
    await userEvent.click(within(form).getByRole("button", { name: "Add account" }));
    expect(createAccount).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "Brokerage",
        accountType: "brokerage",
        trackingMode: "holdings",
        includeInPortfolio: true,
        initialAmount: "",
      }),
    );
  });

  it("creates a mixed bank account when the user chooses cash and holdings", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await continueWizard(form, 2);
    await userEvent.click(within(form).getByRole("radio", { name: /Record cash and holdings separately/ }));
    await continueWizard(form, 1);
    await userEvent.type(within(form).getByLabelText("Name"), "CMB Mixed");
    await userEvent.click(within(form).getByLabelText("Alice"));
    expect(within(form).queryByLabelText("Initial value")).not.toBeInTheDocument();
    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("button", { name: "Add account" }));
    expect(createAccount).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "CMB Mixed",
        accountType: "bank_account",
        trackingMode: "holdings",
        initialAmount: "",
      }),
    );
  });

  it("creates cash on hand with separate balances by currency", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("button", { name: "More account types" }));
    await userEvent.click(within(form).getByRole("button", { name: "Cash on hand" }));
    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("radio", { name: /Record cash by currency/ }));
    await continueWizard(form, 1);
    await userEvent.type(within(form).getByLabelText("Name"), "Travel cash");
    await userEvent.click(within(form).getByLabelText("Alice"));
    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("button", { name: "Add account" }));

    expect(createAccount).toHaveBeenCalledWith(expect.objectContaining({
      name: "Travel cash",
      accountType: "cash_on_hand",
      trackingMode: "holdings",
      initialAmount: "",
      includeInLiquidAssets: true,
    }));
  });

  it("renders catalog-only account types on the type step", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await continueWizard(form, 1);
    expect(within(form).getByRole("button", { name: "Bank account" })).toHaveTextContent(
      "Recording: Keep one account balance or Record cash and holdings separately",
    );
    expect(within(form).getByRole("button", { name: "Brokerage" })).toHaveTextContent(
      "Recording: Record cash and holdings separately or Keep one account value",
    );
    expect(within(form).getByRole("button", { name: "Credit card" })).toHaveTextContent("Recording: Keep one account balance");
    expect(within(form).getByRole("button", { name: "Other" })).toBeInTheDocument();
    expect(within(form).queryByRole("button", { name: "Cash on hand" })).not.toBeInTheDocument();
    await userEvent.click(within(form).getByRole("button", { name: "More account types" }));
    expect(within(form).getByRole("button", { name: "Cash on hand" })).toHaveTextContent(
      "Recording: Keep one account balance or Record cash by currency",
    );
  });

  it("opens simple account detail instead of the metadata editor", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /Checking/ }));
    expect(await screen.findByTestId("account-detail")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Update balance" })).toBeInTheDocument();
    expect(screen.queryByText("Cash")).not.toBeInTheDocument();
    expect(screen.queryByRole("form", { name: "Account form" })).not.toBeInTheDocument();
  });

  it("shows cash, holdings, and completeness on a composite account", async () => {
    listAccounts.mockResolvedValue([brokerageAccount]);
    accountValuations.mockResolvedValue([
      {
        account: brokerageAccount.account,
        ownership: brokerageAccount.ownership,
        complete: false,
        composition: {
          currency: "USD", totalExact: "2450", complete: false, status: "partial", valuedCount: 3, missingCount: 1,
          types: [
            { key: "account-composition:type:stock", typeKey: "stock", label: "stock", detailRefs: ["h1", "missing"], baseAmountExact: "1500", shareBps: 6122, valuedCount: 1, missingCount: 1 },
            { key: "account-composition:type:cash", typeKey: "cash", label: "cash", detailRefs: ["cash-USD", "cash-SGD"], baseAmountExact: "950", shareBps: 3878, valuedCount: 2, missingCount: 0 },
          ],
          details: [
            { key: "h1", label: "NVIDIA", typeKey: "stock", detailRefs: ["h1"], baseAmountExact: "1500", shareBps: 6122, nativeAmount: "1500", nativeCurrency: "USD", valuedCount: 1, missingCount: 0 },
            { key: "cash-USD", label: "USD", typeKey: "cash", detailRefs: ["cash-USD"], baseAmountExact: "800", shareBps: 3265, nativeAmount: "800", nativeCurrency: "USD", valuedCount: 1, missingCount: 0 },
            { key: "cash-SGD", label: "SGD", typeKey: "cash", detailRefs: ["cash-SGD"], baseAmountExact: "150", shareBps: 613, nativeAmount: "200", nativeCurrency: "SGD", valuedCount: 1, missingCount: 0 },
            { key: "missing", label: "Missing instrument", typeKey: "stock", detailRefs: ["missing"], shareBps: 0, valuedCount: 0, missingCount: 1 },
          ],
          detailSlices: [
            { key: "h1", label: "NVIDIA", typeKey: "stock", detailRefs: ["h1"], baseAmountExact: "1500", shareBps: 6122, nativeAmount: "1500", nativeCurrency: "USD", valuedCount: 1, missingCount: 0 },
            { key: "cash-USD", label: "USD", typeKey: "cash", detailRefs: ["cash-USD"], baseAmountExact: "800", shareBps: 3265, nativeAmount: "800", nativeCurrency: "USD", valuedCount: 1, missingCount: 0 },
            { key: "cash-SGD", label: "SGD", typeKey: "cash", detailRefs: ["cash-SGD"], baseAmountExact: "150", shareBps: 613, nativeAmount: "200", nativeCurrency: "SGD", valuedCount: 1, missingCount: 0 },
            { key: "missing", label: "Missing instrument", typeKey: "stock", detailRefs: ["missing"], shareBps: 0, valuedCount: 0, missingCount: 1 },
          ],
        },
        components: [
          { nativeCurrency: "SGD", nativeAmount: "200", available: true, baseAmount: { amount: "150", currency: "USD" } },
          { nativeCurrency: "USD", nativeAmount: "800", available: true, baseAmount: { amount: "800", currency: "USD" } },
          {
            holdingId: "h1",
            instrumentId: "i1",
            instrumentName: "NVIDIA",
            nativeCurrency: "USD",
            nativeAmount: "1500",
            available: true,
            baseAmount: { amount: "1500", currency: "USD" },
          },
        ],
        missingInputs: [{ kind: "fx_rate", accountId: "brk-1" }],
        baseValue: { amount: "2450", currency: "USD" },
      },
    ]);
    holdingsByAccounts.mockResolvedValue({
      "brk-1": [{ id: "h1", accountId: "brk-1", instrumentId: "i1", quantity: "10" }],
    });
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", type: "stock", quoteCurrency: "USD" }]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));
    expect(await screen.findByTestId("account-detail")).toBeInTheDocument();
    expect(screen.getByText("$2,450.00 · Valued subtotal", { selector: ".text-2xl" })).toHaveClass("text-2xl");
    expect(screen.getByTestId("composition-chart")).toBeInTheDocument();
    const fetchCounts = [accountValuations.mock.calls.length, currentInstrumentQuote.mock.calls.length, listInstruments.mock.calls.length, holdingsByAccounts.mock.calls.length];
    expect(screen.getByRole("radio", { name: "By type" })).toBeChecked();
    await userEvent.click(screen.getByRole("radio", { name: "By detail" }));
    expect([accountValuations.mock.calls.length, currentInstrumentQuote.mock.calls.length, listInstruments.mock.calls.length, holdingsByAccounts.mock.calls.length]).toEqual(fetchCounts);
    const slices = ["h1", "cash-USD", "cash-SGD"].map((key) => screen.getByTestId(`composition-slice-${key}`));
    expect(slices[0]).toHaveTextContent("NVIDIA");
    expect(slices[1]).toHaveTextContent("USD");
    expect(slices[2]).toHaveTextContent("SGD");
    expect(screen.getByTestId("composition-native-cash-SGD")).toHaveTextContent(/SGD\s*200\.00/);
    expect(screen.getByText("Cash")).toBeInTheDocument();
    expect(screen.getAllByText("Investments").length).toBeGreaterThan(0);
    const cashHoldingsLayout = screen.getByTestId("account-cash-holdings-layout");
    expect(cashHoldingsLayout).toHaveClass("2xl:grid-cols-2");
    expect(cashHoldingsLayout).not.toHaveClass("lg:grid-cols-2");
    expect(screen.getAllByText("$800.00").length).toBeGreaterThan(0);
    expect(screen.getAllByText(/SGD\s*200\.00/).length).toBeGreaterThan(0);
    expect(screen.getAllByText("NVIDIA").length).toBeGreaterThan(0);
    expect(screen.getByText("Stock")).toBeInTheDocument();
    expect(screen.getByText("Partial valuation")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent(/Missing FX rate/);
  });

  it("hides zero positions while keeping dividend entry available", async () => {
    listAccounts.mockResolvedValue([brokerageAccount]);
    holdingsByAccounts.mockResolvedValue({ "brk-1": [
      { id: "zero", accountId: "brk-1", instrumentId: "closed", quantity: "0.00000000" },
      { id: "tiny", accountId: "brk-1", instrumentId: "active", quantity: "0.00000001" },
    ] });
    listInstruments.mockResolvedValue([
      { id: "closed", name: "Closed position", type: "stock", quoteCurrency: "USD" },
      { id: "active", name: "Small position", type: "stock", quoteCurrency: "USD" },
    ]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));
    const layout = within(await screen.findByTestId("account-cash-holdings-layout"));
    expect(await layout.findByText("Small position")).toBeInTheDocument();
    expect(layout.queryByText("Closed position")).not.toBeInTheDocument();
    expect(layout.getByRole("button", { name: "Cash dividend" })).toBeEnabled();
  });

  it("shows complete cash history fields", async () => {
    listAccounts.mockResolvedValue([brokerageAccount]);
    accountValuations.mockResolvedValue([{
      account: brokerageAccount.account,
      ownership: brokerageAccount.ownership,
      complete: true,
      components: [{ nativeCurrency: "SGD", nativeAmount: "200", available: true }],
      missingInputs: [],
      baseValue: { amount: "200", currency: "USD" },
    }]);
    listAccountCashValues.mockResolvedValue([{
      id: "cash-1",
      accountId: "brk-1",
      amount: { amount: "200.0001", currency: "SGD" },
      effectiveAt: "2026-09-15T03:00:00.000Z",
      createdAt: "2026-09-15T03:05:00.000Z",
      observationKind: "event",
      activityId: "activity-1",
      activityEffectId: "effect-1",
      activityKind: "cash_in",
      activityReason: "income",
      activityNote: "Salary",
      change: { amount: "100.0001", currency: "SGD" },
    }, {
      id: "cash-2",
      accountId: "brk-1",
      amount: { amount: "200", currency: "SGD" },
      effectiveAt: "2026-09-15T03:00:00.000Z",
      createdAt: "2026-09-15T03:05:00.000Z",
      observationKind: "event",
      activityId: "activity-2",
      activityEffectId: "effect-2",
      activityKind: "cash_out",
      activityReason: "fee",
      activityNote: "Broker fee",
      change: { amount: "-0.0001", currency: "SGD" },
    }]);

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));

    const history = await screen.findByTestId("account-cash-history");
    expect(within(history).getByRole("columnheader", { name: "Type" })).toBeInTheDocument();
    expect(within(history).getByRole("columnheader", { name: "Change" })).toBeInTheDocument();
    expect(within(history).getByRole("columnheader", { name: "Balance" })).toBeInTheDocument();
    expect(within(history).getByRole("columnheader", { name: "Currency" })).toBeInTheDocument();
    expect(within(history).getByRole("columnheader", { name: "Effective at" })).toBeInTheDocument();
    expect(within(history).getByRole("columnheader", { name: "Recorded at" })).toBeInTheDocument();
    expect(within(history).queryByRole("columnheader", { name: "Reference" })).not.toBeInTheDocument();
    expect(within(history).getByText("Money added")).toBeInTheDocument();
    expect(within(history).getByText("Money removed")).toBeInTheDocument();
    expect(within(history).getByText("Salary")).toBeInTheDocument();
    expect(within(history).getByText("Broker fee")).toBeInTheDocument();
    expect(history.textContent).toContain("+SGD");
    expect(history.textContent).toContain("−SGD");
    expect(history.textContent).toContain("100.0001 SGD");
    expect(history.textContent).toContain("200.0001 SGD");
    expect(history.textContent).toMatch(/−SGD\s*0\.00 \(-0\.0001 SGD\)/);
    expect([...history.querySelectorAll("time")].map((element) => element.dateTime)).toEqual([
      "2026-09-15T03:00:00.000Z",
      "2026-09-15T03:05:00.000Z",
      "2026-09-15T03:00:00.000Z",
      "2026-09-15T03:05:00.000Z",
    ]);
  });

  it("shows multi-currency cash without investment actions for cash on hand", async () => {
    const multiCurrencyCashAccount = {
      account: {
        ...emptyAccount.account,
        id: "cash-1",
        name: "Travel cash",
        accountType: "cash_on_hand",
        trackingMode: "holdings",
        includeInLiquidAssets: true,
      },
      ownership: emptyAccount.ownership,
    };
    listAccounts.mockResolvedValue([multiCurrencyCashAccount]);
    accountValuations.mockResolvedValue([{
      account: multiCurrencyCashAccount.account,
      ownership: multiCurrencyCashAccount.ownership,
      complete: true,
      composition: {
        currency: "USD", totalExact: "425", complete: true, status: "ready", valuedCount: 2, missingCount: 0,
        types: [{ key: "account-composition:type:cash", typeKey: "cash", label: "cash", detailRefs: ["cash-SGD", "cash-USD"], baseAmountExact: "425", shareBps: 10000, valuedCount: 2, missingCount: 0 }],
        details: [
          { key: "cash-SGD", typeKey: "cash", label: "SGD", detailRefs: ["cash-SGD"], baseAmountExact: "225", shareBps: 5294, nativeAmount: "300", nativeCurrency: "SGD", valuedCount: 1, missingCount: 0 },
          { key: "cash-USD", typeKey: "cash", label: "USD", detailRefs: ["cash-USD"], baseAmountExact: "200", shareBps: 4706, nativeAmount: "200", nativeCurrency: "USD", valuedCount: 1, missingCount: 0 },
        ],
        detailSlices: [
          { key: "cash-SGD", typeKey: "cash", label: "SGD", detailRefs: ["cash-SGD"], baseAmountExact: "225", shareBps: 5294, nativeAmount: "300", nativeCurrency: "SGD", valuedCount: 1, missingCount: 0 },
          { key: "cash-USD", typeKey: "cash", label: "USD", detailRefs: ["cash-USD"], baseAmountExact: "200", shareBps: 4706, nativeAmount: "200", nativeCurrency: "USD", valuedCount: 1, missingCount: 0 },
        ],
      },
      components: [
        { nativeCurrency: "SGD", nativeAmount: "300", available: true, baseAmount: { amount: "225", currency: "USD" } },
        { nativeCurrency: "USD", nativeAmount: "200", available: true, baseAmount: { amount: "200", currency: "USD" } },
      ],
      missingInputs: [],
      baseValue: { amount: "425", currency: "USD" },
    }]);
    holdingsByAccounts.mockResolvedValue({ "cash-1": [] });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /Travel cash/ }));
    expect(await screen.findByTestId("account-detail")).toBeInTheDocument();
    expect(screen.getByTestId("composition-slice-account-composition:type:cash")).toHaveTextContent("100.0%");
    await userEvent.click(screen.getByRole("radio", { name: "By detail" }));
    expect(screen.getByText("Cash")).toBeInTheDocument();
    expect(screen.getByTestId("composition-chart")).toBeInTheDocument();
    expect(screen.getByTestId("composition-slice-cash-SGD")).toHaveTextContent("SGD");
    expect(screen.getAllByText(/SGD\s*300\.00/).length).toBeGreaterThan(0);
    expect(screen.queryByText("Investments")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Buy investment" })).not.toBeInTheDocument();
  });

  it("rejects a zero-quantity existing position and does not record a buy", async () => {
    listAccounts.mockResolvedValue([brokerageAccount]);
    accountValuations.mockResolvedValue([
      {
        account: brokerageAccount.account,
        ownership: brokerageAccount.ownership,
        complete: true,
        components: [],
        missingInputs: [],
        baseValue: { amount: "0", currency: "USD" },
      },
    ]);
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", type: "stock", quoteCurrency: "USD" }]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Record existing position" }));
    await userEvent.selectOptions(await screen.findByLabelText("Instrument"), "i1");
    await userEvent.type(screen.getByLabelText("Quantity"), "0");
    const submitButtons = screen.getAllByRole("button", { name: "Record existing position" });
    await userEvent.click(submitButtons[submitButtons.length - 1]);
    expect(await screen.findByRole("alert")).toHaveTextContent(/greater than zero/i);
    expect(createHolding).not.toHaveBeenCalled();
    expect(recordChange).not.toHaveBeenCalled();
  });

  it("creates a credit card as a liability without asking tracking or role enums", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("button", { name: "Credit card" }));
    expect(within(form).queryByText("Is this an asset or a liability?")).not.toBeInTheDocument();
    await continueWizard(form, 1);
    expect(within(form).queryByText("How should this account be recorded?")).not.toBeInTheDocument();
    expect(within(form).queryByLabelText("Balance sheet role")).not.toBeInTheDocument();
    await userEvent.type(within(form).getByLabelText("Name"), "CMB Visa");
    await userEvent.click(within(form).getByLabelText("Alice"));
    await userEvent.type(within(form).getByLabelText("Initial value"), "200");
    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("button", { name: "Add account" }));
    expect(createAccount).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "CMB Visa",
        accountType: "credit_card",
        balanceSheetRole: "liability",
        trackingMode: "balance",
      }),
    );
  });

  it("lets Other choose asset or liability in product language", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("button", { name: "Other" }));
    expect(within(form).getByText("Is this an asset or a liability?")).toBeInTheDocument();
    await userEvent.click(within(form).getByRole("radio", { name: "Liability" }));
    await continueWizard(form, 1);
    await userEvent.type(within(form).getByLabelText("Name"), "Personal loan");
    await userEvent.click(within(form).getByLabelText("Alice"));
    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("button", { name: "Add account" }));
    expect(createAccount).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "Personal loan",
        accountType: "other",
        balanceSheetRole: "liability",
        trackingMode: "balance",
      }),
    );
  });

  it("records opening cash in a foreign currency without treating it as a buy", async () => {
    listAccounts.mockResolvedValue([brokerageAccount]);
    accountValuations.mockResolvedValue([
      {
        account: brokerageAccount.account,
        ownership: brokerageAccount.ownership,
        complete: true,
        components: [],
        missingInputs: [],
        baseValue: { amount: "0", currency: "USD" },
      },
    ]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Add cash balance" }));
    const form = await screen.findByRole("form", { name: "Add cash balance" });
    await userEvent.selectOptions(within(form).getByLabelText("Currency"), "CNY");
    await userEvent.type(within(form).getByLabelText("Resulting balance"), "5000");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    await waitFor(() => expect(previewCash).toHaveBeenCalledWith("brk-1", "5000", "CNY", null));
    expect(confirmCash).not.toHaveBeenCalled();
    expect(appendCash).not.toHaveBeenCalled();
    await userEvent.click(within(form).getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(confirmCash).toHaveBeenCalledWith("cash-plan"));
    await waitFor(() => expect(screen.getByRole("button", { name: "Add cash balance" })).toHaveFocus());
    expect(recordChange).not.toHaveBeenCalled();
  });

  it("starts History from a deposit and writes nothing when cancelled", async () => {
    listAccounts.mockResolvedValue([brokerageAccount]);
    accountValuations.mockResolvedValue([
      {
        account: brokerageAccount.account,
        ownership: brokerageAccount.ownership,
        complete: true,
        components: [],
        missingInputs: [],
        baseValue: { amount: "0", currency: "USD" },
      },
    ]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Deposit or withdraw" }));
    expect(await screen.findByText("Start history to continue")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(startHistory).not.toHaveBeenCalled();
    expect(recordChange).not.toHaveBeenCalled();
  });

  it("returns to Buy after Start History without waiting for origin refresh", async () => {
    listAccounts.mockResolvedValue([brokerageAccount]);
    accountValuations.mockResolvedValue([
      {
        account: brokerageAccount.account,
        ownership: brokerageAccount.ownership,
        complete: true,
        components: [],
        missingInputs: [],
        baseValue: { amount: "0", currency: "USD" },
      },
    ]);
    listInstruments.mockResolvedValue([{ id: "fund-1", name: "XYZ Fund", type: "mutual_fund", quoteCurrency: "CNY" }]);
    historyOrigin.mockResolvedValue(null);
    startHistory.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Buy investment" }));
    expect(await screen.findByText("Start history to continue")).toBeInTheDocument();
    expect(screen.getByLabelText("Start date")).toHaveTextContent(localDateInTimeZone(Intl.DateTimeFormat().resolvedOptions().timeZone) ?? "");
    const timezone = await screen.findByLabelText("Timezone");
    expect(timezone).toHaveValue(Intl.DateTimeFormat().resolvedOptions().timeZone);
    expect(timezone).toHaveAttribute("readonly");
    await userEvent.click(screen.getByRole("button", { name: "Start history" }));
    expect(await screen.findByLabelText("Instrument")).toBeInTheDocument();
    const resolvedTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    expect(startHistoryOnDate).toHaveBeenCalledWith(resolvedTimezone, localDateInTimeZone(resolvedTimezone), null);
    expect(historyOrigin).toHaveBeenCalled();
    expect(recordChange).not.toHaveBeenCalled();
  });

  it("first buy of an unheld instrument leaves holdingId empty so Go creates the Holding", async () => {
    const mixed = {
      account: {
        ...emptyAccount.account,
        id: "cmb-mixed",
        name: "CMB Mixed",
        trackingMode: "holdings",
        includeInPortfolio: true,
      },
      ownership: emptyAccount.ownership,
      institutionName: "China Merchants Bank",
    };
    listAccounts.mockResolvedValue([mixed]);
    accountValuations.mockResolvedValue([
      {
        account: mixed.account,
        ownership: mixed.ownership,
        complete: true,
        components: [{ nativeCurrency: "CNY", nativeAmount: "10000", available: true }],
        missingInputs: [],
        baseValue: { amount: "10000", currency: "USD" },
      },
    ]);
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listInstruments.mockResolvedValue([{ id: "fund-1", name: "XYZ Fund", type: "mutual_fund", quoteCurrency: "CNY" }]);
    holdingsByAccounts.mockResolvedValue({ "cmb-mixed": [] });
    previewChange.mockResolvedValue({
      activity: { id: "a1", kind: "buy", effects: [] },
      effects: [],
      resulting: [
        { target: "account_cash", name: "CMB Mixed", amount: "8000", currency: "CNY" },
        { target: "holding_quantity", name: "XYZ Fund", quantity: "100" },
      ],
    });
    recordChange.mockResolvedValue({ activity: { id: "a1", kind: "buy" }, effects: [], resulting: [] });
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /CMB Mixed/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Buy investment" }));
    const form = await screen.findByRole("form", { name: "Record change" });
    await userEvent.selectOptions(within(form).getByLabelText("Instrument"), "fund-1");
    await userEvent.type(within(form).getByLabelText("Quantity"), "100");
    await userEvent.type(within(form).getByLabelText("Gross total"), "2000");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    await waitFor(() =>
      expect(previewChange).toHaveBeenCalledWith(
        expect.objectContaining({
          kind: "trade",
          settlementAccountId: "cmb-mixed",
          instrumentId: "fund-1",
          side: "buy",
          quantity: "100",
          gross: "2000",
        }),
      ),
    );
    expect(previewChange.mock.calls[0][0].holdingId ?? "").toBe("");
    await userEvent.click(within(form).getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(recordChange).toHaveBeenCalled());
    expect(recordChange.mock.calls[0][0].holdingId ?? "").toBe("");
    expect(createHolding).not.toHaveBeenCalled();
  });

  it("completes the CMB mixed-account first fund buy journey", async () => {
    let accounts: unknown[] = [];
    let valuations: unknown[] = [];
    const holdings: Record<string, unknown[]> = {};
    listAccounts.mockImplementation(async () => accounts);
    accountValuations.mockImplementation(async () => valuations);
    holdingsByAccounts.mockImplementation(async () => holdings);
    historyOrigin.mockResolvedValue(null);
    listInstruments.mockResolvedValue([{ id: "fund-1", name: "XYZ Fund", type: "mutual_fund", quoteCurrency: "CNY" }]);
    createAccount.mockImplementation(async (request: { name: string; accountType: string; balanceSheetRole: string; trackingMode: string; defaultCurrency: string; includeInNetWorth: boolean; includeInPortfolio: boolean; includeInLiquidAssets: boolean; institutionId?: string; ownership: unknown[] }) => {
      const record = {
        account: {
          id: "cmb-mixed",
          name: request.name,
          accountType: request.accountType,
          balanceSheetRole: request.balanceSheetRole,
          trackingMode: request.trackingMode,
          defaultCurrency: request.defaultCurrency,
          includeInNetWorth: request.includeInNetWorth,
          includeInPortfolio: request.includeInPortfolio,
          includeInLiquidAssets: request.includeInLiquidAssets,
          institutionId: request.institutionId,
          sortOrder: 0,
          createdAt: "",
          updatedAt: "",
        },
        ownership: request.ownership,
        institutionName: "China Merchants Bank",
      };
      accounts = [record];
      valuations = [
        {
          account: record.account,
          ownership: record.ownership,
          complete: true,
          components: [],
          missingInputs: [],
          baseValue: { amount: "0", currency: "USD" },
        },
      ];
      return record;
    });
    confirmCash.mockImplementation(async () => {
      const [, amount, currency] = previewCash.mock.calls[previewCash.mock.calls.length - 1] as [string, string, string];
      valuations = [
        {
          account: (accounts[0] as { account: unknown }).account,
          ownership: (accounts[0] as { ownership: unknown }).ownership,
          complete: true,
          components: [{ nativeCurrency: currency, nativeAmount: amount, available: true }],
          missingInputs: [],
          baseValue: { amount, currency: "USD" },
        },
      ];
    });
    startHistory.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    previewChange.mockResolvedValue({
      activity: { id: "a1", kind: "buy", effects: [] },
      effects: [],
      resulting: [
        { target: "account_cash", name: "CMB Mixed", amount: "8000", currency: "CNY" },
        { target: "holding_quantity", name: "XYZ Fund", quantity: "100" },
      ],
    });
    recordChange.mockImplementation(async (request: { holdingId?: string }) => {
      expect(request.holdingId ?? "").toBe("");
      holdings["cmb-mixed"] = [{ id: "h-fund", accountId: "cmb-mixed", instrumentId: "fund-1", quantity: "100" }];
      valuations = [
        {
          account: (accounts[0] as { account: unknown }).account,
          ownership: (accounts[0] as { ownership: unknown }).ownership,
          complete: false,
          components: [
            { nativeCurrency: "CNY", nativeAmount: "8000", available: true },
            { holdingId: "h-fund", instrumentId: "fund-1", instrumentName: "XYZ Fund", available: false },
          ],
          missingInputs: [{ kind: "instrument_price", accountId: "cmb-mixed", instrumentName: "XYZ Fund" }],
          baseValue: { amount: "8000", currency: "USD" },
        },
      ];
      return { activity: { id: "a1", kind: "buy" }, effects: [], resulting: [] };
    });

    renderPage();
    await screen.findByText("No accounts yet");
    await userEvent.click(screen.getByText("Add account"));
    const wizard = await screen.findByRole("form", { name: "Create account" });
    await userEvent.click(within(wizard).getByRole("button", { name: "China Merchants Bank" }));
    await continueWizard(wizard, 2);
    await userEvent.click(within(wizard).getByRole("radio", { name: /Record cash and holdings separately/ }));
    await continueWizard(wizard, 1);
    await userEvent.type(within(wizard).getByLabelText("Name"), "CMB Mixed");
    await userEvent.click(within(wizard).getByLabelText("Alice"));
    await continueWizard(wizard, 1);
    await userEvent.click(within(wizard).getByRole("button", { name: "Add account" }));
    await waitFor(() =>
      expect(createAccount).toHaveBeenCalledWith(
        expect.objectContaining({ name: "CMB Mixed", accountType: "bank_account", trackingMode: "holdings" }),
      ),
    );

    expect(await screen.findByTestId("account-detail")).toBeInTheDocument();
    await userEvent.click(await screen.findByRole("button", { name: "Add cash balance" }));
    const cashForm = await screen.findByRole("form", { name: "Add cash balance" });
    await userEvent.selectOptions(within(cashForm).getByLabelText("Currency"), "CNY");
    await userEvent.type(within(cashForm).getByLabelText("Resulting balance"), "10000");
    await userEvent.click(within(cashForm).getByRole("button", { name: "Preview" }));
    await waitFor(() => expect(previewCash).toHaveBeenCalledWith("cmb-mixed", "10000", "CNY", null));
    expect(confirmCash).not.toHaveBeenCalled();
    await userEvent.click(await within(cashForm).findByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(confirmCash).toHaveBeenCalledWith("cash-plan"));
    expect((await screen.findAllByText(/CN¥\s*10,000\.00/)).length).toBeGreaterThan(0);

    await userEvent.click(await screen.findByRole("button", { name: "Buy investment" }));
    expect(await screen.findByText("Start history to continue")).toBeInTheDocument();
    expect(screen.getByLabelText("Start date")).toHaveTextContent(localDateInTimeZone(Intl.DateTimeFormat().resolvedOptions().timeZone) ?? "");
    const timezone = screen.getByLabelText("Timezone");
    expect(timezone).toHaveValue(Intl.DateTimeFormat().resolvedOptions().timeZone);
    expect(timezone).toHaveAttribute("readonly");
    await userEvent.click(screen.getByRole("button", { name: "Start history" }));

    const tradeForm = await screen.findByRole("form", { name: "Record change" });
    await userEvent.selectOptions(within(tradeForm).getByLabelText("Instrument"), "fund-1");
    await userEvent.type(within(tradeForm).getByLabelText("Quantity"), "100");
    await userEvent.type(within(tradeForm).getByLabelText("Gross total"), "2000");
    await userEvent.click(within(tradeForm).getByRole("button", { name: "Preview" }));
    await userEvent.click(await within(tradeForm).findByRole("button", { name: "Confirm" }));

    expect((await screen.findAllByText(/CN¥\s*8,000\.00/)).length).toBeGreaterThan(0);
    expect(screen.queryByText(/CN¥\s*10,000\.00/)).not.toBeInTheDocument();
    expect(screen.getByText("XYZ Fund")).toBeInTheDocument();
    expect(screen.getByText("100")).toBeInTheDocument();
    expect(screen.getByText("Missing current price")).toBeInTheDocument();
    expect(screen.getByText("Partial valuation")).toBeInTheDocument();
    expect(recordChange.mock.calls[0][0].holdingId ?? "").toBe("");
    expect(createHolding).not.toHaveBeenCalled();
  });

  it("archives an active account from account settings", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /Checking/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Account settings" }));
    const dialogTrigger = await screen.findByRole("button", { name: "Archive" });
    await userEvent.click(dialogTrigger);
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Archive" }));
    expect(archiveAccount).toHaveBeenCalledWith("acc-1", true);
  });

  it("edits an existing account's name through Account settings", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /Checking/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Account settings" }));
    const form = await screen.findByRole("form", { name: "Account form" });
    await waitFor(() => expect(within(form).getByLabelText("Name")).toHaveFocus());
    expect(within(form).getByLabelText("Currency")).toHaveAttribute("readonly");
    expect(screen.getByRole("button", { name: "Save" })).toHaveAttribute("form", form.id);
    await userEvent.click(within(form).getByRole("button", { name: /Details/i }));
    const nameInput = within(form).getByLabelText("Name");
    const type = within(form).getByLabelText("Account type");
    const icon = within(form).getByLabelText("Choose icon");
    expect(nameInput.compareDocumentPosition(type) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(type.compareDocumentPosition(icon) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    await userEvent.clear(nameInput);
    await userEvent.type(nameInput, "Renamed");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(updateAccount).toHaveBeenCalledWith(
      "acc-1",
      expect.objectContaining({ name: "Renamed" }),
    );
  });

  it.each(["success", "failure"])("keeps a pending settings %s in its original editing session", async (outcome) => {
    let resolve!: (value: typeof emptyAccount) => void;
    let reject!: (error: Error) => void;
    updateAccount.mockReturnValueOnce(new Promise((yes, no) => { resolve = yes; reject = no; }));
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole("button", { name: /Checking/ }));
    await user.click(await screen.findByRole("button", { name: "Account settings" }));
    const form = await screen.findByRole("form", { name: "Account form" });
    const name = within(form).getByLabelText("Name");
    await user.clear(name);
    await user.type(name, "Pending draft");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(updateAccount).toHaveBeenCalledOnce());
    expect(name).toBeDisabled();
    expect(within(form).getByLabelText("Include in net worth")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Close" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Archive" })).toBeDisabled();
    await user.keyboard("{Escape}");
    expect(form).toBeInTheDocument();
    // A repeated form submission must not enqueue another write.
    fireEvent.submit(form);
    expect(updateAccount).toHaveBeenCalledOnce();
    await act(async () => {
      if (outcome === "success") resolve(emptyAccount);
      else reject(new Error("Temporary connection failure"));
    });
    if (outcome === "failure") {
      expect(await screen.findByRole("alert")).toBeInTheDocument();
      expect(name).toHaveValue("Pending draft");
      expect(name).toBeEnabled();
      await user.click(screen.getByRole("button", { name: "Cancel" }));
    }
    await waitFor(() => expect(screen.queryByRole("form", { name: "Account form" })).not.toBeInTheDocument());
    await user.click(screen.getByRole("button", { name: "Account settings" }));
    const reopened = await screen.findByRole("form", { name: "Account form" });
    await user.clear(within(reopened).getByLabelText("Name"));
    await user.type(within(reopened).getByLabelText("Name"), "New session");
    expect(within(reopened).getByLabelText("Name")).toHaveValue("New session");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(updateAccount).toHaveBeenCalledTimes(2));
    expect(updateAccount.mock.calls[1][1]).toMatchObject({ name: "New session" });
  });

  it("disables Save and does not update when every owner is unchecked", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /Checking/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Account settings" }));
    const form = await screen.findByRole("form", { name: "Account form" });
    await userEvent.click(within(form).getByLabelText("Alice"));
    const saveButton = screen.getByRole("button", { name: "Save" });
    await waitFor(() => expect(saveButton).toBeDisabled());
    await userEvent.click(saveButton);
    expect(updateAccount).not.toHaveBeenCalled();
  });

  it("edits brokerage holdings type without applying a new default combination", async () => {
    const { selectValues } = await import("@/test/catalog");
    listAccounts.mockResolvedValue([brokerageAccount]);
    accountValuations.mockResolvedValue([
      {
        account: brokerageAccount.account,
        ownership: brokerageAccount.ownership,
        complete: true,
        components: [],
        missingInputs: [],
        baseValue: { amount: "2500", currency: "USD" },
      },
    ]);
    updateAccount.mockResolvedValue(brokerageAccount);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Account settings" }));
    const form = await screen.findByRole("form", { name: "Account form" });
    await waitFor(() => {
      expect(selectValues(within(form).getByLabelText("Account type"))).toEqual(["bank_account", "brokerage"]);
    });
    expect(within(form).getByText("Asset")).toBeInTheDocument();
    expect(within(form).getByText(/Tracking method/)).toBeInTheDocument();
    expect(within(form).getByText(/Record cash and holdings separately/)).toBeInTheDocument();
    expect(within(form).queryByLabelText("Include in portfolio")).not.toBeInTheDocument();
    await userEvent.selectOptions(within(form).getByLabelText("Account type"), "bank_account");
    expect(within(form).getByText("Asset")).toBeInTheDocument();
    expect(within(form).getByText(/Record cash and holdings separately/)).toBeInTheDocument();
    expect(within(form).queryByLabelText("Include in portfolio")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(updateAccount).toHaveBeenCalledWith(
      "brk-1",
      expect.objectContaining({
        accountType: "bank_account",
        balanceSheetRole: "asset",
        trackingMode: "holdings",
        includeInPortfolio: true,
      }),
    );
  });

  it("shows the current cash balance on reconcile and does not prefill the resulting amount", async () => {
    listAccounts.mockResolvedValue([brokerageAccount]);
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    accountValuations.mockResolvedValue([
      {
        account: brokerageAccount.account,
        ownership: brokerageAccount.ownership,
        complete: true,
        components: [{ nativeCurrency: "USD", nativeAmount: "800", available: true }],
        missingInputs: [],
        baseValue: { amount: "800", currency: "USD" },
      },
    ]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Reconcile balance" }));
    const form = await screen.findByRole("form", { name: "Add cash balance" });
    expect(form).toHaveTextContent(/Current balance/);
    expect(form).toHaveTextContent("$800.00");
    expect(within(form).getByLabelText("Resulting balance")).toHaveValue("");
  });

  it("disables simple value Save while the amount is unchanged", async () => {
    const simple = {
      ...emptyAccount,
      account: { ...emptyAccount.account, trackingMode: "manual_value", name: "House" },
    };
    listAccounts.mockResolvedValue([simple]);
    accountValuations.mockResolvedValue([
      {
        account: simple.account,
        ownership: simple.ownership,
        complete: true,
        components: [{ nativeCurrency: "USD", nativeAmount: "1000", available: true }],
        missingInputs: [],
        baseValue: { amount: "1000", currency: "USD" },
      },
    ]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /House/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Update value" }));
    const form = await screen.findByRole("form", { name: "Update value" });
    expect(within(form).getByLabelText("New value")).toHaveValue("1000");
    expect(within(form).getByRole("button", { name: "Save" })).toBeDisabled();
    await userEvent.clear(within(form).getByLabelText("New value"));
    await userEvent.type(within(form).getByLabelText("New value"), "1100");
    expect(within(form).getByRole("button", { name: "Save" })).toBeEnabled();
    await userEvent.click(within(form).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(appendValue).toHaveBeenCalledWith("acc-1", "1100", ""));
  });

  it("defaults record-position unit cost from the latest price", async () => {
    listAccounts.mockResolvedValue([brokerageAccount]);
    accountValuations.mockResolvedValue([
      {
        account: brokerageAccount.account,
        ownership: brokerageAccount.ownership,
        complete: true,
        components: [],
        missingInputs: [],
        baseValue: { amount: "0", currency: "USD" },
      },
    ]);
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", type: "stock", quoteCurrency: "USD", quoteSource: "provider" }]);
    currentInstrumentQuote.mockResolvedValue({
      id: "q1",
      instrumentId: "i1",
      unitPrice: "12.50",
      currency: "USD",
      quotedAt: "2026-08-23T00:00:00Z",
    });
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Record existing position" }));
    await userEvent.selectOptions(await screen.findByLabelText("Instrument"), "i1");
    await waitFor(() => expect(screen.getByLabelText("Unit cost (optional) USD")).toHaveValue("12.50"));
  });

  it("opens cash dividend from a holding row with that holding preselected and submits the existing command", async () => {
    listAccounts.mockResolvedValue([brokerageAccount]);
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listInstruments.mockResolvedValue([{ id: "fund-1", name: "XYZ Fund", type: "mutual_fund", quoteCurrency: "USD", quoteSource: "manual" }]);
    holdingsByAccounts.mockResolvedValue({
      "brk-1": [{ id: "holding-1", accountId: "brk-1", instrumentId: "fund-1", quantity: "10" }],
    });
    accountValuations.mockResolvedValue([
      {
        account: brokerageAccount.account,
        ownership: brokerageAccount.ownership,
        complete: true,
        components: [
          { holdingId: "holding-1", instrumentId: "fund-1", instrumentName: "XYZ Fund", nativeAmount: "1000", nativeCurrency: "USD", available: true },
        ],
        missingInputs: [],
        baseValue: { amount: "1000", currency: "USD" },
      },
    ]);
    previewChange.mockResolvedValue({
      activity: { id: "div-1", kind: "cash_dividend", effects: [] },
      effects: [],
      resulting: [{ target: "account_cash", name: "MooMoo", amount: "25", currency: "USD" }],
    });
    recordChange.mockResolvedValue({ activity: { id: "div-1", kind: "cash_dividend" }, effects: [], resulting: [] });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));
    const holdingRow = (await screen.findByText("XYZ Fund")).closest("tr");
    expect(holdingRow).not.toBeNull();
    await userEvent.click(within(holdingRow as HTMLElement).getByRole("button", { name: /Actions for/ }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Cash dividend" }));
    const form = await screen.findByRole("form", { name: "Record change" });
    expect(within(form).queryByLabelText("Holding")).not.toBeInTheDocument();
    await userEvent.type(within(form).getByLabelText("Amount"), "25");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    await waitFor(() =>
      expect(previewChange).toHaveBeenCalledWith(
        expect.objectContaining({ kind: "cash_dividend", holdingId: "holding-1", amount: "25" }),
      ),
    );
    await userEvent.click(within(form).getByRole("button", { name: "Confirm" }));
    await waitFor(() =>
      expect(recordChange).toHaveBeenCalledWith(
        expect.objectContaining({ kind: "cash_dividend", holdingId: "holding-1", amount: "25" }),
      ),
    );
  });

  it("opens cash dividend from the account action and only offers that account's holdings", async () => {
    const other = {
      account: { ...brokerageAccount.account, id: "brk-2", name: "Other Broker" },
      ownership: brokerageAccount.ownership,
      institutionName: "China Merchants Bank",
    };
    listAccounts.mockResolvedValue([brokerageAccount, other]);
    historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
    listInstruments.mockResolvedValue([{ id: "fund-1", name: "XYZ Fund", type: "mutual_fund", quoteCurrency: "USD", quoteSource: "manual" }]);
    holdingsByAccounts.mockResolvedValue({
      "brk-1": [{ id: "holding-1", accountId: "brk-1", instrumentId: "fund-1", quantity: "10" }],
      "brk-2": [{ id: "holding-2", accountId: "brk-2", instrumentId: "fund-1", quantity: "8" }],
    });
    accountValuations.mockResolvedValue([
      {
        account: brokerageAccount.account,
        ownership: brokerageAccount.ownership,
        complete: true,
        components: [
          { holdingId: "holding-1", instrumentId: "fund-1", instrumentName: "XYZ Fund", nativeAmount: "1000", nativeCurrency: "USD", available: true },
        ],
        missingInputs: [],
        baseValue: { amount: "1000", currency: "USD" },
      },
    ]);
    previewChange.mockResolvedValue({
      activity: { id: "div-1", kind: "cash_dividend", effects: [] },
      effects: [],
      resulting: [{ target: "account_cash", name: "MooMoo", amount: "12", currency: "USD" }],
    });
    recordChange.mockResolvedValue({ activity: { id: "div-1", kind: "cash_dividend" }, effects: [], resulting: [] });

    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));
    await screen.findByText("XYZ Fund");
    const accountAction = screen.getAllByRole("button", { name: "Cash dividend" }).find((button) => !button.closest("tr"));
    expect(accountAction).toBeDefined();
    await userEvent.click(accountAction as HTMLElement);
    const form = await screen.findByRole("form", { name: "Record change" });
    const holdingSelect = within(form).getByLabelText("Holding");
    expect(within(holdingSelect).getByRole("option", { name: /MooMoo/ })).toBeInTheDocument();
    expect(within(holdingSelect).queryByRole("option", { name: /Other Broker/ })).not.toBeInTheDocument();
    await userEvent.selectOptions(holdingSelect, "holding-1");
    await userEvent.type(within(form).getByLabelText("Amount"), "12");
    await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
    await waitFor(() =>
      expect(previewChange).toHaveBeenCalledWith(
        expect.objectContaining({ kind: "cash_dividend", holdingId: "holding-1", amount: "12" }),
      ),
    );
    await userEvent.click(within(form).getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(recordChange).toHaveBeenCalled());
  });
});

it("edits a logo override, then explicitly resets it to inheritance", async () => {
  const record = { ...emptyAccount, account: { ...emptyAccount.account, iconKey: "bank-logo:icbc", institutionId: "cmb" } };
  listAccounts.mockResolvedValue([record]);
  updateAccount.mockResolvedValue(record);
  renderPage();
  await userEvent.click(await screen.findByRole("button", { name: /Checking/ }));
  await userEvent.click(await screen.findByRole("button", { name: "Account settings" }));
  const form = await screen.findByRole("form", { name: "Account form" });
  await userEvent.click(within(form).getByRole("button", { name: /Details/i }));
  expect(within(form).getByLabelText("Choose icon").querySelector("img")).toHaveAttribute("src", "/bank-logos/display/icbc-rect.svg");
  await userEvent.click(within(form).getByRole("button", { name: "Use institution icon" }));
  await userEvent.click(screen.getByRole("button", { name: "Save" }));
  expect(updateAccount).toHaveBeenCalledWith("acc-1", expect.objectContaining({ iconKey: "", iconKeySet: true }));
});

it.each(["search", "result"])("Escape from icon %s closes only the picker and preserves the account Sheet", async (focusTarget) => {
  renderPage();
  await userEvent.click(await screen.findByRole("button", { name: /Checking/ }));
  await userEvent.click(await screen.findByRole("button", { name: "Account settings" }));
  const form = await screen.findByRole("form", { name: "Account form" });
  await userEvent.click(within(form).getByRole("button", { name: /Details/i }));
  await userEvent.clear(within(form).getByLabelText("Name"));
  await userEvent.type(within(form).getByLabelText("Name"), "Unsaved account name");
  const trigger = within(form).getByLabelText("Choose icon");
  await userEvent.click(trigger);
  await userEvent.selectOptions(within(form).getByLabelText("Icon category"), "bank");
  await userEvent.type(within(form).getByLabelText("Search icons"), "icbc");
  if (focusTarget === "result") within(form).getByRole("button", { name: "中国工商银行" }).focus();
  await userEvent.keyboard("{Escape}");
  expect(screen.getByRole("form", { name: "Account form" })).toBeInTheDocument();
  expect(within(form).getByLabelText("Name")).toHaveValue("Unsaved account name");
  expect(trigger).toHaveAttribute("aria-expanded", "false");
  expect(trigger).toHaveFocus();
  expect(updateAccount).not.toHaveBeenCalled();
  // Once the picker is closed, Escape keeps the Sheet's normal dismissal behavior.
  await userEvent.keyboard("{Escape}");
  expect(screen.queryByRole("form", { name: "Account form" })).not.toBeInTheDocument();
});

it("cash review supports back, edits, stale failure and cancel/reopen without writing", async () => {
  listAccounts.mockResolvedValue([brokerageAccount]);
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
  renderPage();
  await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));
  await userEvent.click(await screen.findByRole("button", { name: "Reconcile balance" }));
  let form = await screen.findByRole("form", { name: "Add cash balance" });
  await userEvent.type(within(form).getByLabelText("Resulting balance"), "900");
  await userEvent.type(within(form).getByLabelText("Reason (optional)"), "Statement");
  within(form).getByRole("button", { name: "Preview" }).focus();
  await userEvent.keyboard("{Enter}");
  const status = await within(form).findByRole("status");
  expect(status).toHaveTextContent("Current balance"); expect(status).toHaveTextContent("$800.00");
  expect(status).toHaveTextContent("$900.00"); expect(status).toHaveTextContent("+$100.00");
  expect(status).toHaveTextContent("USD"); expect(status).toHaveTextContent("12:00:00.000 (UTC)"); expect(status).toHaveTextContent("Statement");
  expect(confirmCash).not.toHaveBeenCalled();
  await userEvent.click(within(form).getByRole("button", { name: "Back to draft" }));
  expect(within(form).getByLabelText("Resulting balance")).toHaveValue("900");
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await within(form).findByRole("button", { name: "Confirm" });
  await userEvent.type(within(form).getByLabelText("Resulting balance"), "0");
  expect(within(form).queryByRole("button", { name: "Confirm" })).not.toBeInTheDocument();
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  confirmCash.mockRejectedValueOnce(new Error(JSON.stringify({ code: "stale_preview", field: "previewToken", message: "fresh preview required" })));
  await userEvent.click(await within(form).findByRole("button", { name: "Confirm" }));
  await within(form).findByRole("alert");
  expect(within(form).getByRole("button", { name: "Preview" })).toBeInTheDocument();
  expect(within(form).getByLabelText("Resulting balance")).toHaveValue("9000");
  await userEvent.keyboard("{Escape}");
  await waitFor(() => expect(screen.getByRole("button", { name: "Reconcile balance" })).toHaveFocus());
  await userEvent.click(await screen.findByRole("button", { name: "Reconcile balance" }));
  form = await screen.findByRole("form", { name: "Add cash balance" });
  expect(within(form).getByLabelText("Resulting balance")).toHaveValue("");
  expect(within(form).getByLabelText("Reason (optional)")).toHaveValue("");
  expect(within(form).queryByRole("button", { name: "Confirm" })).not.toBeInTheDocument();
  expect(appendCash).not.toHaveBeenCalled();
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Reconcile balance" })).toHaveFocus());
});

it("cash review shows origin-local time and exact sub-minor-unit values alongside currency formatting", async () => {
  listAccounts.mockResolvedValue([brokerageAccount]);
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "Asia/Shanghai" });
  previewCash.mockResolvedValue({ planId: "precision-plan", current: "10", target: "10.0001", delta: "0.0001", currency: "CNY", effectiveAt: "2026-10-10T16:30:00Z", timezone: "Asia/Shanghai" });
  renderPage();
  await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));
  await userEvent.click(await screen.findByRole("button", { name: "Reconcile balance" }));
  const form = await screen.findByRole("form", { name: "Add cash balance" });
  await userEvent.type(within(form).getByLabelText("Resulting balance"), "10.0001");
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  const review = await within(form).findByRole("status");
  expect(review).toHaveTextContent("CN¥10.00");
  expect(review).toHaveTextContent("10.0001 CNY");
  expect(review).toHaveTextContent("+CN¥0.00 (+0.0001 CNY)");
  expect(review).toHaveTextContent("Oct 11, 2026");
  expect(review).toHaveTextContent("00:30:00.000 (Asia/Shanghai)");
  expect(review).not.toHaveTextContent("2026-10-10T16:30:00Z");
  expect(confirmCash).not.toHaveBeenCalled();
});

it.each(["success", "uncertain"])("retains deferred cash confirmation across all dismissal routes (%s)", async outcome => {
  listAccounts.mockResolvedValue([brokerageAccount]);
  historyOrigin.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
  let resolve!: (value: object) => void;
  let reject!: (error: Error) => void;
  confirmCash.mockImplementationOnce(() => new Promise((done, fail) => { resolve = done; reject = fail; }));
  renderPage();
  await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));
  const trigger = await screen.findByRole("button", { name: "Reconcile balance" });
  await userEvent.click(trigger);
  const form = await screen.findByRole("form", { name: "Add cash balance" });
  await userEvent.type(within(form).getByLabelText("Resulting balance"), "900");
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await userEvent.click(await within(form).findByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(confirmCash).toHaveBeenCalledWith("cash-plan"));
  expect(screen.getByRole("button", { name: "Close" })).toBeDisabled();
  await userEvent.keyboard("{Escape}");
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  await userEvent.click(screen.getByRole("dialog").parentElement!.previousElementSibling!);
  expect(screen.getByRole("form", { name: "Add cash balance" })).toBe(form);
  if (outcome === "uncertain") {
    await act(async () => { reject(new Error("IPC outcome unknown")); });
    await within(form).findByRole("alert");
    await userEvent.keyboard("{Escape}");
    await userEvent.click(screen.getByRole("button", { name: "Close" }));
    await userEvent.click(screen.getByRole("dialog").parentElement!.previousElementSibling!);
    expect(screen.getByRole("form", { name: "Add cash balance" })).toBe(form);
    expect(within(form).getByLabelText("Resulting balance")).toBeDisabled();
    expect(within(form).getByRole("button", { name: "Back to draft" })).toBeDisabled();
    expect(previewCash).toHaveBeenCalledTimes(1);
    await userEvent.click(within(form).getByRole("button", { name: "Retry same confirmation" }));
    expect(confirmCash.mock.calls.map(([id]) => id)).toEqual(["cash-plan", "cash-plan"]);
  } else { await act(async () => { resolve({}); }); }
  await waitFor(() => expect(screen.queryByRole("form", { name: "Add cash balance" })).not.toBeInTheDocument());
  expect(confirmCash).toHaveBeenCalledTimes(outcome === "uncertain" ? 2 : 1);
  await waitFor(() => expect(trigger).toHaveFocus());
  await userEvent.click(trigger);
  expect(await screen.findByLabelText("Resulting balance")).toHaveValue("");
  await userEvent.keyboard("{Escape}");
});

it.each(["holdings", "balance", "manual_value"])("renders account currency primary in list and detail for %s", async (trackingMode) => {
  const record = { ...emptyAccount, account: { ...emptyAccount.account, trackingMode, defaultCurrency: "USD" } };
  listAccounts.mockResolvedValue([record]);
  accountValuations.mockResolvedValue([{
    account: record.account, ownership: record.ownership, complete: true, components: [], missingInputs: [],
    baseValue: { amount: "710", currency: "CNY" },
    displayValue: { currency: "USD", value: { amount: "100", currency: "USD" }, totalExact: "100", complete: true, valuedCount: 1, missingCount: 0, missingInputs: [], fxEvidence: [] },
  }]);
  render(<QueryClientProvider client={createTestQueryClient()}><AccountsPage /></QueryClientProvider>);
  const row = await screen.findByRole("button", { name: /Checking.*100\.00.*710\.00/ });
  expect(row.textContent!.indexOf("100.00")).toBeLessThan(row.textContent!.indexOf("710.00"));
  await userEvent.click(row);
  const detail = await screen.findByTestId("account-detail");
  const primary = within(detail).getByText("$100.00", { exact: true });
  const secondary = within(detail).getByText(/710\.00/);
  expect(primary.parentElement).toContainElement(secondary);
  expect(secondary.className).toContain("text-sm");
});

it.each([
  ["en", "USD total unavailable", "Valued subtotal"],
  ["zh-CN", "USD 合计暂不可用", "已估值小计"],
  ["zh-TW", "USD 合計暫不可用", "已估值小計"],
])("labels missing account FX and household partial amounts in %s", async (language, unavailable, subtotal) => {
  await i18n.changeLanguage(language);
  try {
    const record = { ...brokerageAccount, account: { ...brokerageAccount.account, defaultCurrency: "USD" } };
    listAccounts.mockResolvedValue([record]);
    accountValuations.mockResolvedValue([{
      account: record.account, ownership: record.ownership, complete: false, components: [], missingInputs: [],
      baseValue: { amount: "710", currency: "CNY" },
      displayValue: { currency: "USD", complete: false, valuedCount: 0, missingCount: 1, missingInputs: [], fxEvidence: [] },
    }]);
    renderPage();
    const row = await screen.findByRole("button", { name: /MooMoo.*710\.00/ });
    expect(within(row).getByText(unavailable)).toBeInTheDocument();
    expect(within(row).getByText(new RegExp(`710\\.00.*${subtotal}`))).toHaveClass("text-xs");
    await userEvent.click(row);
    const detail = await screen.findByTestId("account-detail");
    expect(within(detail).getByText(unavailable)).toHaveClass("text-2xl");
    expect(within(detail).getByText(new RegExp(`710\\.00.*${subtotal}`))).toHaveClass("text-sm");
  } finally { await i18n.changeLanguage("en"); }
});

it("keeps a priced holding's native amount visible when household FX is missing", async () => {
  listAccounts.mockResolvedValue([brokerageAccount]);
  accountValuations.mockResolvedValue([{
    account: brokerageAccount.account, ownership: brokerageAccount.ownership, complete: false,
    components: [{ holdingId: "native-holding", instrumentId: "native-instrument", instrumentName: "Native holding", nativeCurrency: "SGD", nativeAmount: "123", available: false }],
    missingInputs: [{ kind: "fx_rate", accountId: brokerageAccount.account.id, quoteCurrency: "SGD", baseCurrency: "USD" }],
    displayValue: { currency: "USD", complete: false, valuedCount: 0, missingCount: 1, missingInputs: [], fxEvidence: [] },
  }]);
  holdingsByAccounts.mockResolvedValue({ [brokerageAccount.account.id]: [{ id: "native-holding", accountId: brokerageAccount.account.id, instrumentId: "native-instrument", quantity: "1" }] });
  listInstruments.mockResolvedValue([{ id: "native-instrument", name: "Native holding", type: "stock", quoteCurrency: "SGD" }]);
  renderPage();
  await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));
  const row = await screen.findByRole("row", { name: /Native holding/ });
  expect(row).toHaveTextContent(/SGD\s*123\.00/);
  expect(row).not.toHaveTextContent("Missing price");
});


it.each(["deposit", "simple"])("retains ordinary account %s submission through child replacement and read failure", async entry => {
  historyOrigin.mockResolvedValue({ id: "origin", timezone: "UTC", startedAt: "2020-01-01T00:00:00Z" });
  listAccounts.mockResolvedValue([entry === "deposit" ? brokerageAccount : emptyAccount]);
  previewChange.mockResolvedValue({ resulting: [{ name: "Synthetic account", amount: "1125", currency: "USD" }] });
  let reject!: (error: Error) => void;
  recordChange.mockImplementationOnce(() => new Promise((_, fail) => { reject = fail; })).mockResolvedValue({});
  const { queryClient } = renderPage();
  await userEvent.click(await screen.findByRole("button", { name: entry === "deposit" ? /MooMoo/ : /Checking.*Bank account/ }));
  await userEvent.click(await screen.findByRole("button", { name: entry === "deposit" ? "Deposit or withdraw" : "Update balance" }));
  const form = await screen.findByRole("form", { name: "Record change" });
  const amount = within(form).getByLabelText(entry === "deposit" ? "Amount" : "New value");
  await userEvent.click(amount); await userEvent.keyboard("{Control>}a{/Control}1125");
  await userEvent.click(within(form).getByRole("button", { name: "Preview" }));
  await userEvent.click(await within(form).findByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(recordChange).toHaveBeenCalledTimes(1));
  const original = recordChange.mock.calls[0][0];
  if (entry === "deposit") {
    const removed = screen.getByRole("radio", { name: "Money removed" });
    expect(removed).toBeDisabled(); await userEvent.click(removed);
    expect(screen.getByRole("radio", { name: "Money added" })).toBeChecked();
  }
  await userEvent.keyboard("{Escape}");
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  expect(screen.getByRole("form", { name: "Record change" })).toBe(form);
  listAccounts.mockRejectedValue(new Error("background account read unavailable"));
  await act(async () => { await queryClient.invalidateQueries({ queryKey: queryKeys.accounts.all }); });
  expect(screen.getByRole("form", { name: "Record change" })).toBe(form);
  await act(async () => { reject(new Error("transport outcome unknown")); });
  const retry = await within(form).findByRole("button", { name: "Retry same submission" });
  expect(amount).toHaveValue("1125"); expect(amount).toBeDisabled();
  await userEvent.click(retry);
  await waitFor(() => expect(recordChange).toHaveBeenCalledTimes(2));
  expect(recordChange.mock.calls[1][0]).toEqual(original);
  await waitFor(() => expect(screen.queryByRole("form", { name: "Record change" })).not.toBeInTheDocument());
});
