import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/i18n";
import type { ReviewRule } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import { LegacyReviewConversionSheet } from "./LegacyReviewConversionSheet";

const state = vi.hoisted(() => ({
  preview: { mutateAsync: vi.fn(async (draft: unknown) => ({
    legacyRevision: 4, configRevision: 2, stateRevision: 3, sourceToken: "a".repeat(64), temporalToken: "b".repeat(64), lowYieldAlerts: 2, lossAlerts: 1, unknownOutcomes: 3, draft,
  })), isPending: false, isError: false, error: null },
  convert: { mutateAsync: vi.fn(async (_request: unknown) => []), isPending: false, isError: false, error: null },
}));
vi.mock("@/queries/investmentReview", () => ({
  useInvestmentReviews: () => ({ data: { timezone: "Asia/Shanghai" }, isError: false }),
}));
vi.mock("@/queries/investmentReviewV2", () => ({
  useReviewLegacyConversionCommands: () => state,
}));

const oldRule = {
  id: "synthetic-legacy-rule", rowRevision: 4,
  legacySpec: { configuration: { enrolled: { "synthetic-fund": true, "synthetic-bank": false }, policy: { sustainDays: 7, windowDays: 30 } } },
} as unknown as ReviewRule;

beforeEach(async () => {
  vi.clearAllMocks();
  await i18n.changeLanguage("en");
});

describe("legacy review conversion", () => {
  it("requires a fresh preview of the selected target and trigger before conversion", async () => {
    const close = vi.fn();
    const converted = vi.fn();
    render(<LegacyReviewConversionSheet rule={oldRule} onClose={close} onConverted={converted} />);
    expect(screen.queryByRole("button", { name: "Convert" })).not.toBeInTheDocument();
    expect(screen.getByText(/combined low yield OR loss/)).toBeVisible();
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Target mode" }), "all_current");
    expect(screen.getByText(/expands beyond/)).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Preview" }));
    await waitFor(() => expect(state.preview.mutateAsync).toHaveBeenCalledWith({
      legacyRuleId: oldRule.id, targetKind: "all_current", timezone: "Asia/Shanghai", triggerPolicy: { kind: "consecutive", count: 7 },
    }));
    expect(await screen.findByRole("button", { name: "Convert" })).toBeVisible();
    await userEvent.selectOptions(screen.getByRole("combobox", { name: "Alert after" }), "single");
    expect(screen.queryByRole("button", { name: "Convert" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Preview" }));
    await userEvent.click(await screen.findByRole("button", { name: "Convert" }));
    await waitFor(() => expect(state.convert.mutateAsync).toHaveBeenCalledTimes(1));
    expect(state.convert.mutateAsync).toHaveBeenCalledWith(expect.objectContaining({
      legacyRuleId: oldRule.id, targetKind: "all_current", timezone: "Asia/Shanghai", triggerPolicy: { kind: "single" },
      expectedRevision: 4, expectedConfigRevision: 2, expectedStateRevision: 3, expectedSourceToken: "a".repeat(64), expectedTemporalToken: "b".repeat(64),
      lowYieldRuleId: expect.any(String), lossRuleId: expect.any(String), mutationId: expect.any(String),
    }));
    expect(converted).toHaveBeenCalledOnce();
    expect(close).toHaveBeenCalledOnce();
  });

  it("retains one submission identity and blocks dismissal after an uncertain save", async () => {
    state.convert.mutateAsync.mockRejectedValueOnce(new Error("connection interrupted"));
    const close = vi.fn();
    render(<LegacyReviewConversionSheet rule={oldRule} onClose={close} onConverted={vi.fn()} />);
    await userEvent.click(screen.getByRole("button", { name: "Preview" }));
    await userEvent.click(await screen.findByRole("button", { name: "Convert" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "Close" })).toBeDisabled());
    await userEvent.keyboard("{Escape}");
    expect(close).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Convert" }));
    await waitFor(() => expect(state.convert.mutateAsync).toHaveBeenCalledTimes(2));
    expect(state.convert.mutateAsync.mock.calls[1][0]).toEqual(state.convert.mutateAsync.mock.calls[0][0]);
  });
});
