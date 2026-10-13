import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { InvestmentReviewItem } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import type { InvestmentReviewReport } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import { createTestQueryClient } from "@/test/queryClient";
import i18n from "@/i18n";
import { investmentReviewKey } from "@/queries/investmentReview";
import { InvestmentReviewPanel } from "./InvestmentReviewPanel";

const calls = vi.hoisted(() => ({ read: vi.fn(), enroll: vi.fn(), snooze: vi.fn(), acknowledge: vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({ Service: {
  InvestmentReviews: calls.read, SetInvestmentReviewEnrollment: calls.enroll,
  SetInvestmentReviewSnooze: calls.snooze, AcknowledgeInvestmentReview: calls.acknowledge,
} }));
function report(changes: Partial<InvestmentReviewItem> = {}, revision = 1): InvestmentReviewReport {
  return {
    schemaVersion: "investment-review/1", asOf: "2026-10-13T12:00:00Z", localDate: "2026-10-13", timezone: "Asia/Shanghai", revision,
    policy: { enabled: true, windowDays: 30, lowAnnualizedPercent: "1", lossPercent: "-1", sustainDays: 7, maxAgeDays: 7 },
    items: [{ instrumentId: "synthetic", displayName: "Synthetic fund", status: "low_yield", monitored: true, episodeId: "episode-one", acknowledged: false, snoozed: false, snoozedUntilOn: "", baselineOn: "2026-09-09", latestOn: "2026-10-09", targetOn: "2026-09-09", elapsedDays: 30, ageDays: 4, missingDays: 0, observationCount: 31, annualizedPercent: "0.25", periodPercent: "0.0205479452", reason: "", ...changes }],
  };
}
function deferred() {
  let resolve!: (value: InvestmentReviewReport) => void;
  const promise = new Promise<InvestmentReviewReport>(finish => { resolve = finish; });
  return { promise, resolve };
}
function mount(initial: InvestmentReviewReport) {
  const mutation = deferred(); const read = deferred();
  calls.read.mockResolvedValueOnce(initial).mockReturnValue(read.promise);
  for (const action of [calls.enroll, calls.snooze, calls.acknowledge]) action.mockReturnValue(mutation.promise);
  const client = createTestQueryClient({ retry: false });
  const tree = (active = true) => <QueryClientProvider client={client}>
    <button>Elsewhere</button><div hidden={!active}><InvestmentReviewPanel isActive={active} /></div>
  </QueryClientProvider>;
  const view = render(tree());
  const finish = async (next: InvestmentReviewReport) => {
    await act(async () => { mutation.resolve(next); });
    await waitFor(() => expect(calls.read).toHaveBeenCalledTimes(2));
    await act(async () => { read.resolve(next); });
  };
  return { ...view, tree, finish, mutation, read, client };
}
async function showAll() {
  await userEvent.click(await screen.findByRole("button", { name: i18n.t("investmentReview.all", { count: 1 }) }));
}
async function keyboardAction(action: string) {
  if (action !== "enroll") await userEvent.click(screen.getByText(i18n.t("investmentReview.controls", { name: "Synthetic fund" })));
  const control = action === "enroll" ? screen.getByRole("checkbox", { name: i18n.t("investmentReview.enroll", { name: "Synthetic fund" }) })
    : screen.getByRole("button", { name: i18n.t(`investmentReview.${action}`, { name: "Synthetic fund", days: 7 }) });
  control.focus(); await userEvent.keyboard(action === "enroll" ? " " : "{Enter}");
}
const transitions = [
  { action: "enroll", before: { monitored: false, episodeId: "" }, after: {}, group: "pending" },
  { action: "resumeNow", before: { snoozed: true, episodeId: "", snoozedUntilOn: "2026-10-20" }, after: {}, group: "pending" },
  { action: "snooze", before: {}, after: { snoozed: true, episodeId: "", snoozedUntilOn: "2026-10-20" }, group: "other" },
  { action: "ignoreIndefinitely", before: {}, after: { monitored: false, episodeId: "" }, group: "other" },
  { action: "acknowledge", before: {}, after: { acknowledged: true }, group: "other" },
] as const;

beforeEach(async () => { Object.values(calls).forEach(call => call.mockReset()); await i18n.changeLanguage("en"); });
describe("authoritative investment review focus transitions", () => {
  it.each(transitions)("keeps logical row focus after $action and the asynchronous refetch", async ({ action, before, after, group }) => {
    const view = mount(report(before)); await showAll();
    if (action === "acknowledge") {
      screen.getByRole("button", { name: "Reviewed · Synthetic fund" }).focus(); await userEvent.keyboard("{Enter}");
    } else await keyboardAction(action);
    // Disabled controls hand off to a stable, named row target during the write.
    await waitFor(() => expect(screen.getByRole("heading", { name: "Synthetic fund" })).toHaveFocus());
    await view.finish(report(after, 2));
    await waitFor(() => expect(screen.getByRole("heading", { name: "Synthetic fund" }).closest("li")).toHaveAttribute("data-review-group", group));
    const heading = screen.getByRole("heading", { name: "Synthetic fund" });
    expect(heading).toHaveFocus();
    expect(view.container.querySelector('[aria-live="polite"]')).toHaveTextContent(i18n.t("investmentReview.moved", { name: "Synthetic fund", group: i18n.t(group === "pending" ? "investmentReview.pending" : "investmentReview.other", { count: 1 }) }));
    expect(screen.getByRole("checkbox", { name: "Enable review reminders · Synthetic fund" })).toBeEnabled();
    expect(screen.getByRole("checkbox", { name: "Enable review reminders · Synthetic fund" }).closest("li")).toBe(heading.closest("li"));
    await userEvent.tab(); expect(document.activeElement).not.toBe(document.body);
  });
  it.each(["snooze", "ignoreIndefinitely", "acknowledge"])("returns to the pending filter when %s removes the focused row from the default view", async action => {
    const view = mount(report()); await screen.findByRole("button", { name: "Pending reviews · 1" });
    if (action === "acknowledge") { screen.getByRole("button", { name: "Reviewed · Synthetic fund" }).focus(); await userEvent.keyboard("{Enter}"); }
    else await keyboardAction(action);
    await view.finish(report(action === "snooze" ? { snoozed: true, episodeId: "" } : action === "ignoreIndefinitely" ? { monitored: false, episodeId: "" } : { acknowledged: true }, 2));
    await waitFor(() => expect(screen.getByRole("button", { name: "Pending reviews · 0" })).toHaveFocus());
    expect(view.container.querySelector('[aria-live="polite"]')).toHaveTextContent("Synthetic fund is no longer in pending reviews.");
  });
  it("preserves an equivalent focused checkbox when a foreground report moves the row without a pending write", async () => {
    const view = mount(report({ monitored: false, episodeId: "" })); await showAll();
    const checkbox = screen.getByRole("checkbox", { name: "Enable review reminders · Synthetic fund" }); checkbox.focus();
    await act(async () => { view.client.setQueryData(investmentReviewKey, report({}, 2)); });
    await waitFor(() => expect(screen.getByRole("checkbox", { name: "Enable review reminders · Synthetic fund" })).toBeChecked());
    expect(screen.getByRole("checkbox", { name: "Enable review reminders · Synthetic fund" })).toHaveFocus();
  });
  it.each(["elsewhere", "inactive", "unmount"])("does not steal focus after %s while the returned report is pending", async destination => {
    const view = mount(report({ monitored: false, episodeId: "" })); await showAll(); await keyboardAction("enroll");
    await act(async () => { view.mutation.resolve(report({}, 2)); });
    await waitFor(() => expect(calls.read).toHaveBeenCalledTimes(2));
    const elsewhere = screen.getByRole("button", { name: "Elsewhere" }); elsewhere.focus();
    if (destination === "inactive") view.rerender(view.tree(false));
    if (destination === "unmount") {
      view.unmount(); document.body.append(elsewhere); elsewhere.focus();
    }
    await view.finish(report({}, 2));
    expect(elsewhere).toHaveFocus();
    if (destination === "unmount") elsewhere.remove();
  });
});
