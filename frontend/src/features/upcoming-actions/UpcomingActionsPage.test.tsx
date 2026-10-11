import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { UpcomingActionsPage } from "./UpcomingActionsPage";
import { OverviewUpcomingActionsCard } from "./OverviewUpcomingActionsCard";
import { upcomingData, upcomingItem } from "@/test/upcomingActions";
import { NavigationContext } from "@/app/NavigationContext";
import i18n from "@/i18n";
const query = vi.hoisted(() => ({ data: undefined as ReturnType<typeof upcomingData> | undefined, isPending: false, isError: false, isFetching: false, refetch: vi.fn() }));
vi.mock("@/queries/liquidity", () => ({ useUpcomingActions: () => query }));
beforeEach(async () => { await i18n.changeLanguage("en"); query.data = upcomingData([upcomingItem()]); query.isError = false; query.isPending = false; });
const open = vi.fn();
function mount(component: React.ReactNode) { return render(<NavigationContext.Provider value={{ open, openHealth: vi.fn() }}>{component}</NavigationContext.Provider>); }
describe("upcoming UI", () => {
  it("limits summary to three while showing the full count and safe navigation", async () => {
    query.data = upcomingData(Array.from({ length: 5 }, (_, i) => upcomingItem({ id: `item-${i}`, displayName: `Deposit ${i}` })));
    mount(<OverviewUpcomingActionsCard />);
    expect(screen.getAllByRole("button", { name: /Verify receipt/ })).toHaveLength(3);
    await userEvent.click(screen.getByRole("button", { name: "View all 5 actions" }));
    expect(open).toHaveBeenLastCalledWith({ page: "upcoming-actions" });
  });
  it("distinguishes initial error, stale data, loading and incomplete empty results", () => {
    query.data = undefined; query.isPending = true;
    const view = mount(<OverviewUpcomingActionsCard />);
    expect(screen.queryByText(/0 receipts/)).not.toBeInTheDocument();
    query.isPending = false; query.isError = true; view.rerender(<OverviewUpcomingActionsCard />);
    expect(screen.getByRole("alert")).toHaveTextContent("Unable to load");
    query.data = upcomingData([upcomingItem()]); view.rerender(<OverviewUpcomingActionsCard />);
    expect(screen.getByRole("alert")).toHaveTextContent("last successfully read");
    expect(screen.getByText("Synthetic deposit")).toBeInTheDocument();
    query.isError = false; query.data = upcomingData(); query.data.completeness = "partial"; query.data.counts.dataChecks = 1; view.rerender(<OverviewUpcomingActionsCard />);
    expect(screen.getByText(/information needed to assess/)).toBeInTheDocument();
  });
  it("pages in fifty-item increments and keeps eligible items collapsed", async () => {
    query.data = upcomingData([...Array.from({ length: 51 }, (_, i) => upcomingItem({ id: `item-${i}`, displayName: `Deposit ${i}` })), upcomingItem({ id: "eligible", displayName: "Old unlock", group: "eligible", phase: "eligible" })]);
    mount(<UpcomingActionsPage />);
    expect(screen.queryByText("Deposit 50")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Show 50 more" }));
    expect(screen.getByText("Deposit 50")).toBeInTheDocument();
    expect(screen.getByText("Eligible to request access (1)").closest("details")).not.toHaveAttribute("open");
    await userEvent.click(screen.getByRole("button", { name: "Reservation issues" }));
    expect(screen.queryByText("Deposit 0")).not.toBeInTheDocument();
  });
  it.each(["en", "zh-CN", "zh-TW"])("shows dates and actionable labels in %s", async language => {
    await i18n.changeLanguage(language); mount(<OverviewUpcomingActionsCard />);
    expect(screen.getByRole("button", { name: `${i18n.t("upcomingActions.verifyReceipt")} · Synthetic deposit` })).toBeInTheDocument();
    expect(screen.getByText(/2026-10-11.*Asia\/Shanghai/)).toBeInTheDocument();
    expect(screen.queryByText(/upcomingActions\./)).not.toBeInTheDocument();
  });
});
