import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { UpcomingActionsPage } from "./UpcomingActionsPage";
import { OverviewUpcomingActionsCard } from "./OverviewUpcomingActionsCard";
import { upcomingData, upcomingItem } from "@/test/upcomingActions";
import { NavigationContext } from "@/app/NavigationContext";
import i18n from "@/i18n";
vi.mock("@/features/investment-review/InvestmentReviewPanel", () => ({ InvestmentReviewPanel: () => null, InvestmentReviewSummary: () => null }));
const query = vi.hoisted(() => ({ data: undefined as ReturnType<typeof upcomingData> | undefined, isPending: false, isError: false, isFetching: false, refetch: vi.fn() }));
vi.mock("@/queries/liquidity", () => ({ useUpcomingActions: () => query }));
beforeEach(async () => { await i18n.changeLanguage("en"); query.data = upcomingData([upcomingItem()]); query.isError = false; query.isPending = false; });
const open = vi.fn();
function mount(component: React.ReactNode) { return render(<NavigationContext.Provider value={{ open, openHealth: vi.fn() }}>{component}</NavigationContext.Provider>); }
describe("upcoming UI", () => {
  it("limits summary to three while showing the full count and safe navigation", async () => {
    query.data = upcomingData(Array.from({ length: 5 }, (_, i) => upcomingItem({ id: `item-${i}`, displayName: `Deposit ${i}` })));
    mount(<OverviewUpcomingActionsCard presentation="homepage" />);
    expect(screen.getAllByRole("button", { name: /Verify receipt/ })).toHaveLength(3);
    await userEvent.click(screen.getByRole("button", { name: "View all 5 actions" }));
    expect(open).toHaveBeenLastCalledWith({ page: "upcoming-actions" });
  });
  it("distinguishes initial error, stale data, loading and incomplete empty results", () => {
    query.data = undefined; query.isPending = true;
    const view = mount(<OverviewUpcomingActionsCard presentation="homepage" />);
    expect(screen.queryByText(/0 receipts/)).not.toBeInTheDocument();
    query.isPending = false; query.isError = true; view.rerender(<OverviewUpcomingActionsCard presentation="homepage" />);
    expect(screen.getByRole("alert")).toHaveTextContent("Unable to load");
    query.data = upcomingData([upcomingItem()]); view.rerender(<OverviewUpcomingActionsCard presentation="homepage" />);
    expect(screen.getByRole("alert")).toHaveTextContent("last successfully read");
    expect(screen.getByText("Synthetic deposit")).toBeInTheDocument();
    query.isError = false; query.data = upcomingData(); query.data.completeness = "partial"; query.data.counts.dataChecks = 1; view.rerender(<NavigationContext.Provider value={{ open, openHealth: vi.fn() }}><OverviewUpcomingActionsCard presentation="homepage" /></NavigationContext.Provider>);
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
  it("shows scheduled actions while a long attention band is still paged", async () => {
    query.data = upcomingData([
      ...Array.from({ length: 51 }, (_, i) => upcomingItem({ id: `attention-${i}`, displayName: `Attention ${i}` })),
      upcomingItem({ id: "scheduled", group: "scheduled", phase: "upcoming", displayName: "Later deposit" }),
    ]);
    mount(<UpcomingActionsPage />);
    const scheduled = screen.getByRole("region", { name: "Scheduled" });
    expect(screen.getByText("51 need attention · 1 schedule · 51 receipts to verify · 0 reservation issues")).toBeInTheDocument();
    expect(within(scheduled).getByText("Later deposit")).toBeInTheDocument();
    expect(screen.queryByText("Attention 50")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Show 50 more" }));
    expect(screen.getByText("Attention 50")).toBeInTheDocument();
    expect(within(scheduled).getByText("Later deposit")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Schedules" }));
    expect(screen.getByText("Later deposit")).toBeInTheDocument();
    expect(screen.queryByText("Attention 0")).not.toBeInTheDocument();
  });
  it("does not offer another page when both bands are fully shown", () => {
    query.data = upcomingData([
      ...Array.from({ length: 30 }, (_, i) => upcomingItem({ id: `attention-${i}`, displayName: `Attention ${i}` })),
      ...Array.from({ length: 30 }, (_, i) => upcomingItem({ id: `scheduled-${i}`, group: "scheduled", phase: "upcoming", displayName: `Schedule ${i}` })),
    ]);
    mount(<UpcomingActionsPage />);
    expect(within(screen.getByRole("region", { name: "Needs attention" })).getByText("Attention 29")).toBeInTheDocument();
    expect(within(screen.getByRole("region", { name: "Scheduled" })).getByText("Schedule 29")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Show 50 more" })).not.toBeInTheDocument();
  });
  it.each(["en", "zh-CN", "zh-TW"])("shows dates and actionable labels in %s", async language => {
    await i18n.changeLanguage(language); mount(<OverviewUpcomingActionsCard presentation="homepage" />);
    expect(screen.getByRole("button", { name: `${i18n.t("upcomingActions.verifyReceipt")} · Synthetic deposit` })).toBeInTheDocument();
    expect(screen.getByText("2026-10-11 → 2026-11-10")).toBeInTheDocument();
    expect(screen.getByText(/Asia\/Shanghai/)).toBeInTheDocument();
    expect(screen.queryByText(/upcomingActions\./)).not.toBeInTheDocument();
  });
});


it.each(["en", "zh-CN", "zh-TW"])("shows the same human household timestamp in card and full page (%s)", async language => {
  await i18n.changeLanguage(language);
  query.data = upcomingData([upcomingItem()]);
  const precise = "2026-10-11T23:30:12.123456789Z";
  query.data.asOf = precise;
  const formatted = new Intl.DateTimeFormat(language, { timeZone: query.data.timezone, dateStyle: "medium", timeStyle: "short" }).format(new Date(precise));
  const expected = i18n.t("upcomingActions.asOf", { asOf: formatted, timezone: query.data.timezone });
  const view = mount(<OverviewUpcomingActionsCard presentation="homepage" />);
  expect(screen.getByText(expected)).toBeInTheDocument();
  expect(screen.queryByText(precise)).not.toBeInTheDocument();
  view.rerender(<UpcomingActionsPage />);
  expect(screen.getByText(expected)).toBeInTheDocument();
  expect(query.data.asOf).toBe(precise);
});

it("renders unavailable timestamps and singular counts without changing their values", () => {
  query.data = upcomingData([upcomingItem()]);
  query.data.asOf = "invalid-time";
  const view = mount(<OverviewUpcomingActionsCard presentation="homepage" />);
  expect(screen.getByText("Read time unavailable")).toBeInTheDocument();
  expect(screen.getByText("1 needs attention · 0 schedules")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "View all 1 action" })).toBeInTheDocument();
  view.rerender(<UpcomingActionsPage />);
  expect(screen.getByText("Read time unavailable")).toBeInTheDocument();
  expect(screen.getByText("1 needs attention · 0 schedules · 1 receipt to verify · 0 reservation issues")).toBeInTheDocument();
  expect(query.data.counts.total).toBe(1);
});

it("keeps scheduled access visible with no attention and routes merged terms and reservations separately", async () => {
  const sourceRef = { kind: "holding", accountId: "account-1", holdingId: "holding-1" };
  query.data = upcomingData([upcomingItem({ id: "scheduled", group: "scheduled", phase: "upcoming", displayName: "Later deposit", targets: [{ kind: "reservations", sourceRef, productId: null, reservationId: null }] })]);
  const view = mount(<OverviewUpcomingActionsCard presentation="homepage" />);
  expect(screen.getByText("0 need attention · 1 schedule")).toBeInTheDocument();
  expect(screen.getByText("Later deposit")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "View all 1 action" }));
  expect(open).toHaveBeenLastCalledWith({ page: "upcoming-actions" });
  view.rerender(<NavigationContext.Provider value={{ open, openHealth: vi.fn() }}><UpcomingActionsPage /></NavigationContext.Provider>);
  expect(screen.getByText("Later deposit")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "View terms · Later deposit" }));
  expect(open).toHaveBeenLastCalledWith({ page: "available-funds", focus: { kind: "product", productId: "product-1" } });
  await userEvent.click(screen.getByRole("button", { name: "Check reservations · Later deposit" }));
  expect(open).toHaveBeenLastCalledWith({ page: "available-funds", focus: { kind: "reservations", sourceRef } });
});

