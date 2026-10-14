import { Sheet, SheetContent, SheetHeader, SheetTitle } from "./sheet";
import { useState } from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { FilterableSelect } from "./filterable-select";

const options = [{ value: "", label: "All" }, { value: "a", label: "Alpha", disabled: true }, { value: "b", label: "Beta", searchKeywords: "基金" }, { value: "c", label: "Charlie" }];
function Fixture({ change = vi.fn(), disabled = false }: { change?: (value: string) => void; disabled?: boolean }) {
  const [value,setValue] = useState("b");
  return <><FilterableSelect aria-label="Choice" options={options} value={value} disabled={disabled} onValueChange={next => { setValue(next); change(next); }} /><button>Outside</button></>;
}
it("reopens every candidate after selection while focused and cancels searches without committing", async () => {
  const change=vi.fn();render(<Fixture change={change} />);
  const input=screen.getByRole("combobox");
  await userEvent.click(input);
  expect(screen.getAllByRole("option")).toHaveLength(4);expect(input).toHaveValue("");expect(change).not.toHaveBeenCalled();
  await userEvent.click(screen.getByRole("option",{name:"Charlie"}));expect(input).toHaveValue("Charlie");
  await userEvent.click(input);expect(screen.getAllByRole("option")).toHaveLength(4);
  await userEvent.type(input,"Beta");expect(screen.getAllByRole("option")).toHaveLength(1);
  await userEvent.keyboard("{Escape}{Enter}");expect(input).toHaveValue("Charlie");expect(change).toHaveBeenCalledTimes(1);
  await userEvent.click(input);await userEvent.type(input,"unmatched");expect(screen.getByText("No matches")).toBeInTheDocument();
  await userEvent.tab();expect(input).toHaveValue("Charlie");expect(change).toHaveBeenCalledTimes(1);
});
it("skips disabled options, exposes the active descendant and explicitly clears using All", async () => {
  const change=vi.fn();render(<Fixture change={change} />);const input=screen.getByRole("combobox");
  input.focus();await userEvent.keyboard("{ArrowUp}");
  expect(document.getElementById(input.getAttribute("aria-activedescendant")!)).toHaveTextContent("All");
  await userEvent.keyboard("{Enter}");expect(change).toHaveBeenLastCalledWith("");
  await userEvent.click(input);await userEvent.keyboard("{ArrowDown}{Enter}");expect(change).toHaveBeenLastCalledWith("b");
});
it("does not confirm or dismiss during IME composition and preserves controlled selection on blur", async () => {
  const change=vi.fn();render(<Fixture change={change} />);const input=screen.getByRole("combobox");
  await userEvent.click(input);fireEvent.compositionStart(input);fireEvent.change(input,{target:{value:"基金"}});
  fireEvent.keyDown(input,{key:"Enter",isComposing:true});fireEvent.keyDown(input,{key:"Escape",isComposing:true});
  expect(change).not.toHaveBeenCalled();expect(input).toHaveAttribute("aria-expanded","true");
  fireEvent.compositionEnd(input);await userEvent.keyboard("{Enter}");expect(change).toHaveBeenCalledTimes(1);
  await userEvent.click(input);await userEvent.clear(input);await userEvent.click(screen.getByText("Outside"));expect(input).toHaveValue("Beta");
});
it("has no open options while disabled", () => {render(<Fixture disabled />);expect(screen.getByRole("combobox")).toBeDisabled();expect(screen.queryByRole("listbox")).not.toBeInTheDocument();});


it("does not expose a controlled identity while asynchronous labels are unavailable", () => {
 const change=vi.fn();const value="00000000-0000-4000-8000-000000000001";
 const view=render(<FilterableSelect aria-label="Async choice" value={value} options={[]} onValueChange={change} />);
 expect(screen.getByRole("combobox")).toHaveValue("");
 view.rerender(<FilterableSelect aria-label="Async choice" value={value} options={[{value,label:"Synthetic instrument"}]} onValueChange={change} />);
 expect(screen.getByRole("combobox")).toHaveValue("Synthetic instrument");expect(change).not.toHaveBeenCalled();
});


it("consumes Escape for the open search without dismissing its owning Sheet", async () => {
 const close=vi.fn();render(<Sheet open onOpenChange={close}><SheetContent><SheetHeader><SheetTitle>Choice sheet</SheetTitle></SheetHeader><Fixture /></SheetContent></Sheet>);
 const input=screen.getByRole("combobox");await userEvent.click(input);await userEvent.keyboard("{Escape}");
 expect(input).toHaveAttribute("aria-expanded","false");expect(screen.getByRole("dialog")).toBeInTheDocument();expect(close).not.toHaveBeenCalled();
});
