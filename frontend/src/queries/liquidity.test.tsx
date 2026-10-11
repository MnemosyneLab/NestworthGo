import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider, focusManager } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { upcomingData } from "@/test/upcomingActions";
import { queryKeys } from "./keys";
import { useUpcomingActions } from "./liquidity";
import { invalidateAccountChange, invalidateActivityChange, invalidateHoldingChange, invalidateInstrumentQuoteChange, invalidateLiquidityReads, invalidateRequiredFX } from "./invalidation";
const read = vi.hoisted(() => vi.fn());
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({ Service: { UpcomingActions: read } }));
afterEach(() => { vi.useRealTimers(); read.mockReset(); focusManager.setFocused(undefined); });
function harness() { const client = createTestQueryClient({ retry: false }); const wrapper = ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client }, children); return { client, wrapper }; }
describe("upcoming query", () => {
  it("shares a key and deduplicates concurrent summary/list reads", async () => {
    let finish!: (data: ReturnType<typeof upcomingData>) => void;
    read.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
    const { wrapper, client } = harness(); const a = renderHook(() => useUpcomingActions(), { wrapper }); const b = renderHook(() => useUpcomingActions(), { wrapper });
    expect(read).toHaveBeenCalledOnce(); await act(async () => { finish(upcomingData()); });
    await waitFor(() => expect(a.result.current.isSuccess && b.result.current.isSuccess).toBe(true));
    expect(client.getQueryData(queryKeys.liquidity.upcomingActions)).toEqual(upcomingData());
  });
  it("polls only active observers and rechecks after focus returns", async () => {
    vi.useFakeTimers(); focusManager.setFocused(true);
    read.mockResolvedValue(upcomingData()); const { wrapper } = harness();
    const hook = renderHook(({ active }) => useUpcomingActions(active), { wrapper, initialProps: { active: true } });
    await act(async () => { await vi.advanceTimersByTimeAsync(10); });
    expect(hook.result.current.isSuccess).toBe(true);
    await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
    expect(read).toHaveBeenCalledTimes(2);
    hook.rerender({ active: false });
    await act(async () => { await vi.advanceTimersByTimeAsync(120_000); });
    expect(read).toHaveBeenCalledTimes(2);
    hook.rerender({ active: true }); await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    expect(read).toHaveBeenCalledTimes(3);
    await act(async () => { focusManager.setFocused(false); focusManager.setFocused(true); await vi.advanceTimersByTimeAsync(0); });
    expect(read).toHaveBeenCalledTimes(4);
  });
  it.each([invalidateAccountChange, invalidateActivityChange, invalidateHoldingChange, (client: ReturnType<typeof createTestQueryClient>) => invalidateInstrumentQuoteChange(client, "instrument"), invalidateLiquidityReads, invalidateRequiredFX])("covers dependency invalidation %s", invalidate => {
    const { client } = harness(); client.setQueryData(queryKeys.liquidity.upcomingActions, upcomingData()); invalidate(client);
    expect(client.getQueryState(queryKeys.liquidity.upcomingActions)?.isInvalidated).toBe(true);
  });
});
