import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import i18n from "@/i18n";
import { RangeToggle } from "./RangeToggle";

describe("RangeToggle", () => {
  it.each([
    ["en", "YTD"],
    ["zh-CN", "今年以来"],
    ["zh-TW", "今年以來"],
  ])("localizes the year-to-date label for %s", async (language, label) => {
    await i18n.changeLanguage(language);

    render(<RangeToggle ranges={["ytd"]} value="ytd" onChange={() => undefined} label="Range" />);

    expect(screen.getByRole("button", { name: label })).toBeInTheDocument();
  });
});
