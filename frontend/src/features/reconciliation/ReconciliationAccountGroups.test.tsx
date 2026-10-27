import { useRef } from "react";
import { beforeEach, expect, it } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/i18n";
import { accountIsVerified, ReconciliationAccountGroups } from "./ReconciliationAccountGroups";
import type { ReconciliationAccount } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";

function account(id: string, verified = false): ReconciliationAccount {
  return { accountId: id, name: `Synthetic ${id}`, currency: "USD", outcome: verified ? "confirmed" : "pending", counts: { confirmed: verified ? 2 : 0, pending: verified ? 0 : 2, skipped: 0, unresolved: 0 }, items: ["scope", "balance"].map(kind => ({ id: kind, kind, outcome: verified ? "confirmed" : "pending" })) } as ReconciliationAccount;
}
function Harness({ accounts, locked = false, session = "one" }: { accounts: ReconciliationAccount[]; locked?: boolean; session?: string }) {
  const trigger = useRef<HTMLElement | null>(null);
  return <ReconciliationAccountGroups key={session} accounts={accounts} locked={locked} trigger={trigger} registerHeader={() => {}} notices={() => null}>{row => <button onClick={event => { trigger.current = event.currentTarget; }}>Review {row.name}</button>}</ReconciliationAccountGroups>;
}
beforeEach(async () => { await i18n.changeLanguage("en"); });
it("groups unfinished first, keeps exact progress semantics, and supports keyboard toggles", async () => {
  const done = account("done", true), pending = account("pending");
  render(<Harness accounts={[done, pending]} />);
  const buttons = screen.getAllByRole("button");
  expect(buttons.findIndex(button => button.textContent?.includes(pending.name))).toBeLessThan(buttons.findIndex(button => button.textContent?.includes(done.name)));
  const toggle = screen.getByRole("button", { name: "Collapse Synthetic pending · USD" });
  expect(toggle).toHaveAttribute("aria-expanded", "true");
  expect(screen.queryByRole("button", { name: "Review Synthetic done" })).not.toBeInTheDocument();
  const user = userEvent.setup({ delay: null });
  toggle.focus(); await user.keyboard("{Enter}");
  expect(toggle).toHaveAttribute("aria-expanded", "false");
  await user.keyboard(" ");
  expect(toggle).toHaveAttribute("aria-expanded", "true");
  for (const bad of [ { ...done, items: [] }, { ...done, items: done.items!.filter(item => item.kind !== "scope") }, { ...done, recheckRequired: true }, { ...done, inventoryCoverageGap: true }, { ...done, counts: { ...done.counts, skipped: 1 } }, { ...done, items: done.items!.map(item => ({ ...item, outcome: "unresolved" })) } ]) expect(accountIsVerified(bad)).toBe(false);
});
it("defers completion until dialog closure, focuses the next unfinished account, and preserves manual choices on refetch", () => {
  const one = account("one"), two = account("two");
  const { rerender } = render(<Harness accounts={[one, two]} />);
  const review = screen.getByRole("button", { name: "Review Synthetic one" });
  review.focus(); fireEvent.click(review);
  rerender(<Harness accounts={[account("one", true), two]} locked />);
  expect(screen.getByRole("button", { name: "Collapse Synthetic one · USD" })).toHaveAttribute("aria-expanded", "true");
  rerender(<Harness accounts={[account("one", true), two]} />);
  expect(screen.getByRole("button", { name: "Expand Synthetic one · USD" })).toHaveAttribute("aria-expanded", "false");
  expect(screen.getByRole("button", { name: "Collapse Synthetic two · USD" })).toHaveFocus();
  fireEvent.click(screen.getByRole("button", { name: "Expand Synthetic one · USD" }));
  rerender(<Harness accounts={[account("one", true), { ...two }]} />);
  expect(screen.getByRole("button", { name: "Collapse Synthetic one · USD" })).toHaveAttribute("aria-expanded", "true");
  rerender(<Harness accounts={[account("one"), two]} />);
  expect(screen.getByRole("button", { name: "Collapse Synthetic one · USD" })).toHaveAttribute("aria-expanded", "true");
});
it("resets per-session expansion and supports all-done/all-unfinished groups", () => {
  const { rerender } = render(<Harness accounts={[account("one")]} />);
  fireEvent.click(screen.getByRole("button", { name: "Collapse Synthetic one · USD" }));
  rerender(<Harness accounts={[account("one")]} session="two" />);
  expect(screen.getByRole("button", { name: "Collapse Synthetic one · USD" })).toHaveAttribute("aria-expanded", "true");
  rerender(<Harness accounts={[account("one", true)]} session="three" />);
  expect(screen.getByText("All accounts have been verified.")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Expand Synthetic one · USD" })).toHaveAttribute("aria-expanded", "false");
});
