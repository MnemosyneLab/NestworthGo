import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/i18n";
import { ReviewEpisodeHistorySheet } from "./ReviewEpisodeHistorySheet";

const state = vi.hoisted(() => ({
  historyCall: vi.fn(), eventCall: vi.fn(),
  history: { data: { items: [{ episodeId: "synthetic-old", ruleId: "synthetic-rule", ruleName: "Synthetic rule", instrumentId: "synthetic-fund", instrumentName: "Synthetic fund", positionEpochId: "synthetic-epoch", openedConditionVersion: 1, firstHitOn: "2026-10-01", triggeredOn: "2026-10-02", firstDetectedAt: "2026-10-03T12:00:00Z", latestClosedOn: "2026-10-04", windowStart: "2026-10-01", windowEnd: "2026-10-02", hitCount: 2, falseCount: 0, unknownCount: 0, acknowledgedAt: "2026-10-03T13:00:00Z", resolvedAt: "2026-10-04T12:00:00Z", resolvedReason: "position_closed", revision: 3 }], nextCursor: "next-history", reportRevision: "history-1", ruleId: "synthetic-rule" }, isPending: false, isError: false, isFetching: false, refetch: vi.fn() },
  events: { data: { items: [{ id: "synthetic-event", episodeId: "synthetic-old", kind: "resolved", payloadJson: "{}", createdAt: "2026-10-04T12:00:00Z" }], nextCursor: "", reportRevision: "events-1", episodeId: "synthetic-old" }, isPending: false, isError: false, isFetching: false, refetch: vi.fn() },
}));
vi.mock("@/queries/investmentReviewV2", () => ({
  useReviewRulesV2: () => ({ data: [{ id: "synthetic-rule", name: "Synthetic rule", evaluator: "daily_predicate_v1" }], isPending: false, isError: false, hasNextPage: false }),
  useReviewEpisodeHistoryPage: (ruleId: string, cursor: string, active: boolean) => { state.historyCall(ruleId, cursor, active); return state.history; },
  useReviewEpisodeEventPage: (episodeId: string, cursor: string, active: boolean) => { state.eventCall(episodeId, cursor, active); return state.events; },
}));

beforeEach(async () => { vi.clearAllMocks(); await i18n.changeLanguage("en"); });

describe("investment review history", () => {
  it("shows a resolved episode and opens its paged event timeline", async () => {
    render(<ReviewEpisodeHistorySheet open onOpenChange={vi.fn()} />);
    expect(screen.getByText("Synthetic fund")).toBeVisible();
    expect(screen.getByText("Resolved: position closed")).toBeVisible();
    await userEvent.click(screen.getByText("Events"));
    await waitFor(() => expect(state.eventCall).toHaveBeenCalledWith("synthetic-old", "", true));
    expect(screen.getByText("Resolved")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(state.historyCall).toHaveBeenLastCalledWith("synthetic-rule", "next-history", true);
  });
});
