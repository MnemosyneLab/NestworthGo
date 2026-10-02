import { describe, it, expect } from "vitest";
import { resolveAccountIcon } from "./accountIcons";

describe("account icon resolution", () => {
  const institutions = [{ id: "a", iconKey: "bank-logo:icbc" }, { id: "b", iconKey: "bank-logo:boc" }];
  it("inherits current institution, responds to institution/icon changes, and falls back by account type", () => {
    const account = { accountType: "bank_account", iconKey: "", institutionId: "a" };
    expect(resolveAccountIcon(account, institutions)).toBe("bank-logo:icbc");
    expect(resolveAccountIcon({ ...account, institutionId: "b" }, institutions)).toBe("bank-logo:boc");
    expect(resolveAccountIcon(account, [{ id: "a", iconKey: "wallet" }])).toBe("wallet");
    expect(resolveAccountIcon({ ...account, institutionId: undefined }, institutions)).toBe("bank");
    expect(resolveAccountIcon({ ...account, institutionId: "missing" }, institutions)).toBe("bank");
  });
  it("preserves explicit generic, logo, and unknown IDs rather than silently rewriting them", () => {
    for (const iconKey of ["wallet", "bank-logo:boc", "legacy-unknown"]) {
      expect(resolveAccountIcon({ iconKey, accountType: "bank_account", institutionId: "a" }, institutions)).toBe(iconKey);
    }
  });
});
