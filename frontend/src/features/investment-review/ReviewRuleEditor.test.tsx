import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/i18n";
import type { ReviewRule } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import { ReviewRuleEditor } from "./ReviewRuleEditor";

const state = vi.hoisted(() => ({ save: vi.fn(async (_value: unknown) => ({})), origin: { timezone: "Asia/Singapore" }, bootstrap: { household: { baseCurrency: "USD" } }, catalog: { currencies: ["USD", "CNY", "EUR"] } as { currencies: string[] } | undefined, catalogError: false, catalogRefetch: vi.fn() }));
vi.mock("@/queries/investments", () => ({ useInstruments: () => ({ data: [{ id: "synthetic-fund", name: "Synthetic fund" }] }) }));
vi.mock("@/queries/settings", () => ({ useSettings: () => ({ data: { timezone: "system" } }) }));
vi.mock("@/queries/catalog", () => ({ useCatalog: () => ({ data: state.catalog, isError: state.catalogError, refetch: state.catalogRefetch }) }));
vi.mock("@/queries/history", () => ({ useHistoryOrigin: () => ({ data: state.origin }) }));
vi.mock("@/queries/household", () => ({ useBootstrap: () => ({ data: state.bootstrap }) }));
vi.mock("@/queries/investmentReviewV2", () => ({ useReviewV2Commands: () => ({
  createRule: { mutateAsync: state.save, isPending: false }, updateRule: { mutateAsync: state.save, isPending: false },
}) }));

const predicate = { any: [
  { all: [
    { leaf: { metric: "type", operator: "in", types: ["mutual_fund", "etf"] } },
    { leaf: { metric: "price_change", operator: "<", value: "-5", lookbackDays: 30 } },
  ] },
  { leaf: { metric: "cost", operator: ">", value: "1000", currency: "CNY", fxMode: "native" } },
] };
const rule = {
  id: "synthetic-rule", name: "Synthetic grouped rule", enabled: true, severity: "warning", rowRevision: 2,
  spec: { target: { kind: "all_current" }, predicate, schedule: { timezone: "UTC" }, triggerPolicy: { kind: "single" } },
} as unknown as ReviewRule;

beforeEach(async () => { vi.clearAllMocks(); state.origin = { timezone: "Asia/Singapore" }; state.bootstrap = { household: { baseCurrency: "USD" } }; state.catalog = { currencies: ["USD", "CNY", "EUR"] }; state.catalogError = false; await i18n.changeLanguage("en"); });

