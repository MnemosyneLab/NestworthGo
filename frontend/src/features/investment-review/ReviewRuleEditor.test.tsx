import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/i18n";
import type { ReviewRule } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import { ReviewRuleEditor } from "./ReviewRuleEditor";

const state = vi.hoisted(() => ({ save: vi.fn(async (_value: unknown) => ({})) }));
vi.mock("@/queries/investments", () => ({ useInstruments: () => ({ data: [{ id: "synthetic-fund", name: "Synthetic fund" }] }) }));
vi.mock("@/queries/settings", () => ({ useSettings: () => ({ data: { timezone: "UTC" } }) }));
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

beforeEach(async () => { vi.clearAllMocks(); await i18n.changeLanguage("en"); });

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
});
