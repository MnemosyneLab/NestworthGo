import { expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ChoiceMenu } from "./choice-menu";

const options = [
  { value: "readOnly", label: "Read-only", description: "Reads only" },
  { value: "full", label: "Read & Write", description: "Edits the ledger" },
];

it("opens from the keyboard, selects an option, and returns focus", async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(<ChoiceMenu label="Permissions" value="readOnly" options={options} onChange={onChange} />);
  const trigger = screen.getByRole("button", { name: "Permissions: Read-only" });
  trigger.focus();
  await user.keyboard("{Enter}");
  expect(await screen.findByRole("menuitemradio", { name: /Read-only/ })).toHaveAttribute("aria-checked", "true");
  expect(screen.getByRole("menuitemradio", { name: /Read & Write/ })).toHaveAttribute("aria-checked", "false");
  expect(screen.getByText("Edits the ledger")).toBeVisible();
  await user.keyboard("{ArrowDown}{Enter}");
  expect(onChange).toHaveBeenCalledExactlyOnceWith("full");
  await vi.waitFor(() => expect(screen.queryByRole("menu")).not.toBeInTheDocument());
  expect(trigger).toHaveFocus();
});

it("dismisses with Escape without changing the value and does not re-announce the current choice", async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(<ChoiceMenu label="Permissions" value="readOnly" options={options} onChange={onChange} />);
  await user.click(screen.getByRole("button", { name: "Permissions: Read-only" }));
  await user.click(await screen.findByRole("menuitemradio", { name: /Read-only/ }));
  expect(onChange).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Permissions: Read-only" }));
  await screen.findByRole("menu");
  await user.keyboard("{Escape}");
  await vi.waitFor(() => expect(screen.queryByRole("menu")).not.toBeInTheDocument());
  expect(onChange).not.toHaveBeenCalled();
});

it("stays closed when disabled and tints the warning tone", async () => {
  const user = userEvent.setup();
  render(<ChoiceMenu label="Permissions" value="full" options={options} onChange={vi.fn()} tone="warning" disabled />);
  const trigger = screen.getByRole("button", { name: "Permissions: Read & Write" });
  expect(trigger).toBeDisabled();
  expect(trigger.className).toContain("bg-warning/15");
  await user.click(trigger);
  expect(screen.queryByRole("menu")).not.toBeInTheDocument();
});


it("closes an open menu when disabled and does not restore it when enabled", async () => {
  const user = userEvent.setup();
  const onChange = vi.fn();
  const props = { label: "Permissions", value: "readOnly", options, onChange };
  const view = render(<ChoiceMenu {...props} />);
  await user.click(screen.getByRole("button", { name: "Permissions: Read-only" }));
  await screen.findByRole("menuitemradio", { name: /Read & Write/ });
  view.rerender(<ChoiceMenu {...props} disabled />);
  await vi.waitFor(() => expect(screen.queryByRole("menu")).not.toBeInTheDocument());
  await user.keyboard("{ArrowDown}{Enter}");
  expect(onChange).not.toHaveBeenCalled();
  view.rerender(<ChoiceMenu {...props} />);
  expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Permissions: Read-only" }));
  await user.click(await screen.findByRole("menuitemradio", { name: /Read & Write/ }));
  expect(onChange).toHaveBeenCalledExactlyOnceWith("full");
});