describe("investment review rule editor", () => {
  it("visually edits a nested predicate without flattening its groups or sibling leaves", async () => {
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} rule={rule} />);
    expect(screen.getByRole("group", { name: "Nested condition group" })).toBeVisible();
    const values = screen.getAllByRole("textbox", { name: "Value" });
    await userEvent.clear(values[0]);
    await userEvent.type(values[0], "-6");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(state.save).toHaveBeenCalledOnce());
    const saved = state.save.mock.calls[0][0] as { spec: { predicate: typeof predicate } };
    expect((state.save.mock.calls[0][0] as { spec: { schedule: { timezone: string } } }).spec.schedule.timezone).toBe("UTC");
    expect(saved.spec.predicate).toEqual({ any: [
      { all: [
        { leaf: { metric: "type", operator: "in", types: ["mutual_fund", "etf"] } },
        { leaf: { metric: "price_change", operator: "<", value: "-6", lookbackDays: 30 } },
      ] },
      { leaf: { metric: "cost", operator: ">", value: "1000", currency: "CNY", fxMode: "native" } },
    ] });
  });
  it("adds a nested all/any group through the visual builder", async () => {
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} rule={rule} />);
    await userEvent.click(screen.getByRole("button", { name: "Add condition group" }));
    expect(screen.getAllByRole("group", { name: "Nested condition group" })).toHaveLength(2);
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(state.save).toHaveBeenCalledOnce());
    const saved = state.save.mock.calls[0][0] as { spec: { predicate: { any: unknown[] } } };
    expect(saved.spec.predicate.any).toHaveLength(3);
    expect(saved.spec.predicate.any[2]).toEqual({ all: [{ leaf: expect.objectContaining({ metric: "amount", value: "1000" }) }] });
  });
  it("allows an existing rule timezone to be edited and submits the changed value", async () => {
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} rule={rule} />);
    const timezone = screen.getByRole("combobox", { name: i18n.t("reviewV2.timezone") });
    expect(timezone).toHaveValue("UTC");
    await userEvent.click(timezone);
    await userEvent.type(timezone, "Asia/Singapore");
    expect(screen.getByRole("option", { name: "Asia/Singapore" })).toBeVisible();
    await userEvent.keyboard("{Enter}");
    expect(timezone).toHaveFocus();
    expect(timezone).toHaveValue("Asia/Singapore");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(state.save).toHaveBeenCalledOnce());
    expect((state.save.mock.calls[0][0] as { spec: { schedule: { timezone: string } } }).spec.schedule.timezone).toBe("Asia/Singapore");
  });
  it("uses the IANA timezone chooser and cancels search text on Escape and Tab", async () => {
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} />);
    const timezone = screen.getByRole("combobox", { name: i18n.t("reviewV2.timezone") });
    expect(timezone).toHaveValue("Asia/Singapore");
    await userEvent.click(timezone);
    expect(screen.queryByRole("option", { name: "system" })).not.toBeInTheDocument();
    await userEvent.type(timezone, "Pacific/Auckland");
    await userEvent.keyboard("{Escape}");
    expect(timezone).toHaveValue("Asia/Singapore");
    expect(timezone).toHaveFocus();
    await userEvent.click(timezone);
    await userEvent.type(timezone, "Pacific/Auckland");
    await userEvent.tab();
    expect(timezone).toHaveValue("Asia/Singapore");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });
  it("snapshots the household timezone and defaults new money conditions to its base currency", async () => {
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} />);
    expect(screen.getByRole("combobox", { name: i18n.t("reviewV2.timezone") })).toHaveValue("Asia/Singapore");
    expect(screen.getByRole("combobox", { name: "Currency" })).toHaveValue("USD");
    await userEvent.type(screen.getByLabelText("Name"), "Synthetic threshold");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(state.save).toHaveBeenCalledOnce());
    const saved = state.save.mock.calls[0][0] as { spec: { schedule: { timezone: string }; predicate: { all: Array<{ leaf: { currency?: string } }> } } };
    expect(saved.spec.schedule.timezone).toBe("Asia/Singapore");
    expect(saved.spec.predicate.all[0].leaf.currency).toBe("USD");
  });
  it("keeps a cleared new-rule currency blank until the user selects a replacement", async () => {
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} />);
    const currency = screen.getByRole("combobox", { name: "Currency" });
    expect(currency).toHaveValue("USD");
    await userEvent.selectOptions(currency, "");
    expect(currency).toHaveValue("");
    expect(currency).toHaveAttribute("aria-invalid", "true");
    expect(document.getElementById(currency.getAttribute("aria-describedby")!)).toHaveTextContent(i18n.t("reviewV2.currencyInvalid"));
    await userEvent.selectOptions(currency, "EUR");
    expect(currency).toHaveValue("EUR");
    await userEvent.type(screen.getByLabelText("Name"), "Synthetic EUR threshold");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(state.save).toHaveBeenCalledOnce());
    expect((state.save.mock.calls[0][0] as { spec: { predicate: { all: Array<{ leaf: { currency?: string } }> } } }).spec.predicate.all[0].leaf.currency).toBe("EUR");
  });
  it("keeps the saved currency visible while the catalog is loading and preserves it on save", async () => {
    state.catalog = undefined;
    const onOpenChange = vi.fn();
    render(<ReviewRuleEditor open onOpenChange={onOpenChange} rule={rule} />);
    const currency = screen.getByRole("combobox", { name: "Currency" });
    expect(currency).toHaveValue("CNY");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(state.save).toHaveBeenCalledOnce());
    const saved = state.save.mock.calls[0][0] as { spec: { predicate: typeof predicate } };
    expect(saved.spec.predicate.any?.[1].leaf?.currency).toBe("CNY");
  });
  it("reports currency catalog errors and retries while retaining the household fallback", async () => {
    state.catalog = undefined;
    state.catalogError = true;
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} />);
    expect(screen.getByText(i18n.t("reviewV2.currencyCatalogError"))).toBeVisible();
    expect(screen.getByRole("combobox", { name: "Currency" })).toHaveValue("USD");
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(state.catalogRefetch).toHaveBeenCalledOnce();
  });
  it("allows a catalog currency to replace an existing monetary condition currency", async () => {
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} rule={rule} />);
    const currency = screen.getAllByRole("combobox", { name: "Currency" })[0];
    expect(currency).toHaveValue("CNY");
    await userEvent.selectOptions(currency, "EUR");
    expect(currency).toHaveValue("EUR");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(state.save).toHaveBeenCalledOnce());
    const saved = state.save.mock.calls[0][0] as { spec: { predicate: typeof predicate } };
    expect(saved.spec.predicate.any[1].leaf).toEqual({ metric: "cost", operator: ">", value: "1000", currency: "EUR", fxMode: "native" });
  });
  it("seeds an untouched new currency when bootstrap arrives after the editor mounts", async () => {
    state.bootstrap = undefined as unknown as { household: { baseCurrency: string } };
    const onOpenChange = vi.fn();
    const view = render(<ReviewRuleEditor open onOpenChange={onOpenChange} />);
    const currency = screen.getByRole("combobox", { name: "Currency" });
    expect(currency).toHaveValue("");
    state.bootstrap = { household: { baseCurrency: "USD" } };
    view.rerender(<ReviewRuleEditor open onOpenChange={onOpenChange} />);
    await waitFor(() => expect(currency).toHaveValue("USD"));
  });
  it("seeds additional conditions added before delayed bootstrap", async () => {
    state.bootstrap = undefined as unknown as { household: { baseCurrency: string } };
    const onOpenChange = vi.fn();
    const view = render(<ReviewRuleEditor open onOpenChange={onOpenChange} />);
    await userEvent.click(screen.getByRole("button", { name: "Add condition to group 1" }));
    expect(screen.getAllByRole("combobox", { name: "Currency" }).map(input => (input as HTMLSelectElement).value)).toEqual(["", ""]);
    state.bootstrap = { household: { baseCurrency: "USD" } };
    view.rerender(<ReviewRuleEditor open onOpenChange={onOpenChange} />);
    await waitFor(() => expect(screen.getAllByRole("combobox", { name: "Currency" }).map(input => (input as HTMLSelectElement).value)).toEqual(["USD", "USD"]));
  });
  it("preserves a currency selected before delayed bootstrap completes", async () => {
    state.bootstrap = undefined as unknown as { household: { baseCurrency: string } };
    const onOpenChange = vi.fn();
    const view = render(<ReviewRuleEditor open onOpenChange={onOpenChange} />);
    const currency = screen.getByRole("combobox", { name: "Currency" });
    await userEvent.selectOptions(currency, "EUR");
    state.bootstrap = { household: { baseCurrency: "USD" } };
    view.rerender(<ReviewRuleEditor open onOpenChange={onOpenChange} />);
    await waitFor(() => expect(currency).toHaveValue("EUR"));
    await userEvent.type(screen.getByLabelText("Name"), "Synthetic delayed EUR threshold");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(state.save).toHaveBeenCalledOnce());
    expect((state.save.mock.calls[0][0] as { spec: { predicate: { all: Array<{ leaf: { currency?: string } }> } } }).spec.predicate.all[0].leaf.currency).toBe("EUR");
  });
  it("blocks saving while a monetary currency is intentionally blank", async () => {
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} />);
    await userEvent.type(screen.getByLabelText("Name"), "Synthetic blank currency");
    const currency = screen.getByRole("combobox", { name: "Currency" });
    await userEvent.selectOptions(currency, "");
    expect(currency).toHaveValue("");
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(state.save).not.toHaveBeenCalled();
  });
  it("keeps new-rule submission disabled until the household timezone is loaded", async () => {
    state.origin = undefined as unknown as { timezone: string };
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} />);
    await userEvent.type(screen.getByLabelText("Name"), "Synthetic threshold");
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(state.save).not.toHaveBeenCalled();
  });
  it("initializes timezone after delayed history bootstrap without locking user edits", async () => {
    state.origin = undefined as unknown as { timezone: string };
    const onOpenChange = vi.fn();
    const view = render(<ReviewRuleEditor open onOpenChange={onOpenChange} />);
    const timezone = screen.getByRole("combobox", { name: i18n.t("reviewV2.timezone") });
    expect(timezone).toHaveValue("");
    state.origin = { timezone: "Asia/Singapore" };
    view.rerender(<ReviewRuleEditor open onOpenChange={onOpenChange} />);
    await waitFor(() => expect(timezone).toHaveValue("Asia/Singapore"));
    await userEvent.click(timezone);
    await userEvent.type(timezone, "Pacific/Auckland");
    await userEvent.keyboard("{Enter}");
    await userEvent.type(screen.getByLabelText("Name"), "Synthetic delayed timezone");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(state.save).toHaveBeenCalledOnce());
    expect((state.save.mock.calls[0][0] as { spec: { schedule: { timezone: string } } }).spec.schedule.timezone).toBe("Pacific/Auckland");
  });
  it("blocks nonnumeric amounts in a new rule after household timezone initialization", async () => {
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} />);
    await userEvent.type(screen.getByLabelText("Name"), "Synthetic threshold");
    await userEvent.selectOptions(screen.getByRole("combobox", { name: i18n.t("reviewV2.previewInstrument") }), "synthetic-fund");
    const amount = screen.getByRole("textbox", { name: "Value" });
    await userEvent.clear(amount);
    await userEvent.type(amount, "not-a-number");
    expect(screen.getByRole("combobox", { name: i18n.t("reviewV2.timezone") })).toHaveValue("Asia/Singapore");
    expect(screen.getByRole("button", { name: "Preview" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(state.save).not.toHaveBeenCalled();
  });
  it("keeps every selected instrument type in a simple daily condition", async () => {
    const simple = { ...rule, spec: { ...rule.spec, predicate: { all: [{ leaf: { metric: "type", operator: "in", types: ["mutual_fund"] } }] } } } as unknown as ReviewRule;
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} rule={simple} />);
    await userEvent.click(screen.getByRole("checkbox", { name: "ETF" }));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(state.save).toHaveBeenCalledOnce());
    const saved = state.save.mock.calls[0][0] as { spec: { predicate: typeof simple.spec.predicate } };
    expect(saved.spec.predicate.all?.[0].leaf?.types).toEqual(["mutual_fund", "etf"]);
  });
  it.each([null, { all: [null] }, { all: [{ leaf: null }] }])("does not crash or save malformed null predicate data", invalidPredicate => {
    const malformed = { ...rule, spec: { ...rule.spec, predicate: invalidPredicate } } as unknown as ReviewRule;
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} rule={malformed} />);
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(state.save).not.toHaveBeenCalled();
  });
  it.each([
    [0, "1e2"],
    [1, "not-a-number"],
    [0, "1.123456789"],
  ])("blocks invalid numeric thresholds before preview or save", async (valueIndex, value) => {
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} rule={rule} />);
    const threshold = screen.getAllByRole("textbox", { name: "Value" })[valueIndex];
    await userEvent.clear(threshold);
    await userEvent.type(threshold, value);
    expect(screen.getByRole("button", { name: "Preview" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(state.save).not.toHaveBeenCalled();
  });
  it("allows exactly twenty conditions and disables another condition", async () => {
    const nineteenConditions = { all: Array.from({ length: 19 }, (_, index) => ({ leaf: { metric: "amount", operator: ">", value: String(index + 1), currency: "CNY", fxMode: "native" } })) };
    const nineteenConditionRule = { ...rule, spec: { ...rule.spec, predicate: nineteenConditions } } as unknown as ReviewRule;
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} rule={nineteenConditionRule} />);
    const addGroup = screen.getByRole("button", { name: "Add condition group" });
    expect(addGroup).toBeEnabled();
    await userEvent.click(addGroup);
    expect(screen.getByRole("button", { name: "Add condition to group 2" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(state.save).toHaveBeenCalledOnce());
    const saved = state.save.mock.calls[0][0] as { spec: { predicate: { all: unknown[] } } };
    expect(saved.spec.predicate.all).toHaveLength(20);
    expect(saved.spec.predicate.all[19]).toEqual({ all: [{ leaf: expect.objectContaining({ metric: "amount", value: "1000" }) }] });
  });
  it.each([
    { all: Array.from({ length: 21 }, (_, index) => ({ leaf: { metric: "amount", operator: ">", value: String(index + 1), currency: "CNY", fxMode: "native" } })) },
    { all: [{ any: [{ all: [{ leaf: { metric: "amount", operator: ">", value: "1", currency: "CNY", fxMode: "native" } }] }] }] },
  ])("rejects predicates beyond the supported leaf count or group depth", predicateValue => {
    const invalid = { ...rule, spec: { ...rule.spec, predicate: predicateValue } } as unknown as ReviewRule;
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} rule={invalid} />);
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(state.save).not.toHaveBeenCalled();
  });
  it("keeps saving disabled when an annualized leaf has no bank-product all gate", async () => {
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} rule={rule} />);
    const metrics = screen.getAllByRole("combobox", { name: "Metric" });
    await userEvent.selectOptions(metrics[0], "annualized");
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(state.save).not.toHaveBeenCalled();
  });
  it.each([7, 30, 90, 180, 365])("submits the supported %s-day performance lookback", async days => {
    const lookbackRule = {
      ...rule,
      spec: { ...rule.spec, predicate: { all: [{ leaf: { metric: "price_change", operator: "<", value: "-5", lookbackDays: 30 } }] } },
    } as unknown as ReviewRule;
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} rule={lookbackRule} />);
    const lookback = screen.getByRole("combobox", { name: i18n.t("reviewV2.lookbackDays") });
    expect(Array.from((lookback as HTMLSelectElement).options).map(option => Number(option.value))).toEqual([7, 30, 90, 180, 365]);
    await userEvent.selectOptions(lookback, String(days));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(state.save).toHaveBeenCalledOnce());
    expect((state.save.mock.calls[0][0] as { spec: { predicate: { all: Array<{ leaf: { lookbackDays?: number } }> } } }).spec.predicate.all[0].leaf.lookbackDays).toBe(days);
  });
  it("defaults the lookback when switching to a performance metric and requires an explicit supported correction for legacy values", async () => {
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} />);
    await userEvent.selectOptions(screen.getByRole("combobox", { name: i18n.t("reviewV2.metric") }), "price_change");
    const newLookback = screen.getByRole("combobox", { name: i18n.t("reviewV2.lookbackDays") });
    expect(newLookback).toHaveValue("30");
    await userEvent.selectOptions(newLookback, "90");
    expect(newLookback).toHaveValue("90");
    await userEvent.type(screen.getByLabelText("Name"), "Lookback condition");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(state.save).toHaveBeenCalledOnce());
    expect((state.save.mock.calls[0][0] as { spec: { predicate: { all: Array<{ leaf: { lookbackDays?: number } }> } } }).spec.predicate.all[0].leaf.lookbackDays).toBe(90);
  });

  it("requires an explicit supported correction for an unsupported saved lookback", async () => {
    const invalidRule = {
      ...rule,
      spec: { ...rule.spec, predicate: { all: [{ leaf: { metric: "price_change", operator: "<", value: "-5", lookbackDays: 31 } }] } },
    } as unknown as ReviewRule;
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} rule={invalidRule} />);
    const legacyLookback = screen.getByRole("combobox", { name: i18n.t("reviewV2.lookbackDays") });
    expect(legacyLookback).toHaveValue("31");
    expect(legacyLookback).toHaveAttribute("aria-invalid", "true");
    expect(document.getElementById(legacyLookback.getAttribute("aria-describedby")!)).toHaveTextContent(i18n.t("reviewV2.lookbackInvalid"));
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    await userEvent.selectOptions(legacyLookback, "180");
    expect(legacyLookback).toHaveValue("180");
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  });
});
