import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { ReservationManager } from "./ReservationManager";
import { PolicySheet, ProductDetailSheet, ProductFormSheet } from "./ProductSheets";
import type { LiquiditySourceDTO, ProductDTO, ProductOperationPreviewDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

const api = vi.hoisted(() => ({ Product: vi.fn(), ListOperations: vi.fn(), PreviewProductOperation: vi.fn(), RecordProductOperation: vi.fn(), UpdateProductTerms: vi.fn(), SavePolicy: vi.fn(), ResetPolicy: vi.fn(), ListReservations:vi.fn(), SaveReservation:vi.fn(), ReleaseReservation:vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({ Service: api }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({ Service: { HistoryOrigin: () => Promise.resolve({ timezone: "Asia/Singapore" }) } }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({ Service: { AccountValuations: () => Promise.resolve([{account:{id:"account"}, components:[{nativeAmount:"777",nativeCurrency:"EUR"},{nativeAmount:"5000",nativeCurrency:"USD"}]}]) } }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({ Service: { SupportedCurrencies: () => Promise.resolve(["USD","EUR","SGD"]) } }));
const sourceRef = { kind: "holding", accountId: "account", holdingId: "holding", currency: null };
const money = (amount: string) => ({ amount, currency: "EUR" });
function product(mode = "simple_act_365"): ProductDTO {
  return {
    id: "product", accountId: "account", holdingId: "holding", instrumentId: "instrument", kind: "term_deposit", name: "Deposit",
    note: "Original terms", currency: "EUR", principal: money("100000"), currentValue: money("100000"), currentCostBasis: money("100000"),
    startOn: "2026-01-01", maturityOn: "2026-09-20", interestMode: mode, annualRate: mode.startsWith("simple_") ? "0.025" : null,
    maturityInterest: mode === "manual_maturity_amount" ? money("2500") : null, interestPaidThroughOn: mode.startsWith("simple_") ? "2026-01-01" : null,
    state: "open", displayState: "due_unconfirmed", revision: 3, predecessorId: null, successorId: null, nextAction: "confirm_receipt",
    policy: {
      id: "policy", sourceRef, accessKind: "on_date", unlockOn: "2026-09-20", settlementDays: 0, dayBasis: "calendar", receiptOnOverride: null,
      accessibleAmountCap: null, normalExitFee: money("0"), earlyKind: "not_allowed", earlySettlementDays: null, earlyDayBasis: null, earlyFee: null,
      earlyAmountMode: null, earlyGrossAmount: null, confirmedAt: null, note: "Policy note", revision: 4,
    },
  };
}
function source(managed: boolean): LiquiditySourceDTO {
  const p = product();
  return { sourceRef, sourceKey: "holding:holding", accountId: "account", displayName: "Deposit", nativeCurrency: "EUR", currentNativeValue: money("100000"),
    valueAsOf: null, priceEvidence: null, fxEvidence: null, policyOrigin: managed ? "contract" : "explicit", policy: p.policy,
    contractState: managed ? "open" : null, displayState: "locked", reservationRequested: money("0"), normalRoute: null, earlyRoute: null,
    bucketResults: [], reasons: [], assumptions: [], excluded: false, dueUnconfirmed: false, productId: managed ? "product" : null };
}
function show(ui: React.ReactNode) { return render(<QueryClientProvider client={createTestQueryClient()}>{ui}</QueryClientProvider>); }
function change(label: string, value: string) { fireEvent.change(screen.getByLabelText(label), { target: { value } }); }
const preview: ProductOperationPreviewDTO = { effectiveAt: "2026-09-20T04:00:00Z", netWorthDelta: money("990"), reservationReleaseDetails: [], kind: "renew", normalizedJson: "{}", payloadSha256: "payload", reviewedStateHash: "reviewed", localDate: "2026-09-20", timezone: "UTC", warnings: [], assumptions: [], missingFields: [], activities: [], cashBefore: [money("50000")], cashAfter: [money("50990")], productBefore: null, productAfter: money("100000"), netWorthKnown: true, reservationReleases: [], draftProductIds: [] };

beforeEach(() => {
  Object.values(api).forEach((mock) => mock.mockReset());
  api.Product.mockResolvedValue({ product: product(), reservations: [], permittedActions: ["settle", "renew", "receive_interest"], disabledReasons: {} });
  api.ListOperations.mockResolvedValue({ operations: [], next: null });
  api.PreviewProductOperation.mockResolvedValue(preview);
  api.RecordProductOperation.mockResolvedValue({ operationId: "recorded" });
  api.UpdateProductTerms.mockResolvedValue({ product: product() });
  api.SavePolicy.mockResolvedValue(product().policy);
  api.ListReservations.mockResolvedValue([]);
  api.SaveReservation.mockResolvedValue({});
  api.ReleaseReservation.mockResolvedValue({});
});

describe("product P1 regression flows", () => {
  it.each(["simple_act_365", "simple_act_360", "manual_maturity_amount"])("previews and records renewal with full %s terms", async (mode) => {
    api.Product.mockResolvedValue({ product: product(mode), reservations: [], permittedActions: ["renew"], disabledReasons: {} });
    const user = userEvent.setup();
    show(<ProductDetailSheet productId="product" open onOpenChange={vi.fn()} />);
    await user.click(await screen.findByRole("button", { name: "Confirm receipt and renew" }));
    change("Returned principal", "100000"); change("Interest received", "1000"); change("New principal", "100000"); change("Opening fee", "10");
    expect(screen.getByLabelText("Start date")).toHaveValue("");
    change("Start date", "2026-09-20"); change("Maturity date", "2027-09-20");
    if (mode.startsWith("simple_")) expect(screen.getByLabelText("Annual rate (%)")).toHaveValue("2.5");
    else expect(screen.getByLabelText("Unpaid maturity interest")).toHaveValue("2500");
    await user.click(screen.getByRole("button", { name: "Preview" }));
    await waitFor(() => expect(api.PreviewProductOperation).toHaveBeenCalledOnce());
    const sent = api.PreviewProductOperation.mock.calls[0][0];
    expect(sent.renew).toMatchObject({ principal: "100000", openingFee: "10", terms: { startOn: "2026-09-20", maturityOn: "2027-09-20", interestMode: mode, annualRate: null, annualRatePercent: mode.startsWith("simple_") ? "2.5" : null, maturityInterest: mode === "manual_maturity_amount" ? "2500" : null, interestPaidThroughOn: null }, policy: { accessKind: "on_date", unlockOn: "2027-09-20" } });
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await waitFor(() => expect(api.RecordProductOperation).toHaveBeenCalledOnce());
    expect(api.RecordProductOperation.mock.calls[0][0]).toMatchObject({ command: sent, reviewedStateHash: "reviewed" });
  });

  it("saves managed rules through atomic contract update and preserves interest", async () => {
    const closed = vi.fn(); const user = userEvent.setup();
    show(<PolicySheet source={source(true)} open onOpenChange={closed} />);
    await screen.findByLabelText("Revised expected receipt date");
    change("Revised expected receipt date", "2026-09-25"); change("Normal exit fee", "12");
    await user.click(screen.getByRole("button", { name: "Save policy" }));
    await waitFor(() => expect(api.UpdateProductTerms).toHaveBeenCalledOnce());
    expect(api.SavePolicy).not.toHaveBeenCalled();
    expect(api.UpdateProductTerms.mock.calls[0][0]).toMatchObject({ productId: "product", expectedRevision: 3, terms: { annualRatePercent: "2.5", interestPaidThroughOn: "2026-01-01", kind: "term_deposit" }, policy: { receiptOnOverride: "2026-09-25", normalExitFee: "12", note: "Policy note" } });
    await waitFor(() => expect(closed).toHaveBeenCalledWith(false));
  });

  it("allows ordinary early access inputs and preserves unknown normal timing and fee", async () => {
    const user = userEvent.setup();
    show(<PolicySheet source={source(false)} open onOpenChange={vi.fn()} />);
    change("Settlement days", ""); change("Normal exit fee", "");
    await user.selectOptions(screen.getByLabelText("Early withdrawal"), "allowed");
    change("Early settlement days", "2");
    await user.selectOptions(screen.getByLabelText("Early day basis"), "weekdays");
    change("Early fee", "5");
    await user.selectOptions(screen.getByLabelText("Early proceeds basis"), "fixed_gross");
    change("Early gross amount", "900");
    await user.click(screen.getByRole("button", { name: "Save policy" }));
    await waitFor(() => expect(api.SavePolicy).toHaveBeenCalledOnce());
    expect(api.SavePolicy.mock.calls[0][0].policy).toMatchObject({ settlementDays: null, normalExitFee: null, earlyKind: "allowed", earlySettlementDays: 2, earlyDayBasis: "weekdays", earlyFee: "5", earlyAmountMode: "fixed_gross", earlyGrossAmount: "900" });
  });

  it("opens locked products with separate lock, maturity, settlement, and fee rules", async () => {
    const user = userEvent.setup();
    show(<ProductFormSheet accountId="account" currency="EUR" open onOpenChange={vi.fn()} />);
    await user.selectOptions(screen.getByLabelText("Asset"), "locked_product");
    change("Name", "Locked fund"); change("Principal", "1000"); change("Start date", "2026-09-20"); change("Maturity date", "2027-09-20");
    await user.selectOptions(screen.getByLabelText("Access"), "on_date");
    change("Unlock date", "2027-03-20"); change("Settlement days", "3"); change("Normal exit fee", "10");
    await user.selectOptions(screen.getByLabelText("Early withdrawal"), "allowed");
    change("Early settlement days", "2"); change("Early fee", "20");
    await user.click(screen.getByRole("button", { name: "Preview" }));
    await waitFor(() => expect(api.PreviewProductOperation).toHaveBeenCalledOnce());
    expect(api.PreviewProductOperation.mock.calls[0][0].open).toMatchObject({ terms: { kind: "locked_product", maturityOn: "2027-09-20" }, policy: { unlockOn: "2027-03-20", settlementDays: 3, normalExitFee: "10", earlyKind: "allowed", earlySettlementDays: 2, earlyFee: "20", earlyAmountMode: "current_value" } });
    await waitFor(() => expect(screen.getByRole("button", { name: "Confirm" })).toBeEnabled());
    change("Normal exit fee", "15");
    expect(screen.getByRole("button", { name: "Confirm" })).toBeDisabled();
  });

  it("keeps the edit form and shows failure when an update is rejected", async () => {
    api.UpdateProductTerms.mockRejectedValue(new Error("revision conflict"));
    const closed = vi.fn();const user = userEvent.setup();
    show(<PolicySheet source={source(true)} open onOpenChange={closed} />);
    await user.click(await screen.findByRole("button", { name: "Save policy" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("An unexpected error occurred.");
    expect(closed).not.toHaveBeenCalled();
  });
});

describe("product P2 regression flows", () => {
  it("selects contract currency and shows only its cash balance", async () => {
    const user=userEvent.setup();
    show(<ProductFormSheet accountId="account" currency="USD" cashAmount="5000" open onOpenChange={vi.fn()} />);
    await screen.findByRole("option",{name:"EUR"});
    await user.selectOptions(screen.getByLabelText("Contract currency"),"EUR");
    expect(screen.getByText(/777/)).toBeInTheDocument();
    change("Name","Euro deposit");change("Principal","500");change("Start date","2026-09-20");change("Maturity date","2027-09-20");
    await waitFor(() => expect(screen.getByLabelText("Local date")).toBeEnabled());
    change("Local date","2026-09-20");change("Local time","12:30");
    await user.click(screen.getByRole("button",{name:"Preview"}));
    await waitFor(() => expect(api.PreviewProductOperation).toHaveBeenCalledOnce());
    expect(api.PreviewProductOperation.mock.calls[0][0].open).toMatchObject({currency:"EUR",effectiveAt:"",effectiveLocalDate:"2026-09-20",effectiveLocalTime:"12:30"});
    expect(await screen.findByText(/Net worth change from this operation/)).toBeInTheDocument();
  });
  it("preserves an ordinary cap and supports explicit zero", async () => {
    const user=userEvent.setup();const item=source(false);item.policy!.accessibleAmountCap=money("5000");
    show(<PolicySheet source={item} open onOpenChange={vi.fn()} />);
    expect(screen.getByLabelText("Accessible amount cap")).toHaveValue("5000");
    change("Normal exit fee","12");await user.click(screen.getByRole("button",{name:"Save policy"}));
    await waitFor(() => expect(api.SavePolicy).toHaveBeenCalledOnce());
    expect(api.SavePolicy.mock.calls[0][0].policy.accessibleAmountCap).toBe("5000");
    change("Accessible amount cap","0");await user.click(screen.getByRole("button",{name:"Save policy"}));
    await waitFor(() => expect(api.SavePolicy).toHaveBeenCalledTimes(2));
    expect(api.SavePolicy.mock.calls[1][0].policy.accessibleAmountCap).toBe("0");
  });
  it("edits and releases ordinary reservations using ID and revision, and shows released rows", async () => {
    const user=userEvent.setup();
    const reservation={id:"r1",sourceRef,label:"Rent",amount:money("1000"),currency:"EUR",revision:3,createdAt:"",updatedAt:"",releasedAt:null};
    api.ListReservations.mockResolvedValue([reservation,{...reservation,id:"r2",label:"Old reserve",releasedAt:"2026-09-20T00:00:00Z"}]);
    show(<ReservationManager source={source(false)} open onOpenChange={vi.fn()} />);
    expect(await screen.findByText("Released")).toBeInTheDocument();
    await user.click(screen.getByRole("button",{name:"Edit reservation"}));change("Amount","1200");
    await user.click(screen.getByRole("button",{name:"Confirm"}));
    await waitFor(() => expect(api.SaveReservation).toHaveBeenCalledOnce());
    expect(api.SaveReservation.mock.calls[0][0]).toMatchObject({id:"r1",expectedRevision:3,label:"Rent",amount:"1200"});
    await user.click(await screen.findByRole("button",{name:"Release reservation"}));
    await waitFor(() => expect(api.ReleaseReservation).toHaveBeenCalledWith({id:"r1",expectedRevision:3}));
  });
  it("permits only name and note fields on a closed product", async () => {
    const user=userEvent.setup();api.Product.mockResolvedValue({product:{...product("none"),state:"settled"},reservations:[],permittedActions:[],disabledReasons:{}});
    show(<PolicySheet source={source(true)} open onOpenChange={vi.fn()} />);
    await screen.findByLabelText("Name");
    expect(screen.queryByLabelText("Maturity date")).not.toBeInTheDocument();expect(screen.queryByLabelText("Normal exit fee")).not.toBeInTheDocument();
    change("Name","Renamed");change("Contract note","Notes");await user.click(screen.getByRole("button",{name:"Save policy"}));
    await waitFor(() => expect(api.UpdateProductTerms).toHaveBeenCalledOnce());
    expect(api.UpdateProductTerms.mock.calls[0][0].terms).toMatchObject({name:"Renamed",note:"Notes",kind:"term_deposit",interestMode:"none"});
  });
});

it("previews the previous active operation after the latest one was reversed, including older pages", async () => {
  api.ListOperations.mockResolvedValueOnce({ operations: [{id:"undo-income",kind:"undo",reversesOperationId:"income"},{id:"income",kind:"receive_interest",reversesOperationId:null}], next:"older" })
    .mockResolvedValueOnce({ operations:[{id:"opening",kind:"open",reversesOperationId:null}], next:null });
  show(<ProductDetailSheet productId="product" open onOpenChange={vi.fn()} />);
  await waitFor(() => expect(api.ListOperations).toHaveBeenCalledTimes(2));
  await userEvent.click(await screen.findByRole("button", { name: "Grouped undo" }));
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  await waitFor(() => expect(api.PreviewProductOperation).toHaveBeenCalled());
  expect(api.PreviewProductOperation.mock.calls[0][0]).toMatchObject({ kind:"undo", undo:{operationId:"opening"} });
});
