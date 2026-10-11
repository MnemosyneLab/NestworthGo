import { beforeEach, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { upcomingData } from "@/test/upcomingActions";
import { AvailableFundsPage } from "./AvailableFundsPage";
const mocks = vi.hoisted(() => ({ refetch: vi.fn(), manager: vi.fn() }));
vi.mock("@/queries/accounts", () => ({ useAccounts: () => ({ data: [] }) }));
vi.mock("@/queries/liquidity", () => ({ useLiquidityOverview: () => ({ data: { localDate: "2026-10-11", timezone: "UTC", baseCurrency: "USD", sources: [], buckets: [], assumptions: [], unresolvedReservations: [] }, refetch: mocks.refetch }), useUpcomingActions: () => ({ data: upcomingData(), refetch: vi.fn() }) }));
vi.mock("./ProductSheets", () => ({
  PolicySheet: ({ open }: { open: boolean }) => open ? <div>Policy opened</div> : null,
  ProductDetailSheet: ({ open, productId, onOpenChange }: { open: boolean; productId: string; onOpenChange: (open: boolean) => void }) => open ? <div role="dialog" aria-label={productId}><button onClick={() => onOpenChange(false)}>Close product</button></div> : null,
  ProductFormSheet: ({ open }: { open: boolean }) => open ? <div>Product form</div> : null,
  moneyText: () => "unknown",
}));
vi.mock("./ReservationManager", () => ({ ReservationManager: (props: { open: boolean; focusReservationId?: string }) => { mocks.manager(props); return props.open ? <div role="dialog" aria-label={`Reservations ${props.focusReservationId}`} /> : null; } }));
beforeEach(() => { mocks.refetch.mockReset().mockResolvedValue({ data: { sources: [] }, isError: false }); mocks.manager.mockClear(); });
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
