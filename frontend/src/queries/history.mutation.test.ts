import { queryKeys } from "./keys";
import { upcomingData } from "@/test/upcomingActions";
import { describe, expect, it, vi } from "vitest";
import { createElement, type ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { act, renderHook } from "@testing-library/react";
import { useFixChange, useRecordChange, useStartHistory } from "@/queries/history";
import { Service as HistoryService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history";

vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({
  Service: {
    StartHistory: vi.fn(async () => ({})),
    StartHistoryWithCosts: vi.fn(async () => ({})),
    StartHistoryOnDate: vi.fn(async () => ({})),
    RecordChange: vi.fn(async () => ({ activity: { id: "a1" }, effects: [], resulting: [] })),
    FixChange: vi.fn(async () => ({ activity: { id: "a2" }, effects: [], resulting: [] })),
  },
}));

function renderMutation<T>(hook: () => T) {
  const queryClient = createTestQueryClient();
  return renderHook(hook, {
    wrapper: ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client: queryClient }, children),
  });
}

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

describe("history mutation IDs", () => {
  it("reuses mutationId when the same record payload is retried after failure", async () => {
    const recordChange = vi.mocked(HistoryService.RecordChange);
    recordChange.mockRejectedValueOnce(new Error("network"));
    const record = renderMutation(useRecordChange);
    const payload = { kind: "money_added" as const, accountId: "account-1", amount: "10", currency: "USD" };
    await act(async () => {
      await expect(record.result.current.mutateAsync(payload as never)).rejects.toThrow();
    });
    await act(async () => {
      await record.result.current.mutateAsync(payload as never);
    });
    expect(recordChange).toHaveBeenCalledTimes(2);
    const firstID = recordChange.mock.calls[0][0].mutationId;
    const secondID = recordChange.mock.calls[1][0].mutationId;
    expect(firstID).toMatch(uuidPattern);
    expect(secondID).toBe(firstID);
  });

  it("issues a new mutationId after a successful record", async () => {
    const recordChange = vi.mocked(HistoryService.RecordChange);
    recordChange.mockClear();
    const record = renderMutation(useRecordChange);
    const payload = { kind: "money_added" as const, accountId: "account-1", amount: "10", currency: "USD" };
    await act(async () => {
      await record.result.current.mutateAsync(payload as never);
    });
    await act(async () => {
      await record.result.current.mutateAsync(payload as never);
    });
    const firstID = recordChange.mock.calls[0][0].mutationId;
    const secondID = recordChange.mock.calls[1][0].mutationId;
    expect(firstID).toMatch(uuidPattern);
    expect(secondID).toMatch(uuidPattern);
    expect(secondID).not.toBe(firstID);
  });

  it("attaches mutationId to FixChange replacement commands", async () => {
    const fixChange = vi.mocked(HistoryService.FixChange);
    const fix = renderMutation(useFixChange);
    await act(async () => {
      await fix.result.current.mutateAsync({
        activityId: "activity-1",
        replacement: { kind: "money_added", accountId: "account-1", amount: "12", currency: "USD" } as never,
      });
    });
    expect(fixChange.mock.calls[0][1].mutationId).toMatch(uuidPattern);
  });
});

it.each([
  { timezone: "Asia/Shanghai" },
  { timezone: "Asia/Shanghai", costOverrides: {} },
  { timezone: "Asia/Shanghai", startDate: "2026-10-11" },
])("invalidates Upcoming Actions for every Start History branch: %o", async request => {
  const client = createTestQueryClient(); client.setQueryData(queryKeys.liquidity.upcomingActions, upcomingData());
  const hook = renderHook(useStartHistory, { wrapper: ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client }, children) });
  await act(async () => { await hook.result.current.mutateAsync(request); });
  expect(client.getQueryState(queryKeys.liquidity.upcomingActions)?.isInvalidated).toBe(true);
});