it("keeps long action names and both navigation paths in wrapping containers", () => {
  query.data = upcomingData([upcomingItem({ displayName: "A long synthetic deposit with an unusually long account description", targets: [{ kind: "reservations", productId: null, sourceRef: { kind: "holding", accountId: "account-1", holdingId: "holding-1" }, reservationId: null }] })]);
  mount(<UpcomingActionsPage />);
  const row = screen.getByText(/A long synthetic deposit/).closest("li");
  expect(row).toHaveClass("flex-wrap", "min-w-0");
  expect(row?.querySelectorAll("button")).toHaveLength(2);
  expect(row?.querySelector("div.flex-wrap")).toBeInTheDocument();
});

it("keeps both links on a merged scope note", async () => {
  const sourceRef = { kind: "holding", accountId: "account-1", holdingId: "holding-1" };
  const terms = { kind: "product", productId: "product-1", sourceRef: null, reservationId: null };
  const reservations = { kind: "reservations", productId: null, sourceRef, reservationId: null };
  query.data = upcomingData();
  query.data.counts.scopeNotes = 1;
  query.data.dataChecks = [{ id: "scope", sourceKey: "holding-1", category: "scope_note", productId: "product-1", displayName: "Deposit terms and reserve", reasonCodes: ["schedule_excluded", "reservation_excluded"], relatedActionId: null, anchorOn: "2026-10-15", coverageStatus: "excluded_source", target: terms, targets: [terms, reservations], referenceOn: "2026-11-10" }];
  mount(<UpcomingActionsPage />);
  await userEvent.click(screen.getByText(/Information affecting assessment/));
  await userEvent.click(screen.getByRole("button", { name: "View terms" }));
  expect(open).toHaveBeenLastCalledWith({ page: "available-funds", focus: { kind: "product", productId: "product-1" } });
  await userEvent.click(screen.getByRole("button", { name: "Check reservations" }));
  expect(open).toHaveBeenLastCalledWith({ page: "available-funds", focus: { kind: "reservations", sourceRef } });
});

