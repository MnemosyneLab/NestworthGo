import { expect, it } from "vitest";
import i18n from "@/i18n";
import { checkpointValue, sourceText } from "./reconciliationDisplay";

it("uses frozen quantity units for holdings and currency for cash", () => {
  expect(checkpointValue(i18n.t, { kind: "holding", value: "100", currency: "USD" })).toBe("100");
  expect(checkpointValue(i18n.t, { kind: "holding", value: "1.234567", currency: "USD", detail: { quantityUnit: "g" } })).toBe("1.234567 g");
  expect(checkpointValue(i18n.t, { kind: "cash", value: "100", currency: "USD" })).toContain("USD");
});

it("joins only available source metadata", () => {
  expect(sourceText({ reference: "", asOfDate: "" })).toBe("");
  expect(sourceText({ reference: "", asOfDate: "2026-10-14" })).toBe("2026-10-14");
  expect(sourceText({ reference: "Statement", asOfDate: "2026-10-14", asOfTime: "12:00", timezone: "UTC" })).toBe("Statement · 2026-10-14 12:00 UTC");
});
