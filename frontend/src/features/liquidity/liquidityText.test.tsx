import { afterEach, describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import i18n from "@/i18n";
import { ProductPreview } from "./ProductPreview";
import { sourceName } from "./sourceName";
import type { LiquiditySourceDTO, ProductOperationPreviewDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
afterEach(async () => { await i18n.changeLanguage("en"); });
describe("liquidity disclosures", () => {
  it.each(["en", "zh-CN", "zh-TW"])("translates preview warning and assumptions in %s", async (language) => {
    await i18n.changeLanguage(language);
    render(<ProductPreview preview={{ effectiveAt: "2026-09-22T00:00:00Z", timezone: "UTC", warnings: ["existing_position_increases_assets"], assumptions: ["current_prices_and_fx", "based_on_current_assets", "outstanding_debt_not_subtracted"] } as ProductOperationPreviewDTO} />);
    const content = screen.getByTestId("product-preview");
    expect(content).not.toHaveTextContent("current_prices_and_fx");
    expect(content).not.toHaveTextContent("existing_position_increases_assets");
    expect(content).toHaveTextContent(i18n.t("availableFunds.outstanding_debt_not_subtracted"));
    if (language !== "en") expect(content).not.toHaveTextContent("Recorded assets increase");
    const source = { sourceRef: { kind: "account_cash" }, displayName: "QA Bank cash" } as LiquiditySourceDTO;
    expect(sourceName(i18n.t, source)).toBe(i18n.t("availableFunds.cashSource", { name: "QA Bank" }));
  });
});
