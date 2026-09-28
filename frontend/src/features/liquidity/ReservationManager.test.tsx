import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { ReservationManager } from "./ReservationManager";
import { ReservationSheet } from "./ReservationSheet";
import type { LiquiditySourceDTO, ReservationDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

const api = vi.hoisted(() => ({ ListReservations: vi.fn(), SaveReservation: vi.fn(), ReleaseReservation: vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({ Service: api }));

function source(currency = "USD"): LiquiditySourceDTO {
  return {
    sourceRef: { kind: "account_cash", accountId: "account", currency }, sourceKey: `account_cash:account:${currency}`, accountId: "account", displayName: "Savings cash", nativeCurrency: currency,
    currentNativeValue: { amount: "1000", currency }, valueAsOf: null, priceEvidence: null, fxEvidence: null, policyOrigin: "default", policy: null,
    contractState: null, displayState: "available", reservationRequested: { amount: "0", currency }, normalRoute: null, earlyRoute: null,
    bucketResults: [], reasons: [], assumptions: [], excluded: false, dueUnconfirmed: false, productId: null,
  };
}
const reservation: ReservationDTO = {
  id: "reserve", sourceRef: source().sourceRef, label: "Rent", amount: { amount: "250", currency: "USD" }, currency: "USD", revision: 3,
  createdAt: "2026-09-20T00:00:00Z", updatedAt: "2026-09-20T00:00:00Z", releasedAt: null,
};
function show(ui: React.ReactNode, retry?: false) { return render(<QueryClientProvider client={createTestQueryClient({ retry })}>{ui}</QueryClientProvider>); }
function change(label: string, value: string) { fireEvent.change(screen.getByLabelText(label), { target: { value } }); }

beforeEach(() => {
  Object.values(api).forEach((mock) => mock.mockReset());
  api.ListReservations.mockResolvedValue([]);
  api.SaveReservation.mockResolvedValue(reservation);
  api.ReleaseReservation.mockResolvedValue({ ...reservation, releasedAt: "2026-09-28T00:00:00Z" });
});

describe("Reservation management", () => {
  it("opens the form in the same dialog, identifies currencies, and cancels back to the list", async () => {
    const user = userEvent.setup();
    show(<ReservationManager source={null} sources={[source(), source("EUR")]} accountNames={{ account: "Family bank" }} open onOpenChange={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Add reservation" }));
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(screen.getByLabelText("Reservation source")).toHaveFocus();
    expect(screen.getByRole("option", { name: "Family bank · Savings cash · USD" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Family bank · Savings cash · EUR" })).toBeInTheDocument();
    change("Purpose", "Draft");
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByRole("heading", { name: "Reservations" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Add reservation" })).toHaveFocus();
    await user.click(screen.getByRole("button", { name: "Add reservation" }));
    expect(screen.getByLabelText("Purpose")).toHaveValue("");
    expect(api.SaveReservation).not.toHaveBeenCalled();
  });

  it("shows field errors, focuses the first error and submits exact decimal strings with Enter", async () => {
    const user = userEvent.setup();
    show(<ReservationManager source={null} sources={[source(), source("EUR")]} open onOpenChange={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Add reservation" }));
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    expect(screen.getByLabelText("Reservation source")).toHaveFocus();
    expect(screen.getByLabelText("Reservation source")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText("Purpose")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText("Amount")).toHaveAttribute("aria-invalid", "true");
    expect(api.SaveReservation).not.toHaveBeenCalled();
    await user.selectOptions(screen.getByLabelText("Reservation source"), source("EUR").sourceKey);
    change("Purpose", "  Bills  ");
    for (const amount of ["0", "-2", "1e2", "1.00001", "1000000000000", "not a number"]) {
      change("Amount", amount);
      await user.click(screen.getByRole("button", { name: "Confirm" }));
      expect(screen.getByLabelText("Amount")).toHaveAttribute("aria-invalid", "true");
    }
    expect(api.SaveReservation).not.toHaveBeenCalled();
    change("Amount", " 0.0001 ");
    await user.click(screen.getByLabelText("Amount"));
    await user.keyboard("{Enter}");
    await waitFor(() => expect(api.SaveReservation).toHaveBeenCalledOnce());
    expect(api.SaveReservation).toHaveBeenCalledWith(expect.objectContaining({ sourceRef: source("EUR").sourceRef, label: "Bills", amount: "0.0001" }));
    expect(await screen.findByRole("heading", { name: "Reservations" })).toBeVisible();
    expect(screen.getByRole("status")).toHaveTextContent("Reservation saved.");
  });

  it("keeps released records collapsed and shows release feedback", async () => {
    const user = userEvent.setup();
    const archived = { ...reservation, id: "old", label: "Previous plan", releasedAt: "2026-09-25T00:00:00Z" };
    api.ListReservations.mockResolvedValue([reservation, archived]);
    api.ReleaseReservation.mockImplementation(async () => {
      api.ListReservations.mockResolvedValue([{ ...reservation, releasedAt: "2026-09-28T00:00:00Z" }, archived]);
      return { ...reservation, releasedAt: "2026-09-28T00:00:00Z" };
    });
    show(<ReservationManager source={source()} accountNames={{ account: "Family bank" }} open onOpenChange={vi.fn()} />);
    expect(await screen.findByText("Rent")).toBeVisible();
    expect(screen.getByText("Previous plan")).not.toBeVisible();
    await user.click(screen.getByText("Released reservations (1)"));
    expect(screen.getByText("Previous plan")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Release reservation" }));
    await waitFor(() => expect(api.ReleaseReservation).toHaveBeenCalledWith({ id: "reserve", expectedRevision: 3 }));
    expect(await screen.findByRole("status")).toHaveTextContent("Rent");
    await waitFor(() => expect(screen.queryByRole("button", { name: "Release reservation" })).not.toBeInTheDocument());
  });

  it("allows a failed list request to be retried", async () => {
    const user = userEvent.setup();
    api.ListReservations.mockRejectedValueOnce(new Error("unavailable")).mockResolvedValueOnce([reservation]);
    show(<ReservationManager source={source()} open onOpenChange={vi.fn()} />, false);
    await user.click(await screen.findByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Rent")).toBeVisible();
  });

  it("blocks duplicate releases, preserves the item after failure, and allows retry", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    let reject!: (cause: Error) => void;
    api.ListReservations.mockResolvedValue([reservation]);
    api.ReleaseReservation.mockReturnValueOnce(new Promise<ReservationDTO>((_resolve, fail) => { reject = fail; }));
    show(<ReservationManager source={source()} open onOpenChange={onOpenChange} />);
    await user.dblClick(await screen.findByRole("button", { name: "Release reservation" }));
    expect(api.ReleaseReservation).toHaveBeenCalledOnce();
    expect(screen.getByRole("button", { name: "Edit reservation" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(onOpenChange).not.toHaveBeenCalled();
    await act(async () => reject(new Error("unavailable")));
    expect(await screen.findByRole("alert")).toBeVisible();
    expect(screen.getByText("Rent")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Release reservation" }));
    await waitFor(() => expect(api.ReleaseReservation).toHaveBeenCalledTimes(2));
    expect(await screen.findByRole("status")).toHaveTextContent("Rent");
  });

  it("keeps edit values after a server field error and permits retry", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    api.SaveReservation.mockRejectedValueOnce(new Error(JSON.stringify({ code: "validation", field: "amount", message: "invalid" }))).mockResolvedValueOnce(reservation);
    show(<ReservationSheet source={source()} reservation={reservation} open onOpenChange={onOpenChange} />);
    change("Purpose", "Revised rent"); change("Amount", "300");
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(screen.getByLabelText("Amount")).toHaveAttribute("aria-invalid", "true"));
    expect(screen.getByLabelText("Purpose")).toHaveValue("Revised rent");
    expect(onOpenChange).not.toHaveBeenCalled();
    change("Amount", "350");
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(api.SaveReservation.mock.calls[1][0]).toMatchObject({ id: "reserve", expectedRevision: 3, amount: "350", label: "Revised rent" });
  });

  it("blocks duplicate saves and closing until the request completes", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    let resolve!: (result: ReservationDTO) => void;
    api.SaveReservation.mockReturnValue(new Promise<ReservationDTO>((done) => { resolve = done; }));
    show(<ReservationSheet source={source()} open onOpenChange={onOpenChange} />);
    change("Purpose", "Rent"); change("Amount", "250");
    await user.dblClick(screen.getByRole("button", { name: "Confirm" }));
    expect(api.SaveReservation).toHaveBeenCalledOnce();
    expect(screen.getByLabelText("Amount")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(onOpenChange).not.toHaveBeenCalled();
    await act(async () => resolve(reservation));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });
});
