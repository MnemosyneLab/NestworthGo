import { describe, expect, it } from "vitest";
import { accountGroupNet, groupAccounts, UNASSIGNED_INSTITUTION } from "./accountGroups";
import type { AccountDTO, AccountRecordDTO, AccountValuationDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

function account(id: string, name: string, institutionId?: string): AccountDTO {
  return {
    id,
    householdId: "household-1",
    institutionId: institutionId ?? null,
    groupId: null,
    name,
    accountType: "bank_account",
    balanceSheetRole: "asset",
    trackingMode: "balance",
    defaultCurrency: "USD",
    note: null,
    iconKey: "bank",
    includeInNetWorth: true,
    includeInPortfolio: false,
    includeInLiquidAssets: true,
    openedOn: null,
    closedOn: null,
    sortOrder: 0,
    createdAt: "",
    updatedAt: "",
    archivedAt: null,
  };
}

function record(id: string, name: string, institutionId?: string, institutionName?: string): AccountRecordDTO {
  const result: AccountRecordDTO = {
    account: account(id, name, institutionId),
    ownership: [],
  };
  if (institutionName !== undefined) {
    result.institutionName = institutionName;
  }
  return result;
}

function valuation(id: string, amount: string): AccountValuationDTO {
  return {
    account: account(id, `Account ${id}`),
    ownership: [],
    complete: true,
    composition: { currency: "USD", complete: true, status: "unsupported", valuedCount: 0, missingCount: 0, types: [], details: [], detailSlices: [] },
    components: [],
    missingInputs: [],
    baseValue: { amount, currency: "USD" },
  };
}

describe("groupAccounts", () => {
  it("sorts institutions and accounts by household amount and keeps unassigned last", () => {
    const grouped = groupAccounts(
      [
        record("a1", "Alpha Checking", "bank", "Alpha Bank"),
        record("a2", "Alpha Savings", "bank", "Alpha Bank"),
        record("b1", "Brokerage", "broker", "Zeta Broker"),
        record("c1", "Cash on hand"),
      ],
      [
        valuation("a1", "100"),
        valuation("a2", "200"),
        valuation("b1", "500"),
        valuation("c1", "900"),
      ],
    );
    expect(grouped.map((group) => group.key)).toEqual(["broker", "bank", UNASSIGNED_INSTITUTION]);
    expect(grouped[1]?.records.map((item) => item.account.name)).toEqual(["Alpha Savings", "Alpha Checking"]);
  });
});

it("uses net signed group totals and suppresses incomplete or mixed-currency totals", () => {
  const records = [{ account: { id: "bank", name: "Synthetic bank", balanceSheetRole: "asset" } }, { account: { id: "card", name: "Synthetic card", balanceSheetRole: "liability" } }] as AccountRecordDTO[];
  const valuations = new Map([ ["bank", { complete: true, baseValue: { amount: "100", currency: "USD" } }], ["card", { complete: true, baseValue: { amount: "150", currency: "USD" } }] ]) as Map<string, AccountValuationDTO>;
  expect(accountGroupNet(records, valuations)).toEqual({ amount: "-50", currency: "USD" });
  expect(accountGroupNet(records.slice(1), valuations)?.amount).toBe("-150");
  valuations.get("bank")!.complete = false;
  expect(accountGroupNet(records, valuations)).toBeUndefined();
  valuations.get("bank")!.complete = true;
  valuations.get("bank")!.baseValue!.currency = "EUR";
  expect(accountGroupNet(records, valuations)).toBeUndefined();
  valuations.delete("bank");
  expect(accountGroupNet(records, valuations)).toBeUndefined();
});

it("sorts by signed net and uses normalized base amounts across currencies and archived filters", () => {
  const asset = record("bank", "Synthetic bank", "mixed", "Synthetic institution");
  const debt = record("card", "Synthetic card", "mixed", "Synthetic institution");
  debt.account.balanceSheetRole = "liability"; debt.account.defaultCurrency = "EUR";
  const other = record("other", "Synthetic other", "other", "Other institution");
  const values = [valuation("bank", "100"), valuation("card", "150"), valuation("other", "0")];
  const groups = groupAccounts([debt, asset, other], values);
  expect(groups.map(group => group.key)).toEqual(["other", "mixed"]);
  expect(groups[1].records.map(row => row.account.id)).toEqual(["bank", "card"]);
  const map = new Map(values.map(value => [value.account.id, value]));
  expect(accountGroupNet([asset, debt], map)?.amount).toBe("-50");
  // Group totals cover exactly the caller's filtered account set.
  debt.account.archivedAt = "2026-08-01T00:00:00Z";
  expect(accountGroupNet([asset], map)?.amount).toBe("100");
  expect(accountGroupNet([debt], map)?.amount).toBe("-150");
});
