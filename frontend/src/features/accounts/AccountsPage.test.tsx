import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { AccountsPage } from "./AccountsPage";

const listAccounts = vi.fn();
const accountValuations = vi.fn();
const createAccount = vi.fn();
const updateAccount = vi.fn();
const archiveAccount = vi.fn().mockResolvedValue(undefined);
const setAccountLogo = vi.fn().mockResolvedValue(undefined);
const pickImage = vi.fn();
const createMediaAsset = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: {
    ListAccounts: (...args: unknown[]) => listAccounts(...args),
    AccountValuations: (...args: unknown[]) => accountValuations(...args),
    CreateAccount: (...args: unknown[]) => createAccount(...args),
    UpdateAccount: (...args: unknown[]) => updateAccount(...args),
    ArchiveAccount: (...args: unknown[]) => archiveAccount(...args),
    SetAccountLogo: (...args: unknown[]) => setAccountLogo(...args),
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
    ListInstitutions: () => Promise.resolve([]),
    ListGroups: () => Promise.resolve([]),
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
  Service: { SupportedCurrencies: () => Promise.resolve(["USD"]) },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});

function renderPage() {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <AccountsPage />
    </QueryClientProvider>,
  );
}

const emptyAccount = {
  account: { id: "acc-1", name: "Checking", primaryCategory: "cash_equivalent", secondaryCategory: "bank_account", trackingMode: "balance", defaultCurrency: "USD", sortOrder: 0, includeInNetWorth: true, includeInInvestment: false, includeInLiquidAssets: false, createdAt: "", updatedAt: "" },
  ownership: [{ memberId: "alice", shareBps: 10000 }],
  latestValue: { id: "v1", accountId: "acc-1", valueKind: "balance", amount: { amount: "1000", currency: "USD" }, effectiveAt: "", createdAt: "" },
};

beforeEach(() => {
  listAccounts.mockReset();
  accountValuations.mockReset();
  createAccount.mockReset();
  updateAccount.mockReset();
  archiveAccount.mockClear();
  setAccountLogo.mockClear();
  pickImage.mockReset();
  createMediaAsset.mockReset();
  listAccounts.mockResolvedValue([emptyAccount]);
  accountValuations.mockResolvedValue([
    { account: emptyAccount.account, ownership: emptyAccount.ownership, complete: true, components: [], missingInputs: [], baseValue: { amount: "1000", currency: "USD" } },
  ]);
  createAccount.mockResolvedValue(emptyAccount);
  updateAccount.mockResolvedValue(emptyAccount);
  pickImage.mockResolvedValue("");
  createMediaAsset.mockResolvedValue({ id: "media-1" });
});

