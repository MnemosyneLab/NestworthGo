import { describe, expect, it } from "vitest";
import { TEST_CATALOG } from "@/test/catalog";
import {
  compatibleAccountTypes,
  defaultRole,
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
});
