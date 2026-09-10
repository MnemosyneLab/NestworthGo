import type { AccountFilterRequest } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account/models";

export function normalizeAccountFilter(filter: AccountFilterRequest = {}): AccountFilterRequest {
  return {
    includeArchived: filter.includeArchived === true,
    ...(filter.memberId ? { memberId: filter.memberId } : {}),
    ...(filter.institutionId ? { institutionId: filter.institutionId } : {}),
    ...(filter.groupId ? { groupId: filter.groupId } : {}),
    ...(filter.accountType ? { accountType: filter.accountType } : {}),
    ...(filter.ownershipScope ? { ownershipScope: filter.ownershipScope } : {}),
  };
}

export function normalizeAccountIds(accountIds: readonly string[]): string[] {
  return [...new Set(accountIds)].sort();
}

export const queryKeys = {
  app: {
    info: ["app", "info"] as const,
    startup: ["app", "startup"] as const,
  },
  household: {
    bootstrap: ["household", "bootstrap"] as const,
  },
  catalog: {
    all: ["catalog"] as const,
  },
  settings: {
    all: ["settings"] as const,
    supportedCurrencies: ["settings", "supportedCurrencies"] as const,
    fxProviders: ["settings", "fxProviders"] as const,
  },
  directory: {
    all: ["directory"] as const,
    entity: (entity: "members" | "institutions" | "groups") => ["directory", entity] as const,
    members: (includeArchived = false) => ["directory", "members", { includeArchived }] as const,
    institutions: (includeArchived = false) => ["directory", "institutions", { includeArchived }] as const,
    groups: (includeArchived = false) => ["directory", "groups", { includeArchived }] as const,
  },
  accounts: {
    all: ["accounts"] as const,
    list: (filter: AccountFilterRequest = {}) => ["accounts", "list", normalizeAccountFilter(filter)] as const,
    valuations: (filter: AccountFilterRequest = {}) => ["accounts", "valuations", normalizeAccountFilter(filter)] as const,
  },
  instruments: {
    all: ["instruments"] as const,
    list: (includeArchived = false) => ["instruments", "list", { includeArchived }] as const,
  },
  holdings: {
    all: ["holdings"] as const,
    byAccounts: (accountIds: readonly string[]) => ["holdings", "byAccounts", normalizeAccountIds(accountIds)] as const,
  },
  quote: {
    all: ["quote"] as const,
    instrument: {
      all: ["quote", "instrument"] as const,
      current: (instrumentId: string) => ["quote", "instrument", instrumentId] as const,
      series: (instrumentId: string, trendRange: string, sourceFilter: string) =>
        ["quote", "instrument", instrumentId, "series", trendRange, sourceFilter] as const,
    },
    fx: {
      all: ["quote", "fx"] as const,
      preferences: ["quote", "fx", "preferences"] as const,
      current: (currencyA: string, currencyB: string) => ["quote", "fx", ...[currencyA, currencyB].sort()] as const,
      series: (currencyA: string, currencyB: string, trendRange: string, sourceFilter: string) =>
        ["quote", "fx", currencyA, currencyB, "series", trendRange, sourceFilter] as const,
    },
  },
  overview: {
    all: ["overview"] as const,
  },
  portfolio: {
    all: ["portfolio"] as const,
    trend: (trendRange: string) => ["portfolio", "trend", trendRange] as const,
  },
  analytics: {
    all: ["analytics"] as const,
    realizedGain: (scope: unknown, range: unknown) => ["analytics", "realizedGain", scope, range] as const,
    dividendIncome: (scope: unknown, range: unknown) => ["analytics", "dividendIncome", scope, range] as const,
    accountGains: {
      all: ["analytics", "accountGains"] as const,
    },
    netWorthTrend: (range: unknown) => ["analytics", "netWorthTrend", range] as const,
    netWorthTrendPrefix: ["analytics", "netWorthTrend"] as const,
  },
  analysis: {
    all: ["analysis"] as const,
    assetChange: (request: unknown) => ["analysis", "assetChange", request] as const,
    assetDriverDetail: (request: unknown, driverKey: string) => ["analysis", "assetDriverDetail", request, driverKey] as const,
    returnCalendar: (request: unknown, cursor: string) => ["analysis", "returnCalendar", request, cursor] as const,
    returnDay: (request: unknown, date: string) => ["analysis", "returnDay", request, date] as const,
    returnTrend: (request: unknown, display: string) => ["analysis", "returnTrend", request, display] as const,
    contribution: (request: unknown, returnType: string, groupBy: string, ordering: string) =>
      ["analysis", "contribution", request, returnType, groupBy, ordering] as const,
    contributionItem: (request: unknown, returnType: string, groupBy: string, groupKey: string) =>
      ["analysis", "contributionItem", request, returnType, groupBy, groupKey] as const,
    assetTrend: (request: unknown, granularity: string, metric: string) =>
      ["analysis", "assetTrend", request, granularity, metric] as const,
    categories: (request: unknown, categoryType: string) => ["analysis", "categories", request, categoryType] as const,
    categoryDetail: (request: unknown, categoryType: string, rowKey: string) =>
      ["analysis", "categoryDetail", request, categoryType, rowKey] as const,
  },
  history: {
    all: ["history"] as const,
    origin: ["history", "origin"] as const,
    mutationAllowed: ["history", "mutationAllowed"] as const,
    snapshotState: ["history", "snapshotState"] as const,
    startingPointDraft: ["history", "startingPointDraft"] as const,
    activity: (activityId: string) => ["history", "activity", activityId] as const,
    activities: (limit: number) => ["history", "activities", limit] as const,
    activityPage: (request: unknown) => ["history", "activityPage", request] as const,
  },
  marketdata: {
    all: ["marketdata"] as const,
    currentSync: ["marketdata", "currentSync"] as const,
    job: (jobId: string) => ["marketdata", "job", jobId] as const,
  },
} as const;
