import { beforeEach, expect, it } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import i18n from "@/i18n";
import { ReconciliationItemGroups } from "./ReconciliationItemGroups";
import type { ReconciliationItem } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
const items = ["pending", "unresolved", "skipped", "confirmed"].map((outcome, index) => ({ id: `item-${index}`, kind: index === 0 ? "scope" : "holding", outcome })) as ReconciliationItem[];
function Harness({ rows = items, locked = false }: { rows?: ReconciliationItem[]; locked?: boolean }) {
  return <ReconciliationItemGroups items={rows} locked={locked}>{item => <button>{item.id}</button>}</ReconciliationItemGroups>;
}
beforeEach(async () => { await i18n.changeLanguage("en"); });
it("keeps unresolved/completeness and skipped visible while confirmed starts collapsed", () => {
  render(<Harness />);
  expect(screen.getByText("Items to review (2)")).toBeInTheDocument();
  expect(screen.getByText("Skipped items (1)")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "item-0" })).toBeVisible();
  expect(screen.getByRole("button", { name: "item-1" })).toBeVisible();
  expect(screen.queryByRole("button", { name: "item-3" })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Confirmed items (1)" }));
  expect(screen.getByRole("button", { name: "item-3" })).toBeVisible();
});
it("preserves manual expansion on refetch, defers movement while a dialog is open, and resurfaces rechecks", () => {
  const { rerender } = render(<Harness />);
  fireEvent.click(screen.getByRole("button", { name: "Confirmed items (1)" }));
  rerender(<Harness rows={items.map(item => ({ ...item }))} />);
  expect(screen.getByRole("button", { name: "item-3" })).toBeVisible();
  const completed = items.map(item => ({ ...item, outcome: "confirmed" }));
  rerender(<Harness rows={completed} locked />);
  expect(screen.getByText("Items to review (2)")).toBeInTheDocument();
  rerender(<Harness rows={completed} />);
  expect(screen.queryByText(/Items to review/)).not.toBeInTheDocument();
  rerender(<Harness rows={completed.map(item => ({ ...item, recheckRequired: true }))} />);
  expect(screen.getByText("Items to review (4)")).toBeInTheDocument();
});

it("resurfaces the affected holding while leaving unrelated confirmations collapsed", () => {
 const confirmed = ["first", "second", "cash-usd", "cash-eur"].map(id => ({ id, kind: id.startsWith("cash") ? "cash" : "holding", outcome: "confirmed" }) as ReconciliationItem);
 const view = render(<ReconciliationItemGroups items={confirmed} locked={false}>{row => <span>{row.id}</span>}</ReconciliationItemGroups>);
 expect(screen.getByText("second")).not.toBeVisible();
 view.rerender(<ReconciliationItemGroups items={confirmed.map(row => row.id === "first" ? {...row,outcome:"pending",recheckRequired:true} : row)} locked={false}>{row => <span>{row.id}</span>}</ReconciliationItemGroups>);
 expect(screen.getByText("first")).toBeVisible();
 for(const id of ["second","cash-usd","cash-eur"]) expect(screen.getByText(id)).not.toBeVisible();
});

it("keeps focus on the confirmed disclosure when the last pending item moves and skipped work remains", () => {
 const rows = [{id:"current",kind:"holding",outcome:"pending"},{id:"skipped",kind:"holding",outcome:"skipped"}] as ReconciliationItem[];
 const view = render(<Harness rows={rows}/>);
 screen.getByRole("button",{name:"current"}).focus();
 view.rerender(<Harness rows={rows.map(row => row.id === "current" ? {...row,outcome:"confirmed"} : row)}/>);
 expect(screen.getByRole("button",{name:"Confirmed items (1)"})).toHaveFocus();
 expect(screen.getByRole("button",{name:"skipped"})).toBeVisible();
});
