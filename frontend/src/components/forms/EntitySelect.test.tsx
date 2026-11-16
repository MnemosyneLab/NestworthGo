import { useState } from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { EntitySelect } from "./EntitySelect";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";

function Fixture({ change = vi.fn() }: { change?: (value: string) => void }) {
  const [value, setValue] = useState("bank-a");
  return <><EntitySelect id="institution" label="Institution" kind="institution" value={value}
    options={[{ id: "bank-a", name: "Alpha Bank", iconKey: "" }, { id: "bank-b", name: "中文银行", iconKey: "" }]}
    emptyLabel="None" onChange={next => { setValue(next); change(next); }} /><button>Outside</button></>;
}

it("selects and clears by keyboard while retaining focus and readable labels", async () => {
  const change = vi.fn();
  render(<Fixture change={change} />);
  const input = screen.getByRole("combobox", { name: "Institution" });
  expect(screen.getByLabelText("Institution")).toBe(input);
  expect(input).toHaveValue("Alpha Bank");
  await userEvent.tab();
  expect(input).toHaveFocus();
  await userEvent.keyboard("{ArrowDown}");
  expect(document.getElementById(input.getAttribute("aria-activedescendant")!)).toHaveTextContent("中文银行");
  await userEvent.keyboard("{Enter}");
  expect(input).toHaveValue("中文银行");
  expect(input).toHaveFocus();
  expect(change).toHaveBeenLastCalledWith("bank-b");
  await userEvent.click(input);
  await userEvent.keyboard("{ArrowUp}{ArrowUp}{Enter}");
  expect(input).toHaveValue("None");
  expect(change).toHaveBeenLastCalledWith("");
});

it("uses a portalled list and preserves selection when searches are cancelled", async () => {
  const change = vi.fn();
  const { container } = render(<Fixture change={change} />);
  const input = screen.getByRole("combobox");
  await userEvent.click(input);
  const list = screen.getByRole("listbox");
  expect(container.contains(list)).toBe(false);
  expect(within(list).getByRole("option", { name: "Alpha Bank" })).toHaveAttribute("aria-selected", "true");
  await userEvent.type(input, "中文");
  fireEvent.compositionStart(input);
  fireEvent.keyDown(input, { key: "Enter", isComposing: true });
  expect(change).not.toHaveBeenCalled();
  fireEvent.compositionEnd(input);
  await userEvent.keyboard("{Escape}");
  expect(input).toHaveValue("Alpha Bank");
  expect(input).toHaveFocus();
  await userEvent.click(input);
  await userEvent.type(input, "missing");
  await userEvent.tab();
  expect(screen.getByRole("button", { name: "Outside" })).toHaveFocus();
  expect(input).toHaveValue("Alpha Bank");
  await userEvent.click(input);
  await userEvent.click(screen.getByRole("button", { name: "Outside" }));
  expect(input).toHaveAttribute("aria-expanded", "false");
  expect(change).not.toHaveBeenCalled();
});

it("dismisses only the entity list on Escape inside a Sheet", async () => {
  const close = vi.fn();
  render(<Sheet open onOpenChange={close}><SheetContent><SheetTitle>Account</SheetTitle><Fixture /></SheetContent></Sheet>);
  const input = screen.getByRole("combobox");
  await userEvent.click(input);
  await userEvent.keyboard("{Escape}");
  expect(input).toHaveAttribute("aria-expanded", "false");
  expect(input).toHaveFocus();
  expect(screen.getByRole("dialog")).toBeInTheDocument();
  expect(close).not.toHaveBeenCalled();
});
