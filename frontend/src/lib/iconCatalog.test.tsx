import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ICON_CATALOG, iconChoice, iconChoicesByCategory } from "./iconCatalog";
import { EntityIcon, hasIcon } from "@/components/icons/EntityIcon";
import i18n from "@/i18n";

describe("shared icon registry", () => {
  it("adds only twelve choices and renders every registered item in all locales", () => {
    expect(ICON_CATALOG).toHaveLength(87);
    expect(new Set(ICON_CATALOG.map(x => x.key)).size).toBe(87);
    for (const choice of ICON_CATALOG) {
      expect(hasIcon(choice.key)).toBe(true);
      const {container, unmount} = render(<EntityIcon iconKey={choice.key} kind="instrument" />);
      expect(container.querySelector(choice.file ? "img" : "svg")).not.toBeNull();
      for (const lng of ["en", "zh-CN", "zh-TW"]) expect(i18n.exists(choice.labelKey, {lng})).toBe(true);
      unmount();
    }
    const groups = iconChoicesByCategory();
    expect(groups[groups.length - 1]?.categoryKey).toBe("icons.category.general");
  });
  it.each([
    ["vault", "vault"], ["percent", "percent"], ["badge-percent", "badge-percent"],
    ["circle-percent", "circle-percent"], ["calendar-clock", "calendar-clock"],
    ["wallet-cards", "wallet-cards"], ["badge-franc", "badge-swiss-franc"],
    ["badge-rupee", "badge-indian-rupee"], ["badge-ruble", "badge-russian-ruble"],
    ["badge-yen", "badge-japanese-yen"], ["badge-pound", "badge-pound-sterling"],
  ])("renders stored %s with its correct glyph", (key, glyph) => {
    const {container} = render(<EntityIcon iconKey={key} kind="account" />);
    expect(container.querySelector(`.lucide-${glyph}`)).not.toBeNull();
  });
  it("keeps the existing bitcoin line icon and requires explicit branding", () => {
    const {container} = render(<EntityIcon iconKey="bitcoin" kind="instrument" />);
    expect(container.querySelector(".lucide-bitcoin")).not.toBeNull();
    expect(iconChoice("BTC")).toBeUndefined();
    expect(iconChoice("crypto-logo:btc")?.file).toBe("crypto-logos/btc.svg");
    expect(hasIcon("crypto-logo:../../unknown")).toBe(false);
  });
});
