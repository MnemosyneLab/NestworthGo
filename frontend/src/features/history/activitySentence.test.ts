import { describe, expect, it } from "vitest";
import { activitySentence } from "./activitySentence";
import type { ActivityDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

const catalog: Record<string, string> = {
  "history.sentence.added": "Added {{amount}} to {{account}}",
  "history.sentence.transferred": "Transferred {{amount}} from {{from}} to {{to}}",
  "history.sentence.bought": "Bought {{quantity}} {{instrument}} for {{amount}}",
  "history.sentence.reversal": "Reversed a previous change",
  "history.sentence.withReason": "{{sentence}} ({{reason}})",
  "history.reason.contribution": "Contribution",
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
    } as unknown as ActivityDTO;

    expect(activitySentence(t, activity, accounts, instruments)).toBe("Bought 10 NVIDIA for $1,500.00");
  });

  it("describes a reversal without requiring effect data", () => {
    const activity = { kind: "reversal", reversesActivityId: "a1", effects: [] } as unknown as ActivityDTO;
    expect(activitySentence(t, activity, accounts, instruments)).toBe("Reversed a previous change");
  });
});
