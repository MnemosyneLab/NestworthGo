import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { ReservationManager } from "./ReservationManager";
import { ReservationSheet, PolicySheet, ProductDetailSheet, ProductFormSheet } from "./ProductSheets";
import type { LiquiditySourceDTO, ProductDTO, ProductOperationPreviewDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

// Product command contracts are tested here; real picker interactions have separate coverage.
vi.mock("@/components/ui/date-picker", () => ({ DatePicker: (props: { id?: string; value: string; min?: string; max?: string; disabled?: boolean; onChange: (value: string) => void; "aria-invalid"?: boolean; "aria-describedby"?: string }) => <input id={props.id} value={props.value} min={props.min} max={props.max} disabled={props.disabled} aria-invalid={props["aria-invalid"]} aria-describedby={props["aria-describedby"]} type="date" onChange={(event) => props.onChange(event.target.value)} /> }));
vi.mock("@/components/ui/time-picker", () => ({ TimePicker: (props: { id?: string; value: string; disabled?: boolean; onChange: (value: string) => void }) => <input id={props.id} value={props.value} disabled={props.disabled} type="time" onChange={(event) => props.onChange(event.target.value)} /> }));
const api = vi.hoisted(() => ({ Product: vi.fn(), ListOperations: vi.fn(), PreviewProductOperation: vi.fn(), RecordProductOperation: vi.fn(), UpdateProductTerms: vi.fn(), AppendProductValuation: vi.fn(), SavePolicy: vi.fn(), ResetPolicy: vi.fn(), ListReservations:vi.fn(), SaveReservation:vi.fn(), ReleaseReservation:vi.fn() }));
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
  it("confirms receipt independently without offering renewal", async () => {
    const user = userEvent.setup();
    show(<ProductDetailSheet productId="product" open onOpenChange={vi.fn()} />);
    await user.click(await screen.findByRole("button", { name: "Confirm receipt" }));
    expect(screen.queryByRole("button", { name: "Confirm receipt and renew" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("New principal")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Returned principal")).toHaveValue("100000");
    change("Interest received", "1000");
    await user.click(screen.getByRole("button", { name: "Preview" }));
    await waitFor(() => expect(api.PreviewProductOperation).toHaveBeenCalledOnce());
    const sent = api.PreviewProductOperation.mock.calls[0][0];
    expect(sent).toMatchObject({ kind: "settle", settle: { productId: "product", returnedPrincipal: "100000", interest: "1000", fee: "0" } });
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
    await user.click(screen.getByText("Advanced settings · timing, fees and limits"));
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
    await user.selectOptions(screen.getByLabelText("Product type"), "locked_product");
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

it("identifies required product fields and cash-exclusion confirmation before preview", async () => {
  const user = userEvent.setup();
  show(<ProductFormSheet accountId="account" currency="EUR" open onOpenChange={vi.fn()} />);
  await user.click(screen.getByRole("button", { name: "Preview" }));
  expect(screen.getByRole("alert")).toHaveTextContent("Principal: complete this field.");
  expect(screen.getByLabelText("Principal")).toHaveAttribute("aria-invalid", "true");
  change("Principal", "1000"); change("Name", "Existing");
  await user.click(screen.getByRole("button", { name: "Preview" }));
  expect(screen.getByRole("alert")).toHaveTextContent("Start date: complete this field.");
  expect(screen.getByLabelText("Start date")).toHaveAttribute("aria-invalid", "true");
  change("Start date", "2026-09-20"); change("Maturity date", "2027-09-20");
  await user.selectOptions(screen.getByRole("combobox", { name: "Entry method" }), "record_existing"); change("Cost", "1000");
  await user.click(screen.getByRole("button", { name: "Preview" }));
  expect(screen.getByRole("alert")).toHaveTextContent("Confirm that account cash excludes this product");
  expect(api.PreviewProductOperation).not.toHaveBeenCalled();
  await user.click(screen.getByRole("checkbox", { name: "The cash balance shown in this account excludes this product." }));
  await user.click(screen.getByRole("button", { name: "Preview" }));
  await waitFor(() => expect(api.PreviewProductOperation).toHaveBeenCalledWith(expect.objectContaining({ kind: "record_existing" })));
});

it("lets global reservation management choose a source and save the first reservation", async () => {
  const user = userEvent.setup();
  const asset = source(false);
  show(<ReservationManager sources={[asset]} source={null} open onOpenChange={vi.fn()} />);
  await user.click(await screen.findByRole("button", { name: "Add reservation" }));
  expect(screen.getAllByRole("dialog")).toHaveLength(1);
  await user.selectOptions(screen.getByLabelText("Reservation source"), asset.sourceKey);
  change("Purpose", "Bills"); change("Amount", "100");
  await user.click(screen.getByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(api.SaveReservation).toHaveBeenCalledWith(expect.objectContaining({ sourceRef: asset.sourceRef, label: "Bills", amount: "100" })));
});

describe("Pending command protection", () => {
  it("submits a new reservation only once while saving", async () => {
    api.SaveReservation.mockReturnValue(new Promise(() => {}));
    show(<ReservationSheet source={source(false)} open onOpenChange={vi.fn()} />);
    change("Purpose", "Rent"); change("Amount", "100");
    await userEvent.dblClick(screen.getByRole("button", { name: "Confirm" }));
    expect(api.SaveReservation).toHaveBeenCalledTimes(1);
    expect(api.SaveReservation.mock.calls.every(([request]) => request.id === undefined)).toBe(true);
  });
  it("rejects a preview for the previous command after editing", async () => {
    let resolvePreview!: (value: ProductOperationPreviewDTO) => void;
    api.PreviewProductOperation.mockReturnValue(new Promise(resolve => { resolvePreview = resolve; }));
    show(<ProductFormSheet accountId="account" currency="USD" open onOpenChange={vi.fn()} />);
    change("Name", "Deposit"); change("Principal", "1000"); change("Start date", "2026-09-20"); change("Maturity date", "2027-09-20");
    await userEvent.click(screen.getByRole("button", { name: "Preview" }));
    change("Principal", "2000");
    await act(async () => resolvePreview(preview));
    expect(screen.getByRole("button", { name: "Confirm" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Confirm" }));
    expect(api.RecordProductOperation).not.toHaveBeenCalled();
    expect(api.PreviewProductOperation.mock.calls[0][0].open.principal).toBe("1000");
  });
});

it("reports reservation release failures and allows retry after completion", async () => {
  let rejectRelease!: (reason: Error) => void;
  const onOpenChange = vi.fn();
  api.ReleaseReservation.mockReturnValueOnce(new Promise((_, reject) => { rejectRelease = reject; }));
  api.Product.mockResolvedValue({product: product(), reservations: [{id: "reserve", revision: 1, label: "Rent", amount: money("100"), releasedAt: null}], permittedActions: [], disabledReasons: {}});
  show(<ProductDetailSheet productId="product" open onOpenChange={onOpenChange} />);
  const release = await screen.findByRole("button", {name: "Release reservation"});
  await userEvent.dblClick(release);
  expect(api.ReleaseReservation).toHaveBeenCalledOnce();
  expect(release).toBeDisabled();
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  await userEvent.keyboard("{Escape}");
  expect(onOpenChange).not.toHaveBeenCalled();
  await act(async () => rejectRelease(new Error("Release failed")));
  expect(await screen.findByRole("alert")).toHaveTextContent("An unexpected error occurred.");
  await waitFor(() => expect(release).toBeEnabled());
  await userEvent.click(release);
  await waitFor(() => expect(api.ReleaseReservation).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(release).toBeEnabled());
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  expect(onOpenChange).toHaveBeenCalledWith(false);
});

it("formats product operation timestamps in the History timezone", async () => {
  api.ListOperations.mockResolvedValue({operations: [{id: "open", kind: "open", effectiveAt: "2026-09-20T20:00:00Z"}], next: null});
  show(<ProductDetailSheet productId="product" open onOpenChange={vi.fn()} />);
  expect(await screen.findByText(/Sep 21, 2026, 4:00 AM/)).toBeVisible();
  expect(screen.queryByText(/2026-09-20T20:00:00Z/)).not.toBeInTheDocument();
});

it("clears monetary input when changing the operation type", async () => {
  show(<ProductDetailSheet productId="product" open onOpenChange={vi.fn()} />);
  await userEvent.click(await screen.findByRole("button", {name: "Confirm receipt"}));
  change("Returned principal", "100000");
  await userEvent.click(screen.getByRole("button", {name: "Record interest received"}));
  expect(screen.getByLabelText("Interest received")).toHaveValue("");
});

it("saves a preset for an assumed policy as a first explicit rule", async () => {
  const asset = source(false);
  asset.policyOrigin = "assumed";
  asset.policy!.revision = 1;
  show(<PolicySheet source={asset} open onOpenChange={vi.fn()} />);
  await userEvent.click(screen.getByRole("radio", {name: /Available soon/}));
  await userEvent.click(screen.getByRole("button", {name: "Save policy"}));
  await waitFor(() => expect(api.SavePolicy).toHaveBeenCalledWith(expect.objectContaining({expectedRevision:0, policy:expect.objectContaining({accessKind:"on_request", settlementDays:1, dayBasis:"weekdays", normalExitFee:"0"})})));
});

 it("opens a deposit using just principal and dates with simple defaults", async () => {
  show(<ProductFormSheet accountId="account" currency="EUR" open onOpenChange={vi.fn()} />);
  change("Principal", "500"); change("Start date", "2026-09-20"); change("Maturity date", "2027-09-20");
  expect(screen.getByLabelText("Settlement days")).not.toBeVisible();
  expect(screen.getByLabelText("Opening fee")).not.toBeVisible();
  expect(screen.getByLabelText("Allow early withdrawal")).not.toBeChecked();
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  await waitFor(() => expect(api.PreviewProductOperation).toHaveBeenCalledOnce());
  expect(api.PreviewProductOperation.mock.calls[0][0].open).toMatchObject({
    principal: "500", currency: "EUR", openingFee: "0",
    terms: { name: "Term deposit", startOn: "2026-09-20", maturityOn: "2027-09-20", interestMode: "none" },
    policy: { accessKind: "on_date", unlockOn: "2027-09-20", settlementDays: 0, normalExitFee: "0", earlyKind: "not_allowed" },
  });
});

it("preserves advanced deposit rules while changing the maturity date", async () => {
  const existing = product("simple_act_360");
  existing.policy.settlementDays = 2;
  existing.policy.normalExitFee = money("12");
  api.Product.mockResolvedValue({ product: existing, reservations: [], permittedActions: [], disabledReasons: {} });
  show(<PolicySheet source={source(true)} open onOpenChange={vi.fn()} />);
  await screen.findByLabelText("Maturity date");
  expect(screen.getByLabelText("Annual rate (%)")).toHaveValue("2.5");
  expect(screen.getByLabelText("Settlement days")).not.toBeVisible();
  change("Maturity date", "2027-10-20");
  await userEvent.click(screen.getByRole("button", { name: "Save policy" }));
  await waitFor(() => expect(api.UpdateProductTerms).toHaveBeenCalledOnce());
  expect(api.UpdateProductTerms.mock.calls[0][0]).toMatchObject({
    terms: { interestMode: "simple_act_360", annualRatePercent: "2.5", maturityOn: "2027-10-20" },
    policy: { settlementDays: 2, normalExitFee: "12", unlockOn: "2027-10-20" },
  });
});

it("requires visible value and cost when recording an existing locked product", async () => {
  const user = userEvent.setup();
  show(<ProductFormSheet accountId="account" currency="EUR" open onOpenChange={vi.fn()} />);
  await user.selectOptions(screen.getByLabelText("Product type"), "locked_product");
  await user.selectOptions(screen.getByRole("combobox", { name: "Entry method" }), "record_existing");
  change("Name", "Existing fund"); change("Principal", "1000"); change("Start date", "2026-01-01");
  expect(screen.getByLabelText("Current value")).toBeVisible();
  expect(screen.getByLabelText("Cost")).toBeVisible();
  await user.click(screen.getByRole("checkbox", { name: "The cash balance shown in this account excludes this product." }));
  await user.click(screen.getByRole("button", { name: "Preview" }));
  expect(screen.getByRole("alert")).toHaveTextContent("Current value: complete this field.");
  expect(screen.getByLabelText("Current value")).toHaveFocus();
  expect(api.PreviewProductOperation).not.toHaveBeenCalled();
  change("Current value", "950");
  await user.click(screen.getByRole("button", { name: "Preview" }));
  expect(screen.getByRole("alert")).toHaveTextContent("Cost: complete this field.");
  expect(screen.getByLabelText("Cost")).toHaveFocus();
  change("Cost", "1010");
  await user.click(screen.getByRole("button", { name: "Preview" }));
  await waitFor(() => expect(api.PreviewProductOperation).toHaveBeenCalledWith(expect.objectContaining({ recordExisting: expect.objectContaining({ principal: "1000", currentValue: "950", totalCostBasis: "1010" }) })));
});

it("preserves an existing deposit's early withdrawal rules when toggled off and on", async () => {
  const existing = product();
  existing.policy = { ...existing.policy, earlyKind: "allowed", earlyAmountMode: "fixed_gross", earlyGrossAmount: money("97000"), earlyFee: money("15"), earlySettlementDays: 3, earlyDayBasis: "weekdays" };
  api.Product.mockResolvedValue({ product: existing, reservations: [], permittedActions: [], disabledReasons: {} });
  show(<PolicySheet source={source(true)} open onOpenChange={vi.fn()} />);
  const toggle = await screen.findByRole("checkbox", { name: "Allow early withdrawal" });
  await userEvent.click(toggle);
  await userEvent.click(toggle);
  expect(screen.getByLabelText("Early gross amount")).toHaveValue("97000");
  await userEvent.click(screen.getByRole("button", { name: "Save policy" }));
  await waitFor(() => expect(api.UpdateProductTerms).toHaveBeenCalledWith(expect.objectContaining({ policy: expect.objectContaining({ earlyKind: "allowed", earlyGrossAmount: "97000", earlyFee: "15", earlySettlementDays: 3, earlyDayBasis: "weekdays" }) })));
});

it("shows the exact undo target and marks reversed operations in history", async () => {
  api.ListOperations.mockResolvedValue({ operations: [
    { id: "undo-income", kind: "undo", effectiveAt: "2026-09-21T04:00:00Z", reversesOperationId: "income" },
    { id: "income", kind: "receive_interest", effectiveAt: "2026-09-20T04:00:00Z", reversesOperationId: null },
    { id: "opening", kind: "open", effectiveAt: "2026-01-01T04:00:00Z", reversesOperationId: null },
  ], next: null });
  show(<ProductDetailSheet productId="product" open onOpenChange={vi.fn()} />);
  await userEvent.click(await screen.findByRole("button", { name: "Grouped undo" }));
  expect(screen.getByText("Reversed")).toBeVisible();
  expect(screen.getByText(/Undo: Opened · Jan 1, 2026, 12:00 PM/)).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
  expect(screen.queryByRole("button", { name: "Preview" })).not.toBeInTheDocument();
  expect(api.RecordProductOperation).not.toHaveBeenCalled();
});

it("offers valuation correction instead of an undo that the backend cannot perform", async () => {
  api.Product.mockResolvedValue({ product: { ...product("none"), kind: "locked_product" }, reservations: [], permittedActions: [], disabledReasons: {} });
  api.ListOperations.mockResolvedValue({ operations: [
    { id: "valuation", kind: "value_observation", effectiveAt: "2026-09-20T04:00:00Z", reversesOperationId: null },
    { id: "opening", kind: "open", effectiveAt: "2026-01-01T04:00:00Z", reversesOperationId: null },
  ], next: null });
  show(<ProductDetailSheet productId="product" open onOpenChange={vi.fn()} />);
  expect(await screen.findByText("The latest entry is a valuation update. Correct it by updating the valuation.")).toBeVisible();
  expect(screen.queryByRole("button", { name: "Grouped undo" })).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Update current valuation" }));
  expect(screen.getByRole("button", { name: "Confirm" })).toBeDisabled();
  expect(screen.getByText("Record the current value of the whole product. Account cash is unchanged.")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
  expect(api.AppendProductValuation).not.toHaveBeenCalled();
});

it("shows contract dates and rate without requiring the edit form", async () => {
  show(<ProductDetailSheet productId="product" open onOpenChange={vi.fn()} />);
  expect(await screen.findByText("Maturity date")).toBeVisible();
  expect(screen.getByText("2026-09-20")).toBeVisible();
  expect(screen.getByText("Annual rate (%)")).toBeVisible();
  expect(screen.getByText("2.5")).toBeVisible();
  expect(screen.getByText("Original terms")).toBeVisible();
  expect(screen.queryByRole("button", { name: "Grouped undo" })).not.toBeInTheDocument();
  expect(screen.queryByText("Reservations")).not.toBeInTheDocument();
});

it("holds the submitted values steady while a product operation is saving", async () => {
  let finish!: (value: { operationId: string }) => void;
  api.RecordProductOperation.mockReturnValue(new Promise((resolve) => { finish = resolve; }));
  const onOpenChange = vi.fn();
  show(<ProductDetailSheet productId="product" open onOpenChange={onOpenChange} />);
  await userEvent.click(await screen.findByRole("button", { name: "Confirm receipt" }));
  change("Interest received", "1000");
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Confirm" })).toBeEnabled());
  await userEvent.click(screen.getByRole("button", { name: "Confirm" }));
  expect(screen.getByLabelText("Returned principal")).toBeDisabled();
  expect(screen.getByLabelText("Interest received")).toBeDisabled();
  expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  await userEvent.keyboard("{Escape}");
  expect(onOpenChange).not.toHaveBeenCalled();
  await act(async () => finish({ operationId: "receipt" }));
  await waitFor(() => expect(screen.queryByLabelText("Returned principal")).not.toBeInTheDocument());
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  expect(onOpenChange).toHaveBeenCalledWith(false);
});

it("keeps a new product form open during recording and allows retry after failure", async () => {
  let rejectRecord!: (reason: Error) => void;
  api.RecordProductOperation.mockReturnValueOnce(new Promise((_, reject) => { rejectRecord = reject; }));
  const onOpenChange = vi.fn();
  show(<ProductFormSheet accountId="account" currency="EUR" open onOpenChange={onOpenChange} />);
  change("Principal", "500"); change("Start date", "2026-09-20"); change("Maturity date", "2027-09-20");
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Confirm" })).toBeEnabled());
  await userEvent.click(screen.getByRole("button", { name: "Confirm" }));
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  await userEvent.keyboard("{Escape}");
  expect(onOpenChange).not.toHaveBeenCalled();
  expect(api.RecordProductOperation).toHaveBeenCalledOnce();
  await act(async () => rejectRecord(new Error("Recording failed")));
  expect(await screen.findByRole("alert")).toBeVisible();
  expect(screen.getByLabelText("Principal")).toHaveValue("500");
  await waitFor(() => expect(screen.getByRole("button", { name: "Confirm" })).toBeEnabled());
  await userEvent.click(screen.getByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(api.RecordProductOperation).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
});

it("keeps product edits open during saving and releases the close guard after failure", async () => {
  let rejectSave!: (reason: Error) => void;
  api.UpdateProductTerms.mockReturnValueOnce(new Promise((_, reject) => { rejectSave = reject; }));
  const onOpenChange = vi.fn();
  show(<PolicySheet source={source(true)} open onOpenChange={onOpenChange} />);
  await userEvent.click(await screen.findByRole("button", { name: "Save policy" }));
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  await userEvent.keyboard("{Escape}");
  expect(onOpenChange).not.toHaveBeenCalled();
  expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
  await act(async () => rejectSave(new Error("Saving failed")));
  expect(await screen.findByRole("alert")).toBeVisible();
  await waitFor(() => expect(screen.getByRole("button", { name: "Save policy" })).toBeEnabled());
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  expect(onOpenChange).toHaveBeenCalledWith(false);
});

it("keeps valuation updates open until the result arrives and permits retry", async () => {
  let rejectValuation!: (reason: Error) => void;
  api.AppendProductValuation.mockReturnValueOnce(new Promise((_, reject) => { rejectValuation = reject; }));
  api.AppendProductValuation.mockResolvedValue({ product: { ...product("none"), kind: "locked_product" } });
  api.Product.mockResolvedValue({ product: { ...product("none"), kind: "locked_product" }, reservations: [], permittedActions: [], disabledReasons: {} });
  const onOpenChange = vi.fn();
  show(<ProductDetailSheet productId="product" open onOpenChange={onOpenChange} />);
  await userEvent.click(await screen.findByRole("button", { name: "Update current valuation" }));
  change("Current value", "99000");
  await userEvent.click(screen.getByRole("button", { name: "Confirm" }));
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  await userEvent.keyboard("{Escape}");
  expect(onOpenChange).not.toHaveBeenCalled();
  await act(async () => rejectValuation(new Error("Valuation failed")));
  expect(await screen.findByRole("alert")).toBeVisible();
  expect(screen.getByLabelText("Current value")).toHaveValue("99000");
  await waitFor(() => expect(screen.getByRole("button", { name: "Confirm" })).toBeEnabled());
  await userEvent.click(screen.getByRole("button", { name: "Confirm" }));
  await waitFor(() => expect(api.AppendProductValuation).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(screen.queryByLabelText("Current value")).not.toBeInTheDocument());
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  expect(onOpenChange).toHaveBeenCalledWith(false);
});

it("bounds new interest periods by the previous receipt, maturity, and the History timezone today", async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-09-19T20:00:00Z"));
  try {
    const existing = product();
    existing.interestPaidThroughOn = "2026-08-01";
    existing.maturityOn = "2026-09-30";
    api.Product.mockResolvedValue({ product: existing, reservations: [], permittedActions: ["receive_interest"], disabledReasons: {} });
    show(<ProductDetailSheet productId="product" open onOpenChange={vi.fn()} />);
    await userEvent.click(await screen.findByRole("button", { name: "Record interest received" }));
    expect(screen.getByLabelText("Interest paid through")).toHaveAttribute("min", "2026-08-01");
    expect(screen.getByLabelText("Interest paid through")).toHaveAttribute("max", "2026-09-20");
    expect(screen.getByLabelText("Local date")).toHaveAttribute("max", "2026-09-20");
  } finally {
    vi.useRealTimers();
  }
});


it("shows and focuses the required paid-through date when adding annual interest to a deposit", async () => {
  api.Product.mockResolvedValue({ product: product("none"), reservations: [], permittedActions: [], disabledReasons: {} });
  api.UpdateProductTerms.mockRejectedValueOnce(new Error(JSON.stringify({ code: "validation", field: "interestPaidThroughOn", message: "is required" })));
  const closed = vi.fn();
  const user = userEvent.setup();
  show(<PolicySheet source={source(true)} open onOpenChange={closed} />);
  await user.selectOptions(await screen.findByLabelText("Interest (optional)"), "rate");
  change("Annual rate (%)", "3.5");
  const paid = screen.getByLabelText("Interest paid through");
  expect(paid.closest("details")).toBeNull();
  expect(paid).toHaveValue("");
  await user.click(screen.getByRole("button", { name: "Save policy" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Interest paid through");
  expect(paid).toHaveAttribute("aria-invalid", "true");
  expect(paid).toHaveFocus();
  expect(closed).not.toHaveBeenCalled();
  expect(api.UpdateProductTerms.mock.calls[0][0].terms.interestPaidThroughOn).toBeNull();
  change("Interest paid through", "2026-02-01");
  await user.click(screen.getByRole("button", { name: "Save policy" }));
  await waitFor(() => expect(api.UpdateProductTerms).toHaveBeenCalledTimes(2));
  expect(api.UpdateProductTerms.mock.calls[1][0].terms).toMatchObject({ annualRatePercent: "3.5", interestPaidThroughOn: "2026-02-01" });
  await waitFor(() => expect(closed).toHaveBeenCalledWith(false));
});

it("retains an explicitly entered paid-through date across interest mode switches", async () => {
  const user = userEvent.setup();
  show(<PolicySheet source={source(true)} open onOpenChange={vi.fn()} />);
  const interest = await screen.findByLabelText("Interest (optional)");
  change("Interest paid through", "2026-02-01");
  await user.selectOptions(interest, "none");
  expect(screen.queryByLabelText("Interest paid through")).not.toBeInTheDocument();
  await user.selectOptions(interest, "rate");
  expect(screen.getByLabelText("Interest paid through")).toHaveValue("2026-02-01");
  change("Annual rate (%)", "3.5");
  await user.selectOptions(screen.getByLabelText("Interest year basis (days)"), "simple_act_360");
  await user.click(screen.getByRole("button", { name: "Save policy" }));
  await waitFor(() => expect(api.UpdateProductTerms).toHaveBeenCalledOnce());
  expect(api.UpdateProductTerms.mock.calls[0][0].terms).toMatchObject({ interestMode: "simple_act_360", interestPaidThroughOn: "2026-02-01" });
});
