import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import i18n from "@/i18n";
import { activitySentence } from "./activitySentence";
import { ActivityDetailSheet } from "./ActivityDetailSheet";
import type { ActivityDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
const accounts = new Map([["a", "Bank"]]);
const instruments = new Map([["i", "Deposit"]]);
function fixture(purpose = "acquisition", productKind = "term_deposit") {
  return {kind: "buy", effectiveAt: "2026-09-22T04:00:00Z", reason: "reconciliation", productContext: {purpose, productKind, instrumentId: "i"}, tradeDetail: {quantity: "1", unitPrice: "500", gross: {amount: "500", currency: "CNY"}}, effects: [{role: "principal", accountId: "a", money: {amount: "500", currency: "CNY"}}]} as unknown as ActivityDTO;
}
describe("managed product history", () => {
  it.each(["en", "zh-CN", "zh-TW"])("uses product language for every recorded purpose in %s", (locale) => {
    const t = i18n.getFixedT(locale);
    for (const productKind of ["term_deposit", "locked_product"]) {
      for (const purpose of ["acquisition", "existing_position", "redemption", "interest", "reversal"]) {
        const text = activitySentence(t, fixture(purpose, productKind), accounts, instruments);
        expect(text).toContain("Deposit");
        expect(text).not.toMatch(/history\.|买入 1|買入 1|Bought 1|Reconciliation|對帳|对账/);
      }
    }
  });
  it("describes a deposit with principal and preserves fees", () => {
    const activity = fixture();
    activity.tradeDetail!.fee = {amount: "2", currency: "CNY"};
    expect(activitySentence(i18n.getFixedT("zh-CN"), activity, accounts, instruments)).toBe("在Bank开立定期存款「Deposit」· 本金 CN¥500.00（费用 CN¥2.00）");
  });
  it("keeps unknown amounts unknown and does not invent principal for an existing position", () => {
    const activity = fixture(); activity.tradeDetail = null; activity.effects = [];
    expect(activitySentence(i18n.getFixedT("en"), activity, accounts, instruments)).toContain("Unknown");
    activity.productContext!.purpose = "existing_position";
    expect(activitySentence(i18n.getFixedT("en"), activity, accounts, instruments)).not.toContain("Principal");
  });
  it("shows business fields instead of a synthetic quantity and unit price", () => {
    render(<ActivityDetailSheet activity={fixture()} timezone="UTC" accounts={accounts} instruments={instruments} onClose={() => {}} t={i18n.getFixedT("en")} />);
    expect(screen.getByText("Principal: CN¥500.00")).toBeInTheDocument();
    expect(screen.queryByText(/^Quantity:/)).not.toBeInTheDocument();
    expect(screen.queryByText(/^Unit price:/)).not.toBeInTheDocument();
  });
});