it("mounts large disclosure lists independently, unmounts on close, and reopens with live data", async () => {
  query.data = upcomingData(Array.from({ length: 250 }, (_, i) => upcomingItem({ id: `eligible-${i}`, displayName: `Eligible ${i}`, group: "eligible", phase: "eligible" })));
  const terms = { kind: "product", productId: "product-1", sourceRef: null, reservationId: null };
  query.data.counts.dataChecks = 250;
  query.data.dataChecks = Array.from({ length: 250 }, (_, i) => ({ id: `check-${i}`, sourceKey: `source-${i}`, category: "data_check", productId: "product-1", displayName: `Check ${i}`, reasonCodes: ["unsupported_source"], relatedActionId: null, anchorOn: null, coverageStatus: "", target: terms, targets: [terms], referenceOn: null }));
  const view = mount(<UpcomingActionsPage />);
  const eligible = screen.getByText("Eligible to request access (250)").closest("details")!;
  const checks = screen.getByText(/Information affecting assessment/).closest("details")!;
  expect(checks.querySelector("summary")).toHaveTextContent("250");
  expect(eligible.querySelectorAll("li")).toHaveLength(0);
  expect(checks.querySelectorAll("li")).toHaveLength(0);
  await userEvent.click(eligible.querySelector("summary")!);
  await waitFor(() => expect(eligible.querySelectorAll("li")).toHaveLength(250));
  expect(checks.querySelectorAll("li")).toHaveLength(0);
  await userEvent.click(within(eligible).getByRole("button", { name: /Eligible 249$/ }));
  expect(open).toHaveBeenLastCalledWith({ page: "available-funds", focus: { kind: "product", productId: "product-1" } });
  await userEvent.click(checks.querySelector("summary")!);
  await waitFor(() => expect(checks.querySelectorAll("li")).toHaveLength(250));
  await userEvent.click(within(checks).getAllByRole("button", { name: "View terms" })[249]);
  expect(open).toHaveBeenLastCalledWith({ page: "available-funds", focus: { kind: "product", productId: "product-1" } });
  await userEvent.click(within(checks).getByRole("button", { name: i18n.t("nav.dataHealth") }));
  expect(open).toHaveBeenLastCalledWith({ page: "data-health" });
  for (const details of [eligible, checks]) {
    for (let cycle = 0; cycle < 2; cycle++) {
      await userEvent.click(details.querySelector("summary")!);
      await waitFor(() => expect(details.querySelectorAll("li")).toHaveLength(0));
      await userEvent.click(details.querySelector("summary")!);
      await waitFor(() => expect(details.querySelectorAll("li")).toHaveLength(250));
    }
    await userEvent.click(details.querySelector("summary")!);
    await waitFor(() => expect(details.querySelectorAll("li")).toHaveLength(0));
  }
  const refresh = (prefix: string) => {
    query.data = { ...query.data!, items: query.data!.items!.map(item => ({ ...item, displayName: `${prefix} eligible` })), dataChecks: query.data!.dataChecks!.map(check => ({ ...check, displayName: `${prefix} check` })) };
    view.rerender(<NavigationContext.Provider value={{ open, openHealth: vi.fn() }}><UpcomingActionsPage /></NavigationContext.Provider>);
  };
  refresh("Fresh");
  expect(screen.queryByText("Fresh eligible")).not.toBeInTheDocument();
  expect(screen.queryByText("Fresh check")).not.toBeInTheDocument();
  for (const section of [eligible, checks]) await userEvent.click(section.querySelector("summary")!);
  await waitFor(() => expect(screen.getAllByText("Fresh eligible")).toHaveLength(250));
  expect(screen.getAllByText("Fresh check")).toHaveLength(250);
  refresh("Live");
  expect(screen.getAllByText("Live eligible")).toHaveLength(250);
  expect(screen.getAllByText("Live check")).toHaveLength(250);
}, 20000);


it.each(["en", "zh-CN", "zh-TW"])("compresses homepage empty results and preserves data-check access in %s", async language => {
  await i18n.changeLanguage(language); query.data = upcomingData();
  const view = mount(<OverviewUpcomingActionsCard presentation="homepage" />);
  expect(screen.getByText(i18n.t("homepage.noScheduledActions"))).toBeVisible();
  expect(screen.queryByRole("button", { name: i18n.t("upcomingActions.viewAll", { count: 0 }) })).not.toBeInTheDocument();
  expect(screen.queryByText(i18n.t("upcomingActions.overviewCounts", { attentionText: i18n.t("upcomingActions.attentionCount", { count: 0 }), schedules: i18n.t("upcomingActions.scheduleCount", { count: 0 }) }))).not.toBeInTheDocument();
  query.data.counts.dataChecks = 1; view.rerender(<NavigationContext.Provider value={{ open, openHealth: vi.fn() }}><OverviewUpcomingActionsCard presentation="homepage" /></NavigationContext.Provider>);
  const action = screen.getByRole("button", { name: i18n.t("homepage.reviewActionDetails") });
  action.focus(); await userEvent.keyboard("{Enter}");
  expect(open).toHaveBeenLastCalledWith({ page: "upcoming-actions" });
});
