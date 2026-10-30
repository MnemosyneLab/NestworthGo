import { describe, expect, it } from "vitest";
import { createInstance } from "i18next";
import i18n from "@/i18n";
import { activitySentence } from "./activitySentence";
import type { ActivityDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

const catalog: Record<string, string> = {
  "history.sentence.added": "Added {{amount}} to {{account}}",
  "history.sentence.transferred": "Transferred {{amount}} from {{from}} to {{to}}",
  "history.sentence.bought": "Bought {{quantity}} {{instrument}} in {{account}} for {{amount}}",
  "history.sentence.sold": "Sold {{quantity}} {{instrument}} in {{account}} for {{amount}}",
  "history.sentence.liabilityValueIncreased": "{{account}} signed liability balance increased {{delta}}",
  "history.sentence.liabilityValueDecreased": "{{account}} signed liability balance decreased {{delta}}",
  "history.sentence.updatedValueIncreased": "{{account}} increased {{delta}}",
  "history.sentence.updatedValueDecreased": "{{account}} decreased {{delta}}",
  "history.sentence.converted": "Converted {{sold}} to {{bought}} in {{account}}",
  "history.sentence.paidDebt": "Paid {{amount}} to {{account}}",
  "history.sentence.dividend": "Dividend {{amount}} from {{instrument}} into {{account}}",
  "history.sentence.reversal": "Reversed a previous change",
  "history.sentence.withFee": "{{sentence}} (Fee {{fee}})",
  "history.sentence.withReason": "{{sentence}} ({{reason}})",
  "history.sentence.updatedCost": "Set {{holding}} unit cost to {{cost}}",
  "history.sentence.updatedValue": "Updated {{account}} to {{amount}}",
  "history.reason.reconciliation": "Reconciliation",
  "history.reason.contribution": "Contribution",
  "common.unknownValue": "Unknown",
  "history.unknownAccount": "an account",
  "history.unknownInstrument": "an instrument",
  "history.kind.cash_in": "Money added",
};

function t(key: string, options?: Record<string, unknown>): string {
  const template = catalog[key] ?? (typeof options?.defaultValue === "string" ? options.defaultValue : key);
  if (!options) {
    return template;
  }
  return Object.entries(options).reduce((value, [name, replacement]) => value.replace(new RegExp(`{{${name}}}`, "g"), String(replacement)), template);
}

describe("activitySentence", () => {
  const accounts = new Map([["acc-1", "Checking"], ["acc-2", "Savings"]]);
  const instruments = new Map([["i1", "NVIDIA"]]);

  it("describes cost correction without implying a trade or quantity movement", () => {
    const activity = {
      kind: "cost_adjustment",
      reason: "reconciliation",
      effects: [{ target: "holding_cost", holdingId: "h1", instrumentId: "i1", costUnitPrice: "30" }],
    } as unknown as ActivityDTO;
    expect(activitySentence(t, activity, accounts, instruments)).toBe("Set NVIDIA unit cost to 30 (Reconciliation)");
  });

  it("describes money added in one sentence", () => {
    const activity = {
      kind: "cash_in",
      reason: "contribution",
      effects: [{ accountId: "acc-1", money: { amount: "1000", currency: "USD" } }],
    } as unknown as ActivityDTO;

    expect(activitySentence(t, activity, accounts, instruments)).toBe("Added US$1,000.00 to Checking (Contribution)");
  });

  it("describes a cash transfer without exposing kind codes", () => {
    const activity = {
      kind: "cash_transfer",
      effects: [
        { role: "transfer_from", accountId: "acc-1", money: { amount: "200", currency: "USD" } },
        { role: "transfer_to", accountId: "acc-2", money: { amount: "200", currency: "USD" } },
      ],
    } as unknown as ActivityDTO;

    expect(activitySentence(t, activity, accounts, instruments)).toBe("Transferred US$200.00 from Checking to Savings");
    expect(activitySentence(t, activity, accounts, instruments)).not.toMatch(/cash_transfer/);
  });

  it("describes a buy using trade detail", () => {
    const activity = {
      kind: "buy",
      tradeDetail: {
        side: "buy",
        instrumentId: "i1",
        quantity: "10",
        gross: { amount: "1500", currency: "USD" },
      },
      effects: [{ role: "principal", accountId: "acc-1" }],
    } as unknown as ActivityDTO;

    expect(activitySentence(t, activity, accounts, instruments)).toBe("Bought 10 NVIDIA in Checking for US$1,500.00");
  });

  it("describes a cash dividend without appending income reason", () => {
    const activity = {
      kind: "cash_dividend",
      reason: "income",
      dividendDetail: { holdingId: "h1", instrumentId: "i1", amount: { amount: "12.5", currency: "USD" } },
      effects: [{ accountId: "acc-1", money: { amount: "12.5", currency: "USD" } }],
    } as unknown as ActivityDTO;
    expect(activitySentence(t, activity, accounts, instruments)).toBe("Dividend US$12.50 from NVIDIA into Checking");
    expect(activitySentence(t, activity, accounts, instruments)).not.toMatch(/Income/);
  });

  it("does not append principal reason for buy or sell", () => {
    const activity = {
      kind: "buy",
      reason: "principal",
      tradeDetail: {
        side: "buy",
        instrumentId: "i1",
        quantity: "10",
        gross: { amount: "1500", currency: "USD" },
      },
      effects: [{ role: "principal", accountId: "acc-1" }],
    } as unknown as ActivityDTO;
    expect(activitySentence(t, activity, accounts, instruments)).toBe("Bought 10 NVIDIA in Checking for US$1,500.00");
    expect(activitySentence(t, activity, accounts, instruments)).not.toMatch(/Principal/);
  });

  it("describes a value update using the resulting endpoint, not the delta", () => {
    const activity = {
      kind: "value_update",
      reason: "reconciliation",
      effects: [{ accountId: "acc-1", money: { amount: "20", currency: "USD" } }],
      resulting: [{ amount: "320", currency: "USD" }],
    } as unknown as ActivityDTO;
    expect(activitySentence(t, activity, accounts, instruments)).toBe("Updated Checking to US$320.00 (Reconciliation)");
  });

  it("falls back to increased or decreased delta when resulting is missing", () => {
    const activity = {
      kind: "value_update",
      effects: [{ accountId: "acc-1", money: { amount: "20", currency: "USD" } }],
    } as unknown as ActivityDTO;
    expect(activitySentence(t, activity, accounts, instruments)).toBe("Checking increased US$20.00");
  });

  it("keeps the signed exact delta when a tiny value decrease rounds to zero", () => {
    const activity = {
      kind: "value_update",
      effects: [{ accountId: "acc-1", money: { amount: "-0.0001", currency: "USD" } }],
    } as unknown as ActivityDTO;
    expect(activitySentence(t, activity, accounts, instruments)).toBe("Checking decreased US$0.00 · − nonzero, below US$0.01 (-0.0001 USD)");
  });

  it("includes trade and FX fees in the sentence", () => {
    const trade = {
      kind: "buy",
      tradeDetail: {
        side: "buy",
        instrumentId: "i1",
        quantity: "10",
        gross: { amount: "1500", currency: "USD" },
        fee: { amount: "5", currency: "USD" },
      },
      effects: [{ role: "principal", accountId: "acc-1" }],
    } as unknown as ActivityDTO;
    expect(activitySentence(t, trade, accounts, instruments)).toBe("Bought 10 NVIDIA in Checking for US$1,500.00 (Fee US$5.00)");

    const fx = {
      kind: "fx_conversion",
      effects: [
        { role: "transfer_from", accountId: "acc-1", money: { amount: "100", currency: "USD" } },
        { role: "transfer_to", accountId: "acc-1", money: { amount: "90", currency: "EUR" } },
        { role: "fee", accountId: "acc-1", money: { amount: "1", currency: "USD" } },
      ],
    } as unknown as ActivityDTO;
    expect(activitySentence(t, fx, accounts, instruments)).toBe("Converted US$100.00 to €90.00 in Checking (Fee US$1.00)");
  });

  it("describes a reversal without requiring effect data", () => {
    const activity = { kind: "reversal", reversesActivityId: "a1", effects: [] } as unknown as ActivityDTO;
    expect(activitySentence(t, activity, accounts, instruments)).toBe("Reversed a previous change");
  });
});

describe.each([
  ["en", "Card purchase US$100.00 on Synthetic card", "Card refund US$100.00 on Synthetic card"],
  ["zh-CN", "在 Synthetic card 刷卡消费 US$100.00", "Synthetic card 收到消费退款 US$100.00"],
  ["zh-TW", "在 Synthetic card 刷卡消費 US$100.00", "Synthetic card 收到消費退款 US$100.00"],
] as const)("card activity with real %s resources", (locale, purchase, refund) => {
  it.each([
    ["cash_in", "card_purchase", purchase],
    ["cash_out", "card_refund", refund],
  ])("interpolates %s / %s without fallback or synthetic translations", async (kind, reason, expected) => {
    const instance = createInstance();
    await instance.init({ lng: locale, fallbackLng: false, resources: { [locale]: { translation: i18n.getResourceBundle(locale, "translation") } }, interpolation: { escapeValue: false } });
    const names = new Map([["card", "Synthetic card"]]);
    const common = { effects: [{ accountId: "card", money: { amount: "100", currency: "USD" } }] };
    expect(activitySentence(instance.t.bind(instance), { ...common, kind, reason } as unknown as ActivityDTO, names, new Map())).toBe(expected);
  });
});

it.each([false, true])("uses removed magnitude direction for value-update fallback (liability=%s)", liability => {
 const activity = { kind: "value_update", effects: [{ accountId: "acc-1", direction: "removed", money: {amount:"20",currency:"USD"} }], resulting: [] } as unknown as ActivityDTO;
 const sentence = activitySentence(t,activity,new Map([["acc-1","Synthetic account"]]),new Map(),new Map(),new Set(liability ? ["acc-1"] : []));
 expect(sentence).toBe(`Synthetic account ${liability ? "signed liability balance increased" : "decreased"} US$20.00`);
});
