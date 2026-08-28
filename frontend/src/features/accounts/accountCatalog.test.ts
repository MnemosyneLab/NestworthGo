import { describe, expect, it } from "vitest";
import { TEST_CATALOG } from "@/test/catalog";
import {
  compatibleAccountTypes,
  defaultRole,
  ownershipShares,
  roleLocked,
  splitAccountTypes,
  trackingPrompt,
} from "./accountCatalog";

describe("accountCatalog", () => {
  it("shows only catalog types, split into primary and more", () => {
    const { primary, more } = splitAccountTypes(TEST_CATALOG.accountTypes);
    expect(primary).toEqual(["bank_account", "brokerage", "credit_card", "other"]);
    expect(more).toEqual(["cash_on_hand"]);
    expect([...primary, ...more].sort()).toEqual([...TEST_CATALOG.accountTypes].sort());
  });

  it("asks bank accounts how to track and defaults to the account total", () => {
    const prompt = trackingPrompt(TEST_CATALOG.accountCombinations, "bank_account", "asset");
    expect(prompt.kind).toBe("ask");
    expect(prompt.options).toEqual(["balance", "holdings"]);
    expect(prompt.defaultMode).toBe("balance");
  });

  it("defaults brokerage to cash and holdings without exposing tracking terms", () => {
    const prompt = trackingPrompt(TEST_CATALOG.accountCombinations, "brokerage", "asset");
    expect(prompt.kind).toBe("advanced");
    expect(prompt.defaultMode).toBe("holdings");
    expect(prompt.options).toContain("manual_value");
  });

  it("locks credit-card role to liability from the catalog", () => {
    expect(defaultRole(TEST_CATALOG.accountCombinations, "credit_card")).toBe("liability");
    expect(roleLocked(TEST_CATALOG.accountCombinations, "credit_card")).toBe(true);
  });

  it("keeps compatible type updates inside the current role and tracking", () => {
    expect(
      compatibleAccountTypes(TEST_CATALOG.accountCombinations, TEST_CATALOG.accountTypes, "asset", "holdings", "brokerage"),
    ).toEqual(["bank_account", "brokerage"]);
  });

  it("does not invent household-wide shares from an empty owner list", () => {
    expect(ownershipShares([], undefined, false)).toEqual([]);
  });

  it("assigns 100% to a single checked owner", () => {
    expect(ownershipShares(["alice"], undefined, false)).toEqual([{ memberId: "alice", shareBps: 10000 }]);
  });

  it("even-splits among checked owners when percentages are blank", () => {
    expect(ownershipShares(["alice", "bob"], undefined, false)).toEqual([
      { memberId: "alice", shareBps: 5000 },
      { memberId: "bob", shareBps: 5000 },
    ]);
  });

  it("distributes the remainder so three owners still sum to 10000 bps", () => {
    expect(ownershipShares(["alice", "bob", "cara"], undefined, false)).toEqual([
      { memberId: "alice", shareBps: 3334 },
      { memberId: "bob", shareBps: 3333 },
      { memberId: "cara", shareBps: 3333 },
    ]);
  });

  it("keeps explicit custom percentages among checked owners", () => {
    expect(ownershipShares(["alice", "bob"], ["70", "30"], true)).toEqual([
      { memberId: "alice", shareBps: 7000 },
      { memberId: "bob", shareBps: 3000 },
    ]);
  });
});
