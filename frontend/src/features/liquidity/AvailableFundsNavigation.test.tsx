import { beforeEach, expect, it, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { LiquidityOverviewDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import { upcomingData } from "@/test/upcomingActions";
import { AvailableFundsPage } from "./AvailableFundsPage";
const mocks = vi.hoisted(() => ({ refetch: vi.fn(), manager: vi.fn(), overviewState: { data: undefined as LiquidityOverviewDTO | undefined, isError: false } }));
vi.mock("@/queries/accounts", () => ({ useAccounts: () => ({ data: [] }) }));
vi.mock("@/queries/liquidity", () => ({ useLiquidityOverview: () => ({ ...mocks.overviewState, refetch: mocks.refetch }), useUpcomingActions: () => ({ data: upcomingData(), refetch: vi.fn() }) }));
vi.mock("./ProductSheets", () => ({
  PolicySheet: ({ open }: { open: boolean }) => open ? <div>Policy opened</div> : null,
  ProductDetailSheet: ({ open, productId, onOpenChange }: { open: boolean; productId: string; onOpenChange: (open: boolean) => void }) => open ? <div role="dialog" aria-label={productId}><button onClick={() => onOpenChange(false)}>Close product</button></div> : null,
  ProductFormSheet: ({ open }: { open: boolean }) => open ? <div>Product form</div> : null,
  moneyText: () => "unknown",
}));
vi.mock("./ReservationManager", () => ({ ReservationManager: (props: { open: boolean; focusReservationId?: string }) => { mocks.manager(props); return props.open ? <div role="dialog" aria-label={`Reservations ${props.focusReservationId}`} /> : null; } }));
const freshOverview = { localDate: "2026-10-11", timezone: "UTC", baseCurrency: "USD", sources: [], buckets: [], assumptions: [], unresolvedReservations: [], asOf: "", includeEarlyWithdrawal: false } as unknown as LiquidityOverviewDTO;
beforeEach(() => { mocks.overviewState.data = freshOverview; mocks.overviewState.isError = false; mocks.refetch.mockReset().mockResolvedValue({ data: { sources: [] }, isError: false }); mocks.manager.mockClear(); });
it("consumes each navigation request once and reopens the same product on a new request", async () => {
  const focus = { kind: "product" as const, productId: "synthetic-product" };
  const view = render(<AvailableFundsPage focus={focus} navigationRequestId={1} />);
  await screen.findByRole("dialog", { name: "synthetic-product" });
  await userEvent.click(screen.getByRole("button", { name: "Close product" }));
  view.rerender(<AvailableFundsPage focus={focus} navigationRequestId={1} />);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument(); expect(mocks.refetch).toHaveBeenCalledOnce();
  view.rerender(<AvailableFundsPage focus={focus} navigationRequestId={2} />);
  await screen.findByRole("dialog", { name: "synthetic-product" }); expect(mocks.refetch).toHaveBeenCalledTimes(2);
  view.rerender(<AvailableFundsPage focus={focus} navigationRequestId={2} isActive={false} />);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  view.rerender(<AvailableFundsPage focus={focus} navigationRequestId={2} isActive />);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
it("opens global reservations when the source disappears and preserves single-reservation focus", async () => {
  render(<AvailableFundsPage focus={{ kind: "reservations", sourceRef: { kind: "holding", accountId: "account", holdingId: "gone" }, reservationId: "reserve" }} navigationRequestId={3} />);
  await screen.findByRole("dialog", { name: "Reservations reserve" });
  expect(mocks.manager).toHaveBeenLastCalledWith(expect.objectContaining({ source: null, focusReservationId: "reserve" }));
});
it("does not invent a policy target when latest sources no longer contain it", async () => {
  render(<AvailableFundsPage focus={{ kind: "policy", sourceRef: { kind: "holding", accountId: "account", holdingId: "gone" } }} navigationRequestId={4} />);
  await waitFor(() => expect(screen.getByRole("status")).toHaveTextContent("no longer available"));
  expect(screen.queryByText("Policy opened")).not.toBeInTheDocument();
});


it("skips an older consumed request restored after a different product navigation", async () => {
  const a = { kind: "product" as const, productId: "product-a" };
  const b = { kind: "product" as const, productId: "product-b" };
  const view = render(<AvailableFundsPage focus={a} navigationRequestId={11} />);
  await screen.findByRole("dialog", { name: "product-a" });
  await userEvent.click(screen.getByRole("button", { name: "Close product" }));
  view.rerender(<AvailableFundsPage focus={a} navigationRequestId={11} isActive={false} />);
  view.rerender(<AvailableFundsPage focus={b} navigationRequestId={12} />);
  await screen.findByRole("dialog", { name: "product-b" });
  view.rerender(<AvailableFundsPage focus={b} navigationRequestId={12} isActive={false} />);
  view.rerender(<AvailableFundsPage focus={a} navigationRequestId={11} />);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(mocks.refetch).toHaveBeenCalledTimes(2);
  view.rerender(<AvailableFundsPage focus={a} navigationRequestId={13} />);
  await screen.findByRole("dialog", { name: "product-a" });
  expect(mocks.refetch).toHaveBeenCalledTimes(3);
});


it("opens a pending product navigation after an explicit successful Retry", async () => {
  mocks.overviewState.data = undefined; mocks.overviewState.isError = true;
  mocks.refetch.mockResolvedValueOnce({ isError: true }).mockImplementationOnce(async () => {
    mocks.overviewState.data = freshOverview; mocks.overviewState.isError = false;
    return { data: freshOverview, isError: false };
  });
  const focus = { kind: "product" as const, productId: "retry-product" };
  const view = render(<AvailableFundsPage focus={focus} navigationRequestId={21} />);
  await waitFor(() => expect(mocks.refetch).toHaveBeenCalledOnce());
  await userEvent.click(screen.getByRole("button", { name: "Try again" }));
  view.rerender(<AvailableFundsPage focus={focus} navigationRequestId={21} />);
  await screen.findByRole("dialog", { name: "retry-product" });
  expect(screen.queryByText("Unable to load upcoming actions.")).not.toBeInTheDocument();
  expect(mocks.refetch).toHaveBeenCalledTimes(2);
});


it.each(["superseded", "cancelled"])("does not resume a %s pending retry", async mode => {
  mocks.overviewState.data = undefined; mocks.overviewState.isError = true;
  let finish!: (result: { data: LiquidityOverviewDTO; isError: boolean }) => void;
  mocks.refetch.mockResolvedValueOnce({ isError: true }).mockImplementationOnce(() => new Promise(resolve => { finish = resolve; })).mockImplementationOnce(async () => {
    mocks.overviewState.data = freshOverview; mocks.overviewState.isError = false;
    return { data: freshOverview, isError: false };
  });
  const a = { kind: "product" as const, productId: "pending-a" };
  const b = { kind: "product" as const, productId: "new-b" };
  const view = render(<AvailableFundsPage focus={a} navigationRequestId={31} />);
  await waitFor(() => expect(mocks.refetch).toHaveBeenCalledOnce());
  await userEvent.click(screen.getByRole("button", { name: "Try again" }));
  await waitFor(() => expect(mocks.refetch).toHaveBeenCalledTimes(2));
  if (mode === "superseded") {
    view.rerender(<AvailableFundsPage focus={b} navigationRequestId={32} />);
    await screen.findByRole("dialog", { name: "new-b" });
  } else { view.rerender(<AvailableFundsPage focus={a} navigationRequestId={31} isActive={false} />); }
  mocks.overviewState.data = freshOverview; mocks.overviewState.isError = false;
  await act(async () => finish({ data: freshOverview, isError: false }));
  expect(screen.queryByRole("dialog", { name: "pending-a" })).not.toBeInTheDocument();
  if (mode === "superseded") {
    expect(screen.getByRole("dialog", { name: "new-b" })).toBeInTheDocument();
    view.rerender(<AvailableFundsPage focus={b} navigationRequestId={32} isActive={false} />);
  }
  view.rerender(<AvailableFundsPage focus={a} navigationRequestId={31} />);
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(mocks.refetch).toHaveBeenCalledTimes(mode === "superseded" ? 3 : 2);
});

it("retries a pending reservation request and never retries itself after another error", async () => {
  mocks.overviewState.data = undefined; mocks.overviewState.isError = true;
  mocks.refetch.mockResolvedValueOnce({ isError: true }).mockResolvedValueOnce({ isError: true }).mockImplementationOnce(async () => {
    mocks.overviewState.data = freshOverview; mocks.overviewState.isError = false;
    return { data: freshOverview, isError: false };
  });
  render(<AvailableFundsPage focus={{ kind: "reservations", reservationId: "pending-reserve" }} navigationRequestId={41} />);
  await waitFor(() => expect(mocks.refetch).toHaveBeenCalledOnce());
  await userEvent.click(screen.getByRole("button", { name: "Try again" }));
  await waitFor(() => expect(mocks.refetch).toHaveBeenCalledTimes(2));
  await act(async () => { await Promise.resolve(); });
  expect(mocks.refetch).toHaveBeenCalledTimes(2);
  await userEvent.click(screen.getByRole("button", { name: "Try again" }));
  await screen.findByRole("dialog", { name: "Reservations pending-reserve" });
  expect(mocks.refetch).toHaveBeenCalledTimes(3);
});

it("resolves a pending policy target from the successful Retry result", async () => {
  mocks.overviewState.data = undefined; mocks.overviewState.isError = true;
  const sourceRef = { kind: "holding", accountId: "account", holdingId: "holding" };
  mocks.refetch.mockResolvedValueOnce({ isError: true }).mockImplementationOnce(async () => {
    mocks.overviewState.data = freshOverview; mocks.overviewState.isError = false;
    // Navigation only uses the returned source reference; the policy sheet is stubbed.
    return { data: { ...freshOverview, sources: [{ sourceRef }] }, isError: false };
  });
  render(<AvailableFundsPage focus={{ kind: "policy", sourceRef }} navigationRequestId={51} />);
  await waitFor(() => expect(mocks.refetch).toHaveBeenCalledOnce());
  await userEvent.click(screen.getByRole("button", { name: "Try again" }));
  await screen.findByText("Policy opened");
  expect(mocks.refetch).toHaveBeenCalledTimes(2);
});
