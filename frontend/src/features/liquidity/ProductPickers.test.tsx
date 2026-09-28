import { expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { ProductFormSheet } from "./ProductSheets";
import type { ProductOperationPreviewDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
const api = vi.hoisted(() => ({ Product: vi.fn(), ListOperations: vi.fn(), PreviewProductOperation: vi.fn(), RecordProductOperation: vi.fn(), UpdateProductTerms: vi.fn(), SavePolicy: vi.fn(), ResetPolicy: vi.fn(), ListReservations:vi.fn(), SaveReservation:vi.fn(), ReleaseReservation:vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({ Service: api }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({ Service: { HistoryOrigin: () => Promise.resolve({ timezone: "Asia/Singapore" }) } }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({ Service: { AccountValuations: () => Promise.resolve([{account:{id:"account"}, components:[{nativeAmount:"777",nativeCurrency:"EUR"},{nativeAmount:"5000",nativeCurrency:"USD"}]}]) } }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({ Service: { SupportedCurrencies: () => Promise.resolve(["USD","EUR","SGD"]) } }));
const money = (amount: string) => ({ amount, currency: "EUR" });
const preview: ProductOperationPreviewDTO = { effectiveAt: "2026-09-20T04:00:00Z", netWorthDelta: money("990"), reservationReleaseDetails: [], kind: "renew", normalizedJson: "{}", payloadSha256: "payload", reviewedStateHash: "reviewed", localDate: "2026-09-20", timezone: "UTC", warnings: [], assumptions: [], missingFields: [], activities: [], cashBefore: [money("50000")], cashAfter: [money("50990")], productBefore: null, productAfter: money("100000"), netWorthKnown: true, reservationReleases: [], draftProductIds: [] };

it("invalidates a reviewed product command when a calendar button changes its date", async () => {
  api.PreviewProductOperation.mockResolvedValue(preview);
  render(<QueryClientProvider client={createTestQueryClient()}><ProductFormSheet accountId="account" currency="USD" open onOpenChange={vi.fn()} /></QueryClientProvider>);
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Calendar deposit" } });
  fireEvent.change(screen.getByLabelText("Principal"), { target: { value: "1000" } });
  const pick = async (label: string, year: string, month: string, day: string) => {
    await userEvent.click(screen.getByLabelText(label));
    await userEvent.selectOptions(screen.getByRole("combobox", { name: /year/i }), year);
    await userEvent.selectOptions(screen.getByRole("combobox", { name: /month/i }), String(Number(month) - 1));
    await userEvent.click(screen.getByRole("grid").querySelector(`[data-day="${year}-${month}-${day}"] button`) as HTMLElement);
  };
  await pick("Start date", "2026", "09", "20");
  await pick("Maturity date", "2027", "09", "20");
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Confirm" })).toBeEnabled());
  await pick("Maturity date", "2027", "09", "21");
  expect(screen.getByRole("button", { name: "Confirm" })).toBeDisabled();
  expect(api.RecordProductOperation).not.toHaveBeenCalled();
});
