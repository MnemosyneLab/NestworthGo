import type { QueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/queries/keys";

function invalidate(queryClient: QueryClient, queryKey: readonly unknown[]) {
  void queryClient.invalidateQueries({ queryKey });
}

export function invalidateAccountReads(queryClient: QueryClient) {
  invalidate(queryClient, queryKeys.accounts.all);
}

export function invalidateDirectoryReads(queryClient: QueryClient, entity: "members" | "institutions" | "groups") {
  invalidate(queryClient, queryKeys.directory.entity(entity));
}

export function invalidateDirectoryChange(queryClient: QueryClient, entity: "members" | "institutions" | "groups") {
  invalidateDirectoryReads(queryClient, entity);
  invalidateAccountReads(queryClient);
  invalidate(queryClient, queryKeys.overview.all);
}

export function invalidateHoldingReads(queryClient: QueryClient) {
  invalidate(queryClient, queryKeys.holdings.all);
}

export function invalidateInstrumentReads(queryClient: QueryClient) {
  invalidate(queryClient, queryKeys.instruments.all);
}

export function invalidateQuoteReads(queryClient: QueryClient, instrumentId?: string) {
  invalidate(queryClient, instrumentId ? queryKeys.quote.instrument.current(instrumentId) : queryKeys.quote.all);
}

export function invalidateCurrentValuation(queryClient: QueryClient, accountIds?: readonly string[]) {
  invalidate(queryClient, queryKeys.overview.all);
  invalidate(queryClient, queryKeys.portfolio.all);
  invalidate(queryClient, queryKeys.analytics.netWorthTrendPrefix);
  invalidateAccountReads(queryClient);
  if (accountIds && accountIds.length > 0) {
    for (const accountId of accountIds) {
      invalidate(queryClient, queryKeys.analytics.accountGain.current(accountId));
    }
    return;
  }
  invalidate(queryClient, queryKeys.analytics.accountGain.all);
}

export function invalidateHistoryReads(queryClient: QueryClient) {
  invalidate(queryClient, queryKeys.history.all);
}

export function invalidateAccountChange(queryClient: QueryClient, accountId?: string) {
  invalidateAccountReads(queryClient);
  invalidateCurrentValuation(queryClient, accountId ? [accountId] : undefined);
}

export function invalidateHoldingChange(queryClient: QueryClient, accountId?: string) {
  invalidateHoldingReads(queryClient);
  invalidateCurrentValuation(queryClient, accountId ? [accountId] : undefined);
}

export function invalidateActivityChange(queryClient: QueryClient, accountIds?: readonly string[]) {
  invalidateHistoryReads(queryClient);
  invalidateHoldingReads(queryClient);
  invalidateCurrentValuation(queryClient, accountIds);
  invalidate(queryClient, ["analytics", "realizedGain"]);
  invalidate(queryClient, ["analytics", "dividendIncome"]);
}

export function invalidateInstrumentQuoteChange(queryClient: QueryClient, instrumentId: string) {
  invalidateQuoteReads(queryClient, instrumentId);
  invalidateCurrentValuation(queryClient);
}

export function invalidateRefreshAll(queryClient: QueryClient) {
  invalidateQuoteReads(queryClient);
  invalidateCurrentValuation(queryClient);
}

export function invalidateRequiredFX(queryClient: QueryClient) {
  invalidate(queryClient, queryKeys.quote.fx.all);
  invalidate(queryClient, queryKeys.quote.fx.preferences);
  invalidateCurrentValuation(queryClient);
}
