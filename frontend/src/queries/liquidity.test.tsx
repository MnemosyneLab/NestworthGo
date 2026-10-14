import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider, focusManager } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { upcomingData } from "@/test/upcomingActions";
import { queryKeys } from "./keys";
import { useRecordProductOperation, useUpcomingActions, useProductOperations } from "./liquidity";
import { invalidateAccountChange, invalidateActivityChange, invalidateHoldingChange, invalidateInstrumentQuoteChange, invalidateLiquidityReads, invalidateRequiredFX } from "./invalidation";
const read = vi.hoisted(() => vi.fn());
const operationRead = vi.hoisted(() => vi.fn());
const recordProduct = vi.hoisted(() => vi.fn());
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({ Service: { UpcomingActions: read, ListOperations: operationRead, RecordProductOperation: recordProduct } }));
afterEach(() => { vi.useRealTimers(); read.mockReset(); operationRead.mockReset(); focusManager.setFocused(undefined); });
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

const historyPage = (head = "head", next: string | null = "older", allowed = true) => ({ operations: [{ id: head, kind: "open", effectiveAt: "2026-09-20T00:00:00Z", createdAt: "2026-09-20T00:00:00Z", reversesOperationId: null, reversedByOperationId: null }], next, latestRecordedOperationId: head, undo: { operation: { id: "off-page", kind: "open", effectiveAt: "2026-09-20T00:00:00Z", createdAt: "2026-09-20T00:00:00Z" }, allowed, reason: allowed ? null : "contract_changed" } });

describe("product operations sessions", () => {
  it("requests one page and one page per explicit load; newest summary and pending deduplication", async () => {
    operationRead.mockResolvedValueOnce(historyPage());
    let finish!: (data: ReturnType<typeof historyPage>) => void;
    operationRead.mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
    const { wrapper } = harness();
    const hook = renderHook(() => useProductOperations("p"), { wrapper });
    await waitFor(() => expect(hook.result.current.isSuccess).toBe(true));
    expect(operationRead).toHaveBeenCalledExactlyOnceWith({ productId: "p", limit: 25 });
    act(() => { void hook.result.current.loadMore(); void hook.result.current.loadMore(); });
    await waitFor(() => expect(operationRead).toHaveBeenCalledTimes(2));
    expect(operationRead.mock.calls[1][0]).toEqual({ productId: "p", limit: 25, cursor: "older" });
    await act(async () => finish(historyPage("head", null, false)));
    await waitFor(() => expect(hook.result.current.data?.pages).toHaveLength(2));
    expect(hook.result.current.data?.pages[1].undo.allowed).toBe(false);
  });
  it("keeps loaded rows and retries exactly the failed cursor", async () => {
    operationRead.mockResolvedValueOnce(historyPage()).mockRejectedValueOnce(new Error("failed")).mockResolvedValueOnce(historyPage("head", null));
    const { wrapper } = harness(); const hook = renderHook(() => useProductOperations("p"), { wrapper });
    await waitFor(() => expect(hook.result.current.isSuccess).toBe(true));
    await act(async () => { await hook.result.current.loadMore(); });
    await waitFor(() => expect(hook.result.current.isFetchNextPageError).toBe(true));
    expect(hook.result.current.data?.pages).toHaveLength(1);
    await act(async () => { await hook.result.current.loadMore(); });
    await waitFor(() => expect(hook.result.current.data?.pages).toHaveLength(2));
    expect(operationRead.mock.calls.slice(1).map(([r]) => r.cursor)).toEqual(["older", "older"]);
  });
  it("abandons a changed head and starts one fresh first page", async () => {
    operationRead.mockResolvedValueOnce(historyPage()).mockResolvedValueOnce(historyPage("changed", null)).mockResolvedValueOnce(historyPage("changed", "new-older"));
    const { wrapper } = harness(); const hook = renderHook(() => useProductOperations("p"), { wrapper });
    await waitFor(() => expect(hook.result.current.isSuccess).toBe(true));
    await act(async () => { await hook.result.current.loadMore(); });
    await waitFor(() => expect(operationRead).toHaveBeenCalledTimes(3));
    await waitFor(() => expect(hook.result.current.data?.pages).toHaveLength(1));
    expect(operationRead.mock.calls[2][0]).toEqual({ productId: "p", limit: 25 });
    expect(hook.result.current.data?.pages[0].latestRecordedOperationId).toBe("changed");
  });
  it("external invalidation, explicit refresh and reopen each reset to one page", async () => {
    operationRead.mockResolvedValue(historyPage());
    const { wrapper, client } = harness(); const hook = renderHook(({ enabled }) => useProductOperations("p", enabled), { wrapper, initialProps: { enabled: true } });
    await waitFor(() => expect(hook.result.current.isSuccess).toBe(true));
    await act(async () => { await hook.result.current.loadMore(); });
    await waitFor(() => expect(hook.result.current.data?.pages).toHaveLength(2));
    act(() => invalidateLiquidityReads(client));
    await waitFor(() => expect(hook.result.current.data?.pages).toHaveLength(1));
    expect(operationRead).toHaveBeenCalledTimes(3);
    act(() => hook.result.current.refresh());
    await waitFor(() => expect(operationRead).toHaveBeenCalledTimes(4));
    hook.rerender({ enabled: false }); hook.rerender({ enabled: true });
    await waitFor(() => expect(operationRead).toHaveBeenCalledTimes(5));
    expect(operationRead.mock.calls.filter(([r]) => r.cursor)).toHaveLength(1);
  });
  it.each(["success", "error"])("cancels a previous product and ignores its late %s", async outcome => {
    let finish!: (data: ReturnType<typeof historyPage>) => void; let reject!: (reason: Error) => void;
    const cancel = vi.fn();
    operationRead.mockImplementationOnce(() => Object.assign(new Promise((resolve, fail) => { finish = resolve; reject = fail; }), { cancel })).mockResolvedValueOnce(historyPage("new", null));
    const { wrapper } = harness(); const hook = renderHook(({ product }) => useProductOperations(product), { wrapper, initialProps: { product: "old" } });
    await waitFor(() => expect(operationRead).toHaveBeenCalledOnce());
    hook.rerender({ product: "new" });
    await waitFor(() => expect(hook.result.current.data?.pages[0].latestRecordedOperationId).toBe("new"));
    expect(cancel).toHaveBeenCalledOnce();
    await act(async () => { if (outcome === "success") finish(historyPage("old")); else reject(new Error("old failed")); });
    expect(hook.result.current.data?.pages).toHaveLength(1); expect(hook.result.current.isError).toBe(false);
    expect(hook.result.current.data?.pages[0].latestRecordedOperationId).toBe("new");
  });
});

