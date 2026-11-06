import { expect, it } from "vitest";
import i18n from "@/i18n";
import { toolDisplayName } from "./toolDisplayName";

it.each([
  ["en", "Activity history", "Tool call"],
  ["zh-CN", "账务记录", "工具调用"],
  ["zh-TW", "帳務紀錄", "工具呼叫"],
])("localizes known and unknown tool names in %s", async (language, activities, fallback) => {
  await i18n.changeLanguage(language);
  expect(toolDisplayName(i18n.t.bind(i18n), "list_activities")).toBe(activities);
  for (const unknown of ["future_tool", "toString", "source.get_overview", ""]) {
    expect(toolDisplayName(i18n.t.bind(i18n), unknown)).toBe(fallback);
  }
});
