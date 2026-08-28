import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { AccountsPage } from "./AccountsPage";
import { localDateInTimeZone } from "@/features/history/historyStartDate";

const listAccounts = vi.fn();
const accountValuations = vi.fn();
const createAccount = vi.fn();
const updateAccount = vi.fn();
const archiveAccount = vi.fn().mockResolvedValue(undefined);
const setAccountLogo = vi.fn().mockResolvedValue(undefined);
const pickImage = vi.fn();
const createMediaAsset = vi.fn();
const historyOrigin = vi.fn();
const startHistory = vi.fn();
const listInstruments = vi.fn();
const createInstrument = vi.fn();
const holdingsByAccounts = vi.fn();
const createHolding = vi.fn();
const appendCash = vi.fn();
const previewChange = vi.fn();
const recordChange = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: {
    ListAccounts: (...args: unknown[]) => listAccounts(...args),
    AccountValuations: (...args: unknown[]) => accountValuations(...args),
    CreateAccount: (...args: unknown[]) => createAccount(...args),
    UpdateAccount: (...args: unknown[]) => updateAccount(...args),
    ArchiveAccount: (...args: unknown[]) => archiveAccount(...args),
    SetAccountLogo: (...args: unknown[]) => setAccountLogo(...args),
    AppendAccountValue: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/media", () => ({
  Service: {
    PickImage: (...args: unknown[]) => pickImage(...args),
    CreateMediaAsset: (...args: unknown[]) => createMediaAsset(...args),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/directory", () => ({
  Service: {
    ListMembers: () => Promise.resolve([{ id: "alice", name: "Alice" }, { id: "bob", name: "Bob" }]),
    ListInstitutions: () => Promise.resolve([{ id: "cmb", name: "China Merchants Bank" }]),
    ListGroups: () => Promise.resolve([]),
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
  Service: { SupportedCurrencies: () => Promise.resolve(["USD", "SGD", "CNY"]) },
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

function renderPage() {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <AccountsPage />
    </QueryClientProvider>,
  );
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
  setAccountLogo.mockClear();
  pickImage.mockReset();
  createMediaAsset.mockReset();
  historyOrigin.mockReset();
  startHistory.mockReset();
  listInstruments.mockReset();
  createInstrument.mockReset();
  holdingsByAccounts.mockReset();
  createHolding.mockReset();
  appendCash.mockReset();
  previewChange.mockReset();
  recordChange.mockReset();
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
  pickImage.mockResolvedValue("");
  createMediaAsset.mockResolvedValue({ id: "media-1" });
  historyOrigin.mockResolvedValue(null);
  startHistory.mockResolvedValue({ id: "origin-1", timezone: "UTC" });
  listInstruments.mockResolvedValue([]);
  holdingsByAccounts.mockResolvedValue({});
  createHolding.mockResolvedValue({ id: "h1", accountId: "brk-1", instrumentId: "i1", quantity: "1" });
  appendCash.mockResolvedValue(undefined);
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
    expect(screen.getByText("$1,000.00")).toBeInTheDocument();
    expect(screen.getByText("Bank account")).toBeInTheDocument();
    expect(screen.getByText("Complete")).toBeInTheDocument();
  });

  it("does not label a still-loading valuation as complete", async () => {
    accountValuations.mockReturnValue(new Promise(() => {}));
    renderPage();
    expect(await screen.findByText("Checking")).toBeInTheDocument();
    expect(screen.getByText("Loading")).toBeInTheDocument();
    expect(screen.queryByText("Complete")).not.toBeInTheDocument();
    expect(screen.getByText("No current value")).toBeInTheDocument();
  });

  it("does not label a missing valuation as complete", async () => {
    accountValuations.mockResolvedValue([]);
    renderPage();
    expect(await screen.findByText("Checking")).toBeInTheDocument();
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
    expect(within(form).getByLabelText("Include in portfolio")).toBeChecked();
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
    expect(screen.getByText("$800.00")).toBeInTheDocument();
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
    await userEvent.clear(timezone);
    await userEvent.type(timezone, "UTC");
    expect(screen.getByLabelText("Start date")).toHaveValue(localDateInTimeZone("UTC") ?? "");
    await userEvent.click(screen.getByRole("button", { name: "Start history" }));
    expect(await screen.findByLabelText("Instrument")).toBeInTheDocument();
    expect(startHistory).toHaveBeenCalledWith("UTC");
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
    expect(await screen.findByText(/CN¥\s*10,000\.00/)).toBeInTheDocument();

    await userEvent.click(await screen.findByRole("button", { name: "Buy investment" }));
    expect(await screen.findByText("Start history to continue")).toBeInTheDocument();
    expect(screen.getByLabelText("Start date")).toHaveValue(localDateInTimeZone(Intl.DateTimeFormat().resolvedOptions().timeZone) ?? "");
    const timezone = screen.getByLabelText("Timezone");
    await userEvent.clear(timezone);
    await userEvent.type(timezone, "UTC");
    await userEvent.click(screen.getByRole("button", { name: "Start history" }));

    const tradeForm = await screen.findByRole("form", { name: "Record change" });
    await userEvent.selectOptions(within(tradeForm).getByLabelText("Instrument"), "fund-1");
    await userEvent.type(within(tradeForm).getByLabelText("Quantity"), "100");
    await userEvent.type(within(tradeForm).getByLabelText("Gross total"), "2000");
    await userEvent.click(within(tradeForm).getByRole("button", { name: "Preview" }));
    await userEvent.click(await within(tradeForm).findByRole("button", { name: "Confirm" }));

    expect(await screen.findByText(/CN¥\s*8,000\.00/)).toBeInTheDocument();
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
    expect(within(form).getByLabelText("Include in portfolio")).toBeChecked();
    await userEvent.selectOptions(within(form).getByLabelText("Account type"), "bank_account");
    expect(within(form).getByText("Asset")).toBeInTheDocument();
    expect(within(form).getByText(/Record cash and holdings separately/)).toBeInTheDocument();
    expect(within(form).getByLabelText("Include in portfolio")).toBeChecked();
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

  it("keeps account creation successful when logo persistence fails", async () => {
    pickImage.mockResolvedValue("cGlj");
    createMediaAsset.mockRejectedValueOnce(new Error("image storage unavailable"));
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Create account" });
    await continueWizard(form, 3);
    await userEvent.click(within(form).getByRole("button", { name: "More settings" }));
    await userEvent.click(within(form).getByRole("button", { name: "Set image" }));
    await userEvent.type(within(form).getByLabelText("Name"), "With Logo Failure");
    await userEvent.click(within(form).getByLabelText("Alice"));
    await continueWizard(form, 1);
    await userEvent.click(within(form).getByRole("button", { name: "Add account" }));
    await waitFor(() => expect(createMediaAsset).toHaveBeenCalledWith("image/png", "cGlj"));
    expect(createAccount).toHaveBeenCalledTimes(1);
    expect(setAccountLogo).not.toHaveBeenCalled();
  });
});
