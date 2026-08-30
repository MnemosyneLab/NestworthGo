import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { queryKeys } from "@/queries/keys";
import { AccountsPage } from "./AccountsPage";
import { localDateInTimeZone } from "@/features/history/historyStartDate";

const listAccounts = vi.fn();
const accountValuations = vi.fn();
const createAccount = vi.fn();
const updateAccount = vi.fn();
const archiveAccount = vi.fn().mockResolvedValue(undefined);
const historyOrigin = vi.fn();
const startHistory = vi.fn();
const listInstruments = vi.fn();
const createInstrument = vi.fn();
const holdingsByAccounts = vi.fn();
const createHolding = vi.fn();
const appendCash = vi.fn();
const appendValue = vi.fn();
const previewChange = vi.fn();
const recordChange = vi.fn();
const settingsLoad = vi.fn();
const currentInstrumentQuote = vi.fn();
const listInstitutions = vi.fn();
const listGroups = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: {
    ListAccounts: (...args: unknown[]) => listAccounts(...args),
    AccountValuations: (...args: unknown[]) => accountValuations(...args),
    CreateAccount: (...args: unknown[]) => createAccount(...args),
    UpdateAccount: (...args: unknown[]) => updateAccount(...args),
    ArchiveAccount: (...args: unknown[]) => archiveAccount(...args),
    SetAccountIcon: vi.fn(),
    AppendAccountValue: (...args: unknown[]) => appendValue(...args),
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
    StartHistoryWithCosts: vi.fn(),
    StartingPointDraft: () => Promise.resolve([]),
    ListActivities: () => Promise.resolve([]),
    PreviewChange: (...args: unknown[]) => previewChange(...args),
    PreviewFixChange: vi.fn(),
    RecordChange: (...args: unknown[]) => recordChange(...args),
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
    CreateHolding: (...args: unknown[]) => createHolding(...args),
    AppendAccountCashValue: (...args: unknown[]) => appendCash(...args),
    ArchiveHolding: vi.fn(),
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
    RefreshInstrument: vi.fn(),
    RefreshFX: vi.fn(),
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
  listInstruments.mockReset();
  createInstrument.mockReset();
  holdingsByAccounts.mockReset();
  createHolding.mockReset();
  appendCash.mockReset();
  appendValue.mockReset();
  previewChange.mockReset();
  recordChange.mockReset();
  settingsLoad.mockReset();
  currentInstrumentQuote.mockReset();
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
  settingsLoad.mockResolvedValue({ timezone: "system", fx_provider: "frankfurter" });
  listInstruments.mockResolvedValue([]);
  holdingsByAccounts.mockResolvedValue({});
  createHolding.mockResolvedValue({ id: "h1", accountId: "brk-1", instrumentId: "i1", quantity: "1" });
  appendCash.mockResolvedValue(undefined);
  appendValue.mockResolvedValue(undefined);
  currentInstrumentQuote.mockResolvedValue(null);
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
    expect(screen.getByText("Pending conversion")).toBeInTheDocument();
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
        components: [],
        missingInputs: [],
        baseValue: { amount: "2500", currency: "USD" },
      },
    ]);
    renderPage();
    expect(await screen.findByText("MooMoo")).toBeInTheDocument();
    expect(screen.getByText("$2,500.00")).toBeInTheDocument();
  });

  it("creates a bank account through the wizard with equal ownership", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await continueWizard(form, 2);
    expect(within(form).getByText("How should this account be recorded?")).toBeInTheDocument();
    expect(within(form).getByText("Record the account total only")).toBeInTheDocument();
    await continueWizard(form, 1);
    await userEvent.type(within(form).getByLabelText("Name"), "New Savings");
    await userEvent.click(within(form).getByLabelText("Alice"));
    await userEvent.click(within(form).getByLabelText("Bob"));
    await userEvent.type(within(form).getByLabelText("Initial value"), "500");
    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("button", { name: "Add account" }));

    expect(createAccount).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "New Savings",
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

  it("defaults brokerage to cash and holdings without exposing tracking enums", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("button", { name: "Brokerage" }));
    await continueWizard(form, 1);
    expect(within(form).queryByText("How should this account be recorded?")).not.toBeInTheDocument();
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

  it("renders catalog-only account types on the type step", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await continueWizard(form, 1);
    expect(within(form).getByRole("button", { name: "Bank account" })).toBeInTheDocument();
    expect(within(form).getByRole("button", { name: "Brokerage" })).toBeInTheDocument();
    expect(within(form).getByRole("button", { name: "Credit card" })).toBeInTheDocument();
    expect(within(form).getByRole("button", { name: "Other" })).toBeInTheDocument();
    expect(within(form).queryByRole("button", { name: "Cash on hand" })).not.toBeInTheDocument();
    await userEvent.click(within(form).getByRole("button", { name: "More account types" }));
    expect(within(form).getByRole("button", { name: "Cash on hand" })).toBeInTheDocument();
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
        components: [
          { nativeCurrency: "USD", nativeAmount: "800", available: true },
          { nativeCurrency: "SGD", nativeAmount: "200", available: true },
          {
            holdingId: "h1",
            instrumentId: "i1",
            instrumentName: "NVIDIA",
            nativeCurrency: "USD",
            nativeAmount: "1500",
            available: true,
          },
        ],
        missingInputs: [{ kind: "fx_rate", accountId: "brk-1" }],
        baseValue: { amount: "2500", currency: "USD" },
      },
    ]);
    holdingsByAccounts.mockResolvedValue({
      "brk-1": [{ id: "h1", accountId: "brk-1", instrumentId: "i1", quantity: "10" }],
    });
    listInstruments.mockResolvedValue([{ id: "i1", name: "NVIDIA", type: "stock", quoteCurrency: "USD" }]);
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /MooMoo/ }));
    expect(await screen.findByTestId("account-detail")).toBeInTheDocument();
    expect(screen.getByText("Cash")).toBeInTheDocument();
    expect(screen.getAllByText("Investments").length).toBeGreaterThan(0);
    expect(screen.getAllByText("$800.00").length).toBeGreaterThan(0);
    expect(screen.getByText(/SGD\s*200\.00/)).toBeInTheDocument();
    expect(screen.getByText("NVIDIA")).toBeInTheDocument();
    expect(screen.getByText("Stock")).toBeInTheDocument();
    expect(screen.getByText("Partial valuation")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent(/Missing FX rate/);
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
    await userEvent.click(within(form).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(appendCash).toHaveBeenCalledWith("brk-1", "5000", "CNY", ""));
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
    expect(screen.getByLabelText("Start date")).toHaveValue(localDateInTimeZone(Intl.DateTimeFormat().resolvedOptions().timeZone) ?? "");
    const timezone = await screen.findByLabelText("Timezone");
    expect(timezone).toHaveValue(Intl.DateTimeFormat().resolvedOptions().timeZone);
    expect(timezone).toHaveAttribute("readonly");
    await userEvent.click(screen.getByRole("button", { name: "Start history" }));
    expect(await screen.findByLabelText("Instrument")).toBeInTheDocument();
    expect(startHistory).toHaveBeenCalledWith(Intl.DateTimeFormat().resolvedOptions().timeZone);
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
    appendCash.mockImplementation(async (_id: string, amount: string, currency: string) => {
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
    await userEvent.click(within(cashForm).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(appendCash).toHaveBeenCalledWith("cmb-mixed", "10000", "CNY", ""));
    expect((await screen.findAllByText(/CN¥\s*10,000\.00/)).length).toBeGreaterThan(0);

    await userEvent.click(await screen.findByRole("button", { name: "Buy investment" }));
    expect(await screen.findByText("Start history to continue")).toBeInTheDocument();
    expect(screen.getByLabelText("Start date")).toHaveValue(localDateInTimeZone(Intl.DateTimeFormat().resolvedOptions().timeZone) ?? "");
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
    const nameInput = within(form).getByLabelText("Name");
    await userEvent.clear(nameInput);
    await userEvent.type(nameInput, "Renamed");
    await userEvent.click(within(form).getByRole("button", { name: "Save" }));
    expect(updateAccount).toHaveBeenCalledWith(
      "acc-1",
      expect.objectContaining({ name: "Renamed" }),
    );
  });

  it("disables Save and does not update when every owner is unchecked", async () => {
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /Checking/ }));
    await userEvent.click(await screen.findByRole("button", { name: "Account settings" }));
    const form = await screen.findByRole("form", { name: "Account form" });
    await userEvent.click(within(form).getByLabelText("Alice"));
    const saveButton = within(form).getByRole("button", { name: "Save" });
    expect(saveButton).toBeDisabled();
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
    await userEvent.click(within(form).getByRole("button", { name: "Save" }));
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
    await waitFor(() => expect(screen.getByLabelText("Unit cost (optional)")).toHaveValue("12.50"));
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
    await userEvent.click(within(holdingRow as HTMLElement).getByRole("button", { name: "Cash dividend" }));
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
