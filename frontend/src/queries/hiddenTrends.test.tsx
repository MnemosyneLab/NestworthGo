import { describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { createTestQueryClient } from "@/test/queryClient";
import { queryKeys } from "@/queries/keys";
import { useNetWorthTrend } from "./analytics";
import { usePortfolioTrend } from "./portfolio";

const trendCalls = vi.hoisted(() => ({ netWorth: vi.fn(async () => ({ points: [] })), portfolio: vi.fn(async () => ({ points: [] })) }));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio", () => ({
  Service: { NetWorthTrend: trendCalls.netWorth, PortfolioTrend: trendCalls.portfolio },
}));

describe("hidden trend reads", () => {
  it("retains mounted query state and fetches invalidated data on reactivation", async () => {
    trendCalls.netWorth.mockClear();
    trendCalls.portfolio.mockClear();
    const client = createTestQueryClient({ retry: false });
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    const view = renderHook(({ active }) => ({
      netWorth: useNetWorthTrend({ kind: "trend", value: "30d" }, active),
      portfolio: usePortfolioTrend("30d", active),
    }), { wrapper, initialProps: { active: true } });
    await waitFor(() => expect(trendCalls.netWorth).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(trendCalls.portfolio).toHaveBeenCalledTimes(1));
    view.rerender({ active: false });
    await client.invalidateQueries({ queryKey: queryKeys.analytics.netWorthTrend({ kind: "trend", value: "30d" }) });
    await client.invalidateQueries({ queryKey: queryKeys.portfolio.trend("30d") });
    expect(trendCalls.netWorth).toHaveBeenCalledTimes(1);
    expect(trendCalls.portfolio).toHaveBeenCalledTimes(1);
    view.rerender({ active: true });
    await waitFor(() => expect(trendCalls.netWorth).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(trendCalls.portfolio).toHaveBeenCalledTimes(2));
    expect(view.result.current.netWorth.data).toBeDefined();
    expect(view.result.current.portfolio.data).toBeDefined();
  });
});
