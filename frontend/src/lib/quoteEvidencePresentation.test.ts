import { expect, it } from "vitest";
import i18n from "@/i18n";
import { quoteSourceLabel } from "./quoteEvidencePresentation";

it.each(["en"])("names Yahoo Finance without exposing its technical key (%s)", language => {
  const t = i18n.getFixedT(language);
  const label = quoteSourceLabel(t, "provider", "yahoo_finance");
  expect(label).toBe(`${t("portfolio.provider")} · Yahoo Finance`);
  expect(label).not.toContain("yahoo_finance");
});

it("preserves Agent identity and hides unknown source keys", () => {
  const t = i18n.getFixedT("en");
  expect(quoteSourceLabel(t, "agent", "agent")).toBe(t("portfolio.agent"));
  expect(quoteSourceLabel(t, "provider", "opaque_source_v1")).toBe(t("portfolio.provider"));
});
