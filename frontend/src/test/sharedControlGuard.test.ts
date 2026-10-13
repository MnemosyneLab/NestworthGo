import { ESLint } from "eslint";
import { describe, expect, it } from "vitest";

const eslint = new ESLint();
describe("shared-control lint boundary", () => {
  it.each([
    '<input type="date" />', '<Input type={"datetime-local"} />',
    '<input type="time" />', '<select />', '<dialog />',
    ...["dialog", "alertdialog"].flatMap(role => [
      `<div role="${role}" />`, `<div role={"${role}"} />`, `<div role={'${role}'} />`,
    ]),
    'import { DayPicker } from "react-day-picker"; void DayPicker;',
    'import { Calendar } from "@/components/ui/calendar"; void Calendar;',
    'import { Dialog } from "@base-ui/react/dialog"; void Dialog;',
  ])("rejects bypass: %s", async source => {
    const [result] = await eslint.lintText(source, { filePath: "src/features/guard-fixture.tsx" });
    expect(result.messages.some(message => message.ruleId?.startsWith("no-restricted"))).toBe(true);
  });
  it.each([
    '<div role={"status"} />',
    'const role: string = "dialog"; <div role={role} />',
  ])("leaves non-dialog roles and dynamic aliases to the documented policy: %s", async source => {
    const [result] = await eslint.lintText(source, { filePath: "src/features/guard-fixture.tsx" });
    expect(result.errorCount).toBe(0);
  });
  it.each(["src/features/guard-fixture.tsx", "src/components/ui/guard-fixture.tsx", "src/features/guard-fixture.test.tsx"])("permits canonical controls and deliberate fixture/internal exceptions: %s", async filePath => {
    const source = filePath.includes("fixture.tsx") && filePath.includes("features")
      ? '<DatePicker value="" onChange={() => {}} />'
      : '<><input type="date" /><div role={"dialog"} /><div role={"alertdialog"} /></>';
    const [result] = await eslint.lintText(source, { filePath });
    expect(result.errorCount).toBe(0);
  });
});
