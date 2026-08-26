import { describe, expect, it } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/queries/keys";
import {
  invalidateCurrentValuation,
  invalidateHistoryReads,
  invalidateInstrumentReads,
  invalidateRefreshAll,
  invalidateRequiredFX,
  invalidateHoldingChange,
} from "@/queries/invalidation";

function clientWith(...keys: readonly (readonly unknown[])[]) {
  const queryClient = new QueryClient();
  for (const key of keys) {
    queryClient.setQueryData(key, "stale");
  }
  return queryClient;
}

function isInvalidated(queryClient: QueryClient, key: readonly unknown[]) {
  return queryClient.getQueryState(key)?.isInvalidated ?? false;
}

describe("query keys and dependency invalidation", () => {
  it("normalizes account filters and account ID lists before key construction", () => {
    expect(queryKeys.accounts.list()).toEqual(queryKeys.accounts.list({ includeArchived: false }));
    expect(queryKeys.holdings.byAccounts(["account-b", "account-a", "account-a"])).toEqual(
      queryKeys.holdings.byAccounts(["account-a", "account-b"]),
    );
  });

  it("keeps Start History inside the History namespace", () => {
    const queryClient = clientWith(
      queryKeys.history.origin,
      queryKeys.history.activities(50),
      queryKeys.overview.all,
      queryKeys.accounts.list(),
    );

    invalidateHistoryReads(queryClient);

    expect(isInvalidated(queryClient, queryKeys.history.origin)).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.history.activities(50))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.overview.all)).toBe(false);
    expect(isInvalidated(queryClient, queryKeys.accounts.list())).toBe(false);
  });

  it("does not refetch valuation when an Instrument is created or archived", () => {
    const queryClient = clientWith(
      queryKeys.instruments.list(),
      queryKeys.instruments.list(true),
      queryKeys.overview.all,
      queryKeys.quote.instrument.current("instrument-1"),
    );

    invalidateInstrumentReads(queryClient);

    expect(isInvalidated(queryClient, queryKeys.instruments.list())).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.instruments.list(true))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.overview.all)).toBe(false);
    expect(isInvalidated(queryClient, queryKeys.quote.instrument.current("instrument-1"))).toBe(false);
  });

  it("refreshes Overview and current gains for FX-only refresh, not raw quote or holding lists", () => {
    const queryClient = clientWith(
      queryKeys.overview.all,
      queryKeys.analytics.accountGain.current("account-1"),
      queryKeys.analytics.accountGain.current("account-2"),
      queryKeys.quote.instrument.current("instrument-1"),
      queryKeys.holdings.byAccounts(["account-1"]),
    );

    invalidateRequiredFX(queryClient);

    expect(isInvalidated(queryClient, queryKeys.overview.all)).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-1"))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-2"))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.quote.instrument.current("instrument-1"))).toBe(false);
    expect(isInvalidated(queryClient, queryKeys.holdings.byAccounts(["account-1"]))).toBe(false);
  });

  it("refreshes quote and valuation for Refresh All, not raw holdings", () => {
    const queryClient = clientWith(
      queryKeys.overview.all,
      queryKeys.quote.instrument.current("instrument-1"),
      queryKeys.analytics.accountGain.current("account-1"),
      queryKeys.holdings.byAccounts(["account-1"]),
    );

    invalidateRefreshAll(queryClient);

    expect(isInvalidated(queryClient, queryKeys.overview.all)).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.quote.instrument.current("instrument-1"))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-1"))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.holdings.byAccounts(["account-1"]))).toBe(false);
  });

  it("invalidates only the affected account gain for a holding change when its account is known", () => {
    const queryClient = clientWith(
      queryKeys.holdings.byAccounts(["account-1"]),
      queryKeys.overview.all,
      queryKeys.analytics.accountGain.current("account-1"),
      queryKeys.analytics.accountGain.current("account-2"),
    );

    invalidateHoldingChange(queryClient, "account-1");

    expect(isInvalidated(queryClient, queryKeys.holdings.byAccounts(["account-1"]))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.overview.all)).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-1"))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-2"))).toBe(false);
  });

  it("falls back to the account-gain prefix when affected accounts are unknown", () => {
    const queryClient = clientWith(
      queryKeys.overview.all,
      queryKeys.analytics.accountGain.current("account-1"),
      queryKeys.analytics.accountGain.current("account-2"),
    );

    invalidateCurrentValuation(queryClient);

    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-1"))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-2"))).toBe(true);
  });
});
