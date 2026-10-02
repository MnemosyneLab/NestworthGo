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

it("hides wordmarks from new selections without changing an existing saved wordmark", async () => {
  await i18n.changeLanguage("en");
  const changes: string[] = [];
  render(<IconPicker id="historical" kind="institution" value="bank-logo:citibank" onChange={key => changes.push(key)} />);
  const trigger = screen.getByLabelText("Choose icon");
  expect(trigger.querySelector("img")).toHaveAttribute("src", "/bank-logos/display/citibank-rect.svg");
  await userEvent.click(trigger);
  await userEvent.type(screen.getByLabelText("Search icons"), "citibank");
  expect(screen.queryAllByRole("button")).toHaveLength(0);
  expect(screen.getByRole("status")).toHaveTextContent("No matching icons");
  await userEvent.keyboard("{Escape}");
  expect(changes).toEqual([]);
  expect(trigger.querySelector("img")).toHaveAttribute("src", "/bank-logos/display/citibank-rect.svg");
  // A user can deliberately choose a generic symbol instead; no automatic rewrite.
  await userEvent.click(trigger);
  await userEvent.selectOptions(screen.getByLabelText("Icon category"), "generic");
  await userEvent.clear(screen.getByLabelText("Search icons"));
  await userEvent.click(screen.getByRole("button", {name: "Bank"}));
  expect(changes).toEqual(["bank"]);
});