describe("AccountsPage", () => {
  it("renders the empty state when there are no accounts", async () => {
    listAccounts.mockResolvedValue([]);
    renderPage();
    expect(await screen.findByText("No accounts yet")).toBeInTheDocument();
  });

  it("lists existing accounts with their current value", async () => {
    renderPage();
    expect(await screen.findByText("Checking")).toBeInTheDocument();
    expect(screen.getByText("$1,000.00")).toBeInTheDocument();
  });

  it("shows computed valuation for holdings accounts without a stored latestValue", async () => {
    const holdingsAccount = {
      account: { ...emptyAccount.account, id: "inv-1", name: "Brokerage", trackingMode: "holdings" },
      ownership: emptyAccount.ownership,
    };
    listAccounts.mockResolvedValue([holdingsAccount]);
    accountValuations.mockResolvedValue([
      {
        account: holdingsAccount.account,
        ownership: holdingsAccount.ownership,
        complete: true,
        components: [],
        missingInputs: [],
        baseValue: { amount: "2500", currency: "USD" },
      },
    ]);
    renderPage();
    expect(await screen.findByText("Brokerage")).toBeInTheDocument();
    expect(screen.getByText("$2,500.00")).toBeInTheDocument();
  });

  it("creates an account with minimal required fields (equal ownership split)", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Account form" });
    await userEvent.type(within(form).getByLabelText("Name"), "New Savings");
    await userEvent.click(within(form).getByLabelText("Alice"));
    await userEvent.click(within(form).getByLabelText("Bob"));
    await userEvent.type(within(form).getByLabelText("Initial value"), "500");
    await userEvent.click(within(form).getByRole("button", { name: "Add account" }));

    expect(createAccount).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "New Savings",
        ownership: [{ memberId: "alice", shareBps: 5000 }, { memberId: "bob", shareBps: 5000 }],
        initialAmount: "500",
      }),
    );
  });

  it("creates an account with explicit ownership percentages", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Account form" });
    await userEvent.type(within(form).getByLabelText("Name"), "Joint");
    await userEvent.click(within(form).getByLabelText("Alice"));
    await userEvent.click(within(form).getByLabelText("Bob"));
    await userEvent.click(within(form).getByText(/split evenly if blank/i));
    await userEvent.type(within(form).getByLabelText("Alice ownership percentage"), "70");
    await userEvent.type(within(form).getByLabelText("Bob ownership percentage"), "30");
    await userEvent.type(within(form).getByLabelText("Initial value"), "100");
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

  it("defaults includeInInvestment when the category is Investment", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Account form" });
    await userEvent.type(within(form).getByLabelText("Name"), "Brokerage");
    await userEvent.selectOptions(within(form).getByLabelText("Category"), "investment");
    expect(within(form).getByLabelText("Include in investment")).toBeChecked();
    await userEvent.click(within(form).getByLabelText("Alice"));
    await userEvent.click(within(form).getByRole("button", { name: "Add account" }));
    expect(createAccount).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "Brokerage",
        primaryCategory: "investment",
        includeInInvestment: true,
      }),
    );
  });

  it("archives an active account after confirming the AlertDialog", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByRole("button", { name: "Archive" }));
    const dialog = await screen.findByRole("alertdialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Archive" }));
    expect(archiveAccount).toHaveBeenCalledWith("acc-1", true);
  });

  it("edits an existing account's name through UpdateAccount", async () => {
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByRole("button", { name: "Edit" }));
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

  it("persists a picked logo after creating an account", async () => {
    pickImage.mockResolvedValue("cGlj");
    createMediaAsset.mockResolvedValue({ id: "media-1" });
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Account form" });
    await userEvent.click(within(form).getByRole("button", { name: /details/i }));
    await userEvent.click(within(form).getByRole("button", { name: "Set image" }));
    await userEvent.type(within(form).getByLabelText("Name"), "With Logo");
    await userEvent.click(within(form).getByLabelText("Alice"));
    await userEvent.click(within(form).getByRole("button", { name: "Add account" }));
    expect(createAccount).toHaveBeenCalled();
    await waitFor(() => {
      expect(createMediaAsset).toHaveBeenCalledWith("image/png", "cGlj");
      expect(setAccountLogo).toHaveBeenCalledWith("acc-1", "media-1");
    });
  });

  it("keeps the account creation successful when logo persistence partially fails", async () => {
    pickImage.mockResolvedValue("cGlj");
    createMediaAsset.mockRejectedValueOnce(new Error("image storage unavailable")).mockResolvedValue({ id: "media-1" });
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Account form" });
    await userEvent.click(within(form).getByRole("button", { name: /details/i }));
    await userEvent.click(within(form).getByRole("button", { name: "Set image" }));
    await userEvent.type(within(form).getByLabelText("Name"), "With Logo Failure");
    await userEvent.click(within(form).getByLabelText("Alice"));
    await userEvent.click(within(form).getByRole("button", { name: "Add account" }));

    await waitFor(() => expect(createMediaAsset).toHaveBeenCalledWith("image/png", "cGlj"));
    expect(createAccount).toHaveBeenCalled();
    expect(setAccountLogo).not.toHaveBeenCalled();
    expect(createAccount).toHaveBeenCalledTimes(1);

    const table = await screen.findByTestId("accounts-table");
    expect(within(table).getByRole("alert")).toHaveTextContent(/try the image again/i);
    pickImage.mockResolvedValue("cGlj");
    const pickerButtons = within(table).getAllByRole("button", { name: "Set image" });
    await userEvent.click(pickerButtons[pickerButtons.length - 1]);
    await userEvent.click(within(table).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(setAccountLogo).toHaveBeenCalledWith("acc-1", "media-1"));
    expect(createAccount).toHaveBeenCalledTimes(1);
  });

  it("renders category options from the catalog only", async () => {
    const { TEST_CATALOG, selectValues } = await import("@/test/catalog");
    renderPage();
    await screen.findByText("Checking");
    await userEvent.click(screen.getByText("Add account"));
    const form = await screen.findByRole("form", { name: "Account form" });
    await waitFor(() => {
      expect(selectValues(within(form).getByLabelText("Category"))).toEqual(TEST_CATALOG.primaryCategories);
    });
  });
});
