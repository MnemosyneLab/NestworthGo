import { describe, expect, it } from "vitest";
import { BANK_LOGOS, BANK_LOGO_CHOICES, bankLogo, bankLogoPresentation } from "./bankLogos";
import { resolveAccountIcon } from "./accountIcons";

describe("reviewed bank presentations", () => {
  it("covers every stable ID once without removing original catalog entries", () => {
    expect(BANK_LOGOS).toHaveLength(608);
    expect(BANK_LOGO_CHOICES).toHaveLength(304);
    const keys = BANK_LOGO_CHOICES.flatMap(logo => [logo.key, ...logo.legacyKeys]);
    expect(new Set(keys).size).toBe(608);
    for (const logo of BANK_LOGOS) {
      expect(bankLogo(logo.key)).toBe(logo);
      expect(bankLogoPresentation(logo.key)?.file).toMatch(/^display\/[a-z0-9-]+\.svg$/);
    }
  });
  it("prefers reviewed symbols but retains a single wordmark-only fallback", () => {
    expect(bankLogoPresentation("bank-logo:icbc")).toMatchObject({ key: "bank-logo:icbc-rect", kind: "symbol" });
    expect(bankLogoPresentation("bank-logo:citibank")).toMatchObject({ key: "bank-logo:citibank-rect", kind: "wordmark" });
    expect(BANK_LOGO_CHOICES.filter(x => x.aliases.includes("icbc"))).toHaveLength(1);
    expect(bankLogoPresentation("bank-logo:unknown")).toBeUndefined();
  });
  it("keeps old stored overrides distinct from inheritance while sharing the presentation", () => {
    const institutions = [{id: "a", iconKey: "bank-logo:icbc"}];
    const account = { accountType: "bank_account", institutionId: "a", iconKey: "" };
    const inherited = resolveAccountIcon(account, institutions);
    const explicit = resolveAccountIcon({...account, iconKey: "bank-logo:boc"}, institutions);
    expect(inherited).toBe("bank-logo:icbc");
    expect(explicit).toBe("bank-logo:boc");
    expect(bankLogoPresentation(inherited)?.key).toBe("bank-logo:icbc-rect");
    expect(bankLogoPresentation(explicit)?.key).toBe("bank-logo:boc-rect");
    expect(account.iconKey).toBe("");
  });
});
