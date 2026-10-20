import { expect, it } from "vitest";
import i18n from "@/i18n";
import { checkpointReference, checkpointValue, sourceText } from "./reconciliationDisplay";

it("uses frozen quantity units for holdings and currency for cash", () => {
  expect(checkpointValue(i18n.t, { kind: "holding", value: "100", currency: "USD" })).toBe("100");
  expect(checkpointValue(i18n.t, { kind: "holding", value: "1.234567", currency: "USD", detail: { quantityUnit: "g" } })).toBe("1.234567 g");
  expect(checkpointValue(i18n.t, { kind: "cash", value: "100", currency: "USD" })).toContain("USD");
  expect(checkpointValue(i18n.t, { kind: "cash", value: "100.001", currency: "USD" }, true)).toBe("$100.00 (100.001 USD)");
});

it("formats the ledger value and the source figure with the same currency format", () => {
  const item = { kind: "cash", value: "800", referenceValue: "790", currency: "EUR" };
  expect([checkpointValue(i18n.t, item, true), checkpointReference(i18n.t, item, true)].join(" · ")).toBe("€800.00 · €790.00");
  expect(checkpointReference(i18n.t, { ...item, referenceValue: "1234.5" }, true)).toBe("€1,234.50");
  expect(checkpointReference(i18n.t, { ...item, referenceValue: "" }, true)).toBe("");
  expect(checkpointReference(i18n.t, { kind: "scope", referenceValue: "1" })).toBe("1");
  expect(checkpointReference(i18n.t, { kind: "scope", referenceValue: "3 holdings" }, true)).toBe("3 holdings");
});

it("keeps holding quantities exact and unit-labelled for the source figure", () => {
  expect(checkpointReference(i18n.t, { kind: "holding", value: "1", referenceValue: "1.234567", detail: { quantityUnit: "g" } }, true)).toBe("1.234567 g");
});

it("joins only available source metadata", () => {
  expect(sourceText({ reference: "", asOfDate: "" })).toBe("");
  expect(sourceText({ reference: "", asOfDate: "2026-10-14" })).toBe("2026-10-14");
  expect(sourceText({ reference: "Statement", asOfDate: "2026-10-14", asOfTime: "12:00", timezone: "UTC" })).toBe("Statement · 2026-10-14 12:00 UTC");
});
