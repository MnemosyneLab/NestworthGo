import { afterEach, expect, it } from "vitest";
import i18n from "@/i18n";
import { displayEnum } from "./display";

afterEach(async () => { await i18n.changeLanguage("en"); });

it("uses a translated fallback instead of a raw code", async () => {
  expect(displayEnum(i18n.t, "enum", "not_a_real_code")).toBe("Unknown");
  await i18n.changeLanguage("zh-CN");
  expect(displayEnum(i18n.t, "enum", "not_a_real_code")).toBe("未知");
  await i18n.changeLanguage("zh-TW");
  expect(displayEnum(i18n.t, "enum", "not_a_real_code")).toBe("未知");
});
