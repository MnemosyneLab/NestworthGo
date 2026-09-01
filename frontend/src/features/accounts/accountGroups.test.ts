import { describe, expect, it } from "vitest";
import { groupAccounts, UNASSIGNED_INSTITUTION } from "./accountGroups";
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
