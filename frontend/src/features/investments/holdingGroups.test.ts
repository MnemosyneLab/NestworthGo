import { describe, expect, it } from "vitest";
import { amountDirection, normalizeHoldingGroupKey, sortHoldingRows, visibleHoldingGroups, HOLDING_GROUP_ORDER, type HoldingSort } from "./holdingGroups";
import type { HoldingAmountsDTO, InstrumentHoldingGroupDTO, InstrumentHoldingsDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics/models";

const amounts = (quantity: string, value: string | null, currency = "USD"): HoldingAmountsDTO => ({
  quantity, totalCost: { amount: "100", currency }, ...(value !== null ? { currentValue: { amount: value, currency }, unrealizedGain: { amount: "1", currency } } : {}),
});
function instrument(id: string, name: string, type: string, members: Array<[string, string, string, (string | null)?]>, currency = "USD"): InstrumentHoldingsDTO {
  return { instrumentId: id, type, name, symbol: id.toUpperCase(), quoteCurrency: currency, quantityUnit: "", archived: false,
    amounts: amounts(members.reduce((sum, member) => String(Number(sum) + Number(member[1])), "0"), "10", currency),
    holdings: members.map(([holdingId, quantity, accountId, value]) => ({ holdingId, accountId, accountName: `Account ${accountId}`, amounts: amounts(quantity, value === undefined ? "10" : value, currency) })) };
}
const group = (key: string, instruments: InstrumentHoldingsDTO[]): InstrumentHoldingGroupDTO => ({ key, instruments, subtotals: [{ currency: "USD", instrumentCount: instruments.length, totalCost: { amount: "100", currency: "USD" } }] });
const byName: HoldingSort = { key: "name", desc: false };
const options = { view: "instrument" as const, showZero: false, sort: byName, language: "en" };

describe("holding group presentation", () => {
  it("keeps the backend default order and drops empty groups", () => {
    const groups = [group("crypto", [instrument("btc", "Bitcoin", "crypto", [["h1", "1", "a"]])]), group("stock", [instrument("aapl", "Apple", "stock", [["h2", "1", "a"]])]), group("bond", [])];
    expect(visibleHoldingGroups(groups, options).map((item) => item.key)).toEqual(["stock", "crypto"]);
    expect(HOLDING_GROUP_ORDER[0]).toBe("stock");
  });

  it("maps unknown backend keys to Other and merges them", () => {
    const groups = [group("other", [instrument("x", "Xylo", "other", [["h1", "1", "a"]])]), group("mystery", [instrument("m", "Mystery", "mystery", [["h2", "1", "a"]])])];
    const result = visibleHoldingGroups(groups, options);
    expect(normalizeHoldingGroupKey("mystery")).toBe("other");
    expect(result).toHaveLength(1);
    expect(result[0].rows.map((row) => row.instrument.name)).toEqual(["Mystery", "Xylo"]);
    expect(result[0].subtotals).toHaveLength(2);
  });

  it("hides zero-quantity positions until requested and removes groups that become empty", () => {
    const groups = [group("precious_metal", [instrument("au", "Gold", "precious_metal", [["h1", "0", "a"]])]), group("stock", [instrument("aapl", "Apple", "stock", [["h2", "2", "a"], ["h3", "0", "b"]])])];
    const hidden = visibleHoldingGroups(groups, options);
    expect(hidden.map((item) => item.key)).toEqual(["stock"]);
    expect(hidden[0].rows[0].instrument.holdings).toHaveLength(1);
    expect(visibleHoldingGroups(groups, { ...options, showZero: true }).map((item) => item.key)).toEqual(["stock", "precious_metal"]);
  });

  it("builds per-holding rows for the account view while counting instruments once", () => {
    const groups = [group("stock", [instrument("aapl", "Apple", "stock", [["h1", "1", "a"], ["h2", "2", "b"]])])];
    const [stock] = visibleHoldingGroups(groups, { ...options, view: "account" });
    expect(stock.instrumentCount).toBe(1);
    expect(stock.rows.map((row) => [row.id, row.accountName, row.holdingId])).toEqual([["holding:h1", "Account a", "h1"], ["holding:h2", "Account b", "h2"]]);
  });

  it("passes backend subtotals through unchanged", () => {
    const groups = [group("stock", [instrument("aapl", "Apple", "stock", [["h1", "1", "a"]])])];
    expect(visibleHoldingGroups(groups, options)[0].subtotals).toStrictEqual(groups[0].subtotals);
  });

  it("sorts within a currency, keeps unknown values last in both directions, and is stable for equal names", () => {
    const rows = visibleHoldingGroups([group("stock", [
      instrument("a", "Alpha", "stock", [["h1", "1", "a", "50"]]),
      instrument("b", "Beta", "stock", [["h2", "1", "a", "900"]]),
      instrument("c", "Gamma", "stock", [["h3", "1", "a", null]]),
      instrument("z", "Zeta", "stock", [["h4", "1", "a", "5"]], "CNY"),
    ])], { ...options, view: "account" })[0].rows;
    const names = (sort: HoldingSort) => sortHoldingRows(rows, sort, "en").map((row) => row.instrument.name);
    expect(names({ key: "currentValue", desc: false })).toEqual(["Zeta", "Alpha", "Beta", "Gamma"]);
    expect(names({ key: "currentValue", desc: true })).toEqual(["Zeta", "Beta", "Alpha", "Gamma"]);
  });

  it("derives arrow direction from the displayed rounding", () => {
    expect(amountDirection("12.5")).toBe("up");
    expect(amountDirection("-0.01")).toBe("down");
    expect(amountDirection("0.004")).toBe("flat");
    expect(amountDirection("-0.004")).toBe("flat");
    expect(amountDirection(undefined)).toBeUndefined();
  });
});
