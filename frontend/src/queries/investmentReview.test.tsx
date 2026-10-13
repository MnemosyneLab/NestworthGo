import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider, focusManager } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { investmentReviewKey, useInvestmentReviews, useInvestmentReviewActions } from "./investmentReview";
import { invalidateActivityChange, invalidateRefreshAll, invalidateInstrumentQuoteChange } from "./invalidation";
const calls = vi.hoisted(() => ({ read: vi.fn(), save: vi.fn(), acknowledge: vi.fn(), enroll: vi.fn(), snooze: vi.fn() }));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({ Service: { InvestmentReviews: calls.read, SaveInvestmentReviewPolicy: calls.save, AcknowledgeInvestmentReview: calls.acknowledge, SetInvestmentReviewEnrollment: calls.enroll, SetInvestmentReviewSnooze: calls.snooze } }));
afterEach(() => { vi.useRealTimers(); Object.values(calls).forEach(call => call.mockReset()); focusManager.setFocused(undefined); });
function harness() { const client = createTestQueryClient({ retry: false }); const wrapper = ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client }, children); return { client, wrapper }; }
describe("investment review query integration", () => {
  it("deduplicates concurrent summary/list evaluation", async () => {
    let finish!: (value: unknown) => void; calls.read.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
    const { wrapper } = harness(); const a = renderHook(() => useInvestmentReviews(), { wrapper }); const b = renderHook(() => useInvestmentReviews(), { wrapper });
    expect(calls.read).toHaveBeenCalledOnce(); await act(async () => { finish({ items: [] }); });
    await waitFor(() => expect(a.result.current.isSuccess && b.result.current.isSuccess).toBe(true));
  });
  it("evaluates only active foreground views and resumes after focus", async () => {
    vi.useFakeTimers(); focusManager.setFocused(true); calls.read.mockResolvedValue({ items: [] });
    const { wrapper } = harness(); const hook = renderHook(({ active }) => useInvestmentReviews(active), { wrapper, initialProps: { active: true } });
    await act(async () => { await vi.advanceTimersByTimeAsync(10); }); expect(calls.read).toHaveBeenCalledTimes(1);
    await act(async () => { focusManager.setFocused(false); await vi.advanceTimersByTimeAsync(120_000); }); expect(calls.read).toHaveBeenCalledTimes(1);
    await act(async () => { focusManager.setFocused(true); await vi.advanceTimersByTimeAsync(10); }); expect(calls.read).toHaveBeenCalledTimes(2);
    hook.rerender({ active: false }); await act(async () => { await vi.advanceTimersByTimeAsync(120_000); }); expect(calls.read).toHaveBeenCalledTimes(2);
  });
  it.each([invalidateRefreshAll, invalidateActivityChange, (client: ReturnType<typeof createTestQueryClient>) => invalidateInstrumentQuoteChange(client, "synthetic")])("re-evaluates after authoritative invalidation %s", invalidate => {
    const { client } = harness(); client.setQueryData(investmentReviewKey, { items: [] }); invalidate(client);
    expect(client.getQueryState(investmentReviewKey)?.isInvalidated).toBe(true);
  });
  it("saves exact percent strings and reloads after acknowledge/enrollment", async () => {
    const { client, wrapper } = harness(); client.setQueryData(investmentReviewKey, { items: [] });
    calls.save.mockResolvedValue({ items: [] }); calls.acknowledge.mockResolvedValue({ items: [] }); calls.enroll.mockResolvedValue({ items: [] }); calls.snooze.mockResolvedValue({ items: [] });
    const hook = renderHook(() => useInvestmentReviewActions(), { wrapper });
    const request = { policy: { enabled: true, windowDays: 7, lowAnnualizedPercent: "1.25", lossPercent: "-0.5", sustainDays: 7, maxAgeDays: 3 }, expectedRevision: 2 };
    await act(async () => { await hook.result.current.save.mutateAsync(request); });
    expect(calls.save).toHaveBeenCalledWith(request); expect(client.getQueryState(investmentReviewKey)?.isInvalidated).toBe(true);
    await act(async () => { await hook.result.current.acknowledge.mutateAsync("episode"); }); expect(calls.acknowledge).toHaveBeenCalledWith("episode");
    await act(async () => { await hook.result.current.enroll.mutateAsync({ instrumentId: "synthetic", enabled: true, expectedRevision: 2 }); }); expect(calls.enroll).toHaveBeenCalledWith({ instrumentId: "synthetic", enabled: true, expectedRevision: 2 });
    await act(async () => { await hook.result.current.snooze.mutateAsync({ instrumentId: "synthetic", days: 7, expectedRevision: 2 }); });
    expect(calls.snooze).toHaveBeenCalledWith({ instrumentId: "synthetic", days: 7, expectedRevision: 2 });
    expect(client.getQueryState(investmentReviewKey)?.isInvalidated).toBe(true);
    await act(async () => { await hook.result.current.snooze.mutateAsync({ instrumentId: "synthetic", days: 0, expectedRevision: 3 }); });
    expect(calls.snooze).toHaveBeenLastCalledWith({ instrumentId: "synthetic", days: 0, expectedRevision: 3 });
  });
});
