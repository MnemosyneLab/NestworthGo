import { describe, it, expect, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { InstrumentHoldingsTable } from "./InstrumentHoldingsTable";
import type { InstrumentHoldingsDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics/models";

const amounts = (quantity: string, value: string, currency = "USD") => ({ quantity, totalCost: { amount: "100", currency }, currentValue: { amount: value, currency }, unrealizedGain: { amount: "-10", currency } });
function fixture(): InstrumentHoldingsDTO {
  return { instrumentId: "i1", name: "Apple", symbol: "AAPL", quoteCurrency: "USD", quantityUnit: "", archived: false,
    amounts: amounts("3", "900"), holdings: [
      { holdingId: "h1", accountId: "a1", accountName: "Broker A", amounts: amounts("1", "300") },
      { holdingId: "h2", accountId: "a2", accountName: "Broker B", amounts: amounts("2", "600") },
      { holdingId: "h0", accountId: "a0", accountName: "Closed account position", amounts: amounts("0", "0") },
    ] };
}
describe("InstrumentHoldingsTable", () => {
  it("shows authoritative totals, expands accounts by keyboard, and navigates to the account", async () => {
    const onOpenAccount = vi.fn();
    render(<InstrumentHoldingsTable groups={[fixture()]} onOpenAccount={onOpenAccount} />);
    expect(screen.getByRole("button", { name: "By instrument" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByText("2 accounts")).toBeInTheDocument();
    expect(screen.getByText("$900.00")).toBeInTheDocument();
    const expand = screen.getByRole("button", { name: "Show accounts for Apple" });
    expand.focus();
    await userEvent.keyboard("{Enter}");
    expect(expand).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByText("$300.00")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Broker B" }));
    expect(onOpenAccount).toHaveBeenCalledWith("a2");
    await userEvent.click(screen.getByRole("button", { name: "Account details" }));
    expect(screen.queryByText("$900.00")).not.toBeInTheDocument();
    expect(screen.queryByText("Closed account position")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("checkbox", { name: "Show zero holdings" }));
    expect(screen.getByRole("button", { name: "Closed account position" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "By instrument" }));
    expect(screen.getByText("3 accounts")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show accounts for Apple" })).toHaveAttribute("aria-expanded", "true");
  });
  it("keeps an all-zero group accessible through the filter and labels archived metal units", async () => {
    const group = fixture(); group.name = "Gold"; group.archived = true; group.quantityUnit = "g";
    group.amounts = amounts("0", "0"); group.holdings = [group.holdings![2]];
    render(<InstrumentHoldingsTable groups={[group]} />);
    expect(screen.queryByTestId("holdings-table")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("checkbox"));
    expect(screen.getByText("Archived")).toBeInTheDocument();
    expect(screen.getByTestId("holdings-table")).toHaveTextContent("0 g");
  });
  it("keeps unavailable fields separate and exposes valid account values", async () => {
    const group = fixture();
    group.amounts = { quantity: "3", totalCost: { amount: "200", currency: "USD" }, valueMissingReason: "current instrument price is unavailable", gainMissingReason: "current instrument price is unavailable" };
    render(<InstrumentHoldingsTable groups={[group]} />);
    expect(screen.getByText("$200.00")).toBeInTheDocument();
    expect(screen.getAllByText("No current price")).toHaveLength(2);
    await userEvent.click(screen.getByRole("button", { name: "Show accounts for Apple" }));
    expect(screen.getByText("$300.00")).toBeInTheDocument();
  });
  it("sorts amounts within currencies, keeps missing last, and does not merge identical names", async () => {
    const usd = fixture(); usd.name = "Z USD";
    const cny = { ...fixture(), name: "A CNY", instrumentId: "cny", quoteCurrency: "CNY", amounts: amounts("3", "9999", "CNY") };
    const missing = { ...fixture(), name: "Missing", instrumentId: "missing", amounts: { quantity: "3" } };
    const sameName = { ...fixture(), name: "Z USD", instrumentId: "other", amounts: amounts("3", "1") };
    render(<InstrumentHoldingsTable groups={[usd, cny, missing, sameName]} />);
    const table = screen.getByTestId("holdings-table");
    const names = () => [...table.querySelectorAll("tbody tr")].map(r => r.querySelector("td")?.textContent);
    expect(names()).toEqual(["AAPLA CNY", "AAPLMissing", "AAPLZ USD", "AAPLZ USD"]);
    await userEvent.click(within(table).getByRole("button", { name: "Current value" }));
    expect(names()).toEqual(["AAPLA CNY", "AAPLZ USD", "AAPLZ USD", "AAPLMissing"]);
    const values = () => [...table.querySelectorAll("tbody tr")].map(r => r.children[4].textContent);
    expect(values()[1]).toBe("$1.00");
    await userEvent.click(within(table).getByRole("button", { name: "Current value" }));
    expect(values()[1]).toBe("$900.00");
    expect(names()[3]).toBe("AAPLMissing");
  });
});
