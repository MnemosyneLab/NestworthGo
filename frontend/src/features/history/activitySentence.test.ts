import { describe, expect, it } from "vitest";
import { activitySentence } from "./activitySentence";
import type { ActivityDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

const catalog: Record<string, string> = {
  "history.sentence.added": "Added {{amount}} to {{account}}",
  "history.sentence.transferred": "Transferred {{amount}} from {{from}} to {{to}}",
  "history.sentence.bought": "Bought {{quantity}} {{instrument}} in {{account}} for {{amount}}",
  "history.sentence.sold": "Sold {{quantity}} {{instrument}} in {{account}} for {{amount}}",
  "history.sentence.updatedValueIncreased": "{{account}} increased {{delta}}",
  "history.sentence.updatedValueDecreased": "{{account}} decreased {{delta}}",
  "history.sentence.converted": "Converted {{sold}} to {{bought}} in {{account}}",
  "history.sentence.paidDebt": "Paid {{amount}} to {{account}}",
  "history.sentence.reversal": "Reversed a previous change",
  "history.sentence.withFee": "{{sentence}} (Fee {{fee}})",
  "history.sentence.withReason": "{{sentence}} ({{reason}})",
  "history.sentence.updatedValue": "Updated {{account}} to {{amount}}",
  "history.reason.reconciliation": "Reconciliation",
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

  it("describes money added in one sentence", () => {
    const activity = {
      kind: "cash_in",
      reason: "contribution",
      effects: [{ accountId: "acc-1", money: { amount: "1000", currency: "USD" } }],
    } as unknown as ActivityDTO;

    expect(activitySentence(t, activity, accounts, instruments)).toBe("Added $1,000.00 to Checking (Contribution)");
  });

  it("describes a cash transfer without exposing kind codes", () => {
    const activity = {
      kind: "cash_transfer",
      effects: [
        { role: "transfer_from", accountId: "acc-1", money: { amount: "200", currency: "USD" } },
        { role: "transfer_to", accountId: "acc-2", money: { amount: "200", currency: "USD" } },
      ],
    } as unknown as ActivityDTO;

    expect(activitySentence(t, activity, accounts, instruments)).toBe("Transferred $200.00 from Checking to Savings");
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

    expect(activitySentence(t, activity, accounts, instruments)).toBe("Bought 10 NVIDIA in Checking for $1,500.00");
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
    expect(activitySentence(t, activity, accounts, instruments)).toBe("Bought 10 NVIDIA in Checking for $1,500.00");
    expect(activitySentence(t, activity, accounts, instruments)).not.toMatch(/Principal/);
  });

  it("describes a value update using the resulting endpoint, not the delta", () => {
    const activity = {
      kind: "value_update",
      reason: "reconciliation",
      effects: [{ accountId: "acc-1", money: { amount: "20", currency: "USD" } }],
      resulting: [{ amount: "320", currency: "USD" }],
    } as unknown as ActivityDTO;
    expect(activitySentence(t, activity, accounts, instruments)).toBe("Updated Checking to $320.00 (Reconciliation)");
  });

  it("falls back to increased or decreased delta when resulting is missing", () => {
    const activity = {
      kind: "value_update",
      effects: [{ accountId: "acc-1", money: { amount: "20", currency: "USD" } }],
    } as unknown as ActivityDTO;
    expect(activitySentence(t, activity, accounts, instruments)).toBe("Checking increased $20.00");
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
    expect(activitySentence(t, trade, accounts, instruments)).toBe("Bought 10 NVIDIA in Checking for $1,500.00 (Fee $5.00)");

    const fx = {
      kind: "fx_conversion",
      effects: [
        { role: "transfer_from", accountId: "acc-1", money: { amount: "100", currency: "USD" } },
        { role: "transfer_to", accountId: "acc-1", money: { amount: "90", currency: "EUR" } },
        { role: "fee", accountId: "acc-1", money: { amount: "1", currency: "USD" } },
      ],
    } as unknown as ActivityDTO;
    expect(activitySentence(t, fx, accounts, instruments)).toBe("Converted $100.00 to €90.00 in Checking (Fee $1.00)");
  });

  it("describes a reversal without requiring effect data", () => {
    const activity = { kind: "reversal", reversesActivityId: "a1", effects: [] } as unknown as ActivityDTO;
    expect(activitySentence(t, activity, accounts, instruments)).toBe("Reversed a previous change");
  });
});
