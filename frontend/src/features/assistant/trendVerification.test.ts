import { expect, it, vi } from "vitest";
import { TrendVerification, type TrendObservation } from "./trendVerification";
const expected: TrendObservation = {accountId: "", from: "2026-02-01", to: "2026-02-28", valuation: "base", includeCash: true, display: "linked_rate", currency: "", assetClass: "", status: "ready"};
it("spends a waiter once, cancels concurrent requests, and never reuses old acknowledgements", async () => {
  const host = new TrendVerification();
  host.observe(expected);
  const first = host.begin(expected); const second = host.begin({...expected, to: "2026-03-01"});
  expect(await first).toBe("unverified");
  host.observe(expected);
  expect(await second).toBe("unverified");
  host.observe({...expected, to: "2026-03-01"});
});
it("bounds an absent component acknowledgement and clears its timer", async () => {
  vi.useFakeTimers();
  try {
    const host = new TrendVerification(); const waiting = host.begin(expected);
    host.observe({...expected, status: "loading"});
    await vi.advanceTimersByTimeAsync(5000);
    expect(await waiting).toBe("unverified"); expect(vi.getTimerCount()).toBe(0);
    host.observe(expected);
  } finally { vi.useRealTimers(); }
});
