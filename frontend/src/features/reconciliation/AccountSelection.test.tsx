import { useState } from "react";
import { expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AccountSelection } from "./AccountSelection";
import type { AccountRecordDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

vi.mock("@/queries/directory", () => ({ useMembers: () => ({ data: [{ id: "alice", name: "Alice" }, { id: "bob", name: "Bob" }] }) }));
const accounts = [
  { account: { id: "a", name: "Shared", defaultCurrency: "USD", accountType: "brokerage", institutionId: "bank-a" }, ownership: [{ memberId: "alice" }, { memberId: "bob" }], institutionName: "Bank A" },
  { account: { id: "b", name: "Savings", defaultCurrency: "CNY", accountType: "bank_account", institutionId: "bank-b" }, ownership: [{ memberId: "bob" }], institutionName: "Bank B" },
  { account: { id: "c", name: "Wallet", defaultCurrency: "USD", accountType: "cash_on_hand" }, ownership: [{ memberId: "alice" }] },
] as AccountRecordDTO[];
function Harness() {
  const [selected, setSelected] = useState(["a", "b", "c"]);
  return <AccountSelection accounts={accounts} selected={selected} onChange={setSelected} />;
}
it("combines filters, preserves hidden selections, supports empty/partial selection and clearing", async () => {
  const user = userEvent.setup();
  render(<Harness />);
  await user.selectOptions(screen.getByRole("combobox", { name: "Owner" }), "alice");
  expect(screen.getAllByRole("checkbox")).toHaveLength(2);
  expect(screen.getByRole("status")).toHaveTextContent("1 selected outside this filter");
  await user.click(screen.getByRole("button", { name: "Deselect filtered accounts" }));
  expect(screen.getByRole("status")).toHaveTextContent("Selected 1 of 3");
  await user.click(screen.getByRole("button", { name: "Select filtered accounts" }));
  expect(screen.getByRole("status")).toHaveTextContent("Selected 3 of 3");
  expect(screen.getByRole("button", { name: "Select filtered accounts" })).toBeDisabled();
  await user.click(screen.getAllByRole("checkbox")[0]);
  expect(screen.getByRole("status")).toHaveTextContent("Filtered: 1 of 2 selected");
  await user.selectOptions(screen.getByRole("combobox", { name: "Institution" }), "bank-b");
  expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
  expect(screen.getByText("No accounts match these filters.")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Select filtered accounts" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "Clear account filters" }));
  expect(screen.getByRole("checkbox", { name: /Savings/ })).toBeChecked();
  expect(screen.getByRole("checkbox", { name: /Shared/ })).not.toBeChecked();
  await user.selectOptions(screen.getByRole("combobox", { name: "Account type" }), "cash_on_hand");
  await user.selectOptions(screen.getByRole("combobox", { name: "Institution" }), "none");
  expect(screen.getAllByRole("checkbox")).toHaveLength(1);
  await user.click(screen.getByRole("button", { name: "Deselect all accounts" }));
  expect(screen.getByRole("status")).toHaveTextContent("Selected 0 of 3");
  await user.click(screen.getByRole("button", { name: "Clear account filters" }));
  expect(screen.getAllByRole("checkbox").every(node => !(node as HTMLInputElement).checked)).toBe(true);
  await user.tab();
  expect(document.activeElement).not.toBe(document.body);
});
