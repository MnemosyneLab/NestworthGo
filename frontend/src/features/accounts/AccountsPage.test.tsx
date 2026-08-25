import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AccountsPage } from "./AccountsPage";

const listAccounts = vi.fn();
const createAccount = vi.fn();
const archiveAccount = vi.fn().mockResolvedValue(undefined);

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: {
    ListAccounts: (...args: unknown[]) => listAccounts(...args),
    CreateAccount: (...args: unknown[]) => createAccount(...args),
    ArchiveAccount: (...args: unknown[]) => archiveAccount(...args),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/directory", () => ({
  Service: {
    ListMembers: () => Promise.resolve([{ id: "alice", name: "Alice" }, { id: "bob", name: "Bob" }]),
    ListInstitutions: () => Promise.resolve([]),
    ListGroups: () => Promise.resolve([]),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({
  Service: { SupportedCurrencies: () => Promise.resolve(["USD"]) },
}));

function renderPage() {
  const queryClient = new QueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <AccountsPage />
    </QueryClientProvider>,
  );
}

const emptyAccount = {
  account: { id: "acc-1", name: "Checking", primaryCategory: "cash_equivalent", defaultCurrency: "USD", sortOrder: 0, includeInNetWorth: true, includeInInvestment: false, includeInLiquidAssets: false, createdAt: "", updatedAt: "" },
  ownership: [{ memberId: "alice", shareBps: 10000 }],
  latestValue: { id: "v1", accountId: "acc-1", valueKind: "balance", amount: { amount: "1000", currency: "USD" }, effectiveAt: "", createdAt: "" },
};

beforeEach(() => {
  listAccounts.mockReset();
  createAccount.mockReset();
  archiveAccount.mockClear();
  listAccounts.mockResolvedValue([emptyAccount]);
  createAccount.mockResolvedValue(emptyAccount);
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
        ownerIds: ["alice", "bob"],
        ownershipPercentages: undefined,
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
      expect.objectContaining({ ownerIds: ["alice", "bob"], ownershipPercentages: ["70", "30"] }),
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
});
