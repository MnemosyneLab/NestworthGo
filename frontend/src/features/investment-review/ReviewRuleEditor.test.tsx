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
  it("preserves a nested predicate and multi-type leaf when editing the rule name", async () => {
    render(<ReviewRuleEditor open onOpenChange={vi.fn()} rule={rule} />);
    expect(screen.getByRole("textbox", { name: "Grouped conditions (all/any JSON)" })).toHaveValue(JSON.stringify(predicate, null, 2));
    await userEvent.clear(screen.getByRole("textbox", { name: "Name" }));
    await userEvent.type(screen.getByRole("textbox", { name: "Name" }), "Renamed rule");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(state.save).toHaveBeenCalledOnce());
    expect(state.save.mock.calls[0][0]).toEqual(expect.objectContaining({ name: "Renamed rule", spec: expect.objectContaining({ predicate }) }));
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
});