it("allows Load more in a replacement session while an abandoned native next page never settles", async () => {
  const cancel = vi.fn();
  operationRead.mockResolvedValueOnce(historyPage("old")).mockImplementationOnce(() => Object.assign(new Promise(() => {}), { cancel })).mockResolvedValueOnce(historyPage("new")).mockResolvedValueOnce(historyPage("new", null));
  const { wrapper } = harness(); const hook = renderHook(({ product }) => useProductOperations(product), { wrapper, initialProps: { product: "old" } });
  await waitFor(() => expect(hook.result.current.isSuccess).toBe(true));
  act(() => { void hook.result.current.loadMore(); });
  await waitFor(() => expect(operationRead).toHaveBeenCalledTimes(2));
  hook.rerender({ product: "new" });
  await waitFor(() => expect(hook.result.current.data?.pages[0].latestRecordedOperationId).toBe("new"));
  expect(cancel).toHaveBeenCalledOnce();
  await act(async () => { await hook.result.current.loadMore(); });
  await waitFor(() => expect(hook.result.current.data?.pages).toHaveLength(2));
  expect(operationRead).toHaveBeenCalledTimes(4);
});


it("invalidates cached instrument ownership after creating or renewing a product", async () => {
 const {client,wrapper}=harness();
 client.setQueryData(queryKeys.instruments.list(true),[{id:"existing",name:"Synthetic instrument"}]);
 recordProduct.mockResolvedValue({productIds:["new-product"],operationId:"synthetic-operation"});
 const hook=renderHook(()=>useRecordProductOperation(),{wrapper});
 await act(async()=>{await hook.result.current.mutateAsync({mutationId:"synthetic-mutation",command:{kind:"open"}} as never);});
 expect(client.getQueryState(queryKeys.instruments.list(true))?.isInvalidated).toBe(true);
});
