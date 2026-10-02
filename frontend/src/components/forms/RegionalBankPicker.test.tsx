import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { IconPicker } from "./IconPicker";
import i18n from "@/i18n";

it.each([
  ["邮储", "中国邮政储蓄银行"], ["郵儲", "中国邮政储蓄银行"],
  ["StanChart", "渣打银行（Standard Chartered）"], ["渣打", "渣打银行（Standard Chartered）"],
  ["中銀香港", "中银香港（BOCHK）"], ["BOCHK", "中银香港（BOCHK）"],
])("finds the correct regional bank for %s", async (query, label) => {
  await i18n.changeLanguage("en");
  render(<IconPicker id="regional" kind="institution" value="bank-logo:boc" onChange={() => undefined} />);
  await userEvent.click(screen.getByLabelText("Choose icon"));
  await userEvent.type(screen.getByLabelText("Search icons"), query);
  const results = screen.getAllByRole("button");
  expect(results).toHaveLength(1);
  expect(results[0]).toHaveTextContent(label);
});
