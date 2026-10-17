import React from "react";
import { createRoot } from "react-dom/client";
import { setTransport } from "@wailsio/runtime";
import "@/index.css";

const calls: Array<{ methodID?: number; args?: unknown[] }> = [];
const instrument = {
  id: "synthetic-second", householdId: "synthetic-household", name: "Synthetic Second Holding", shortName: "SYN2", type: "mutual_fund",
  quoteCurrency: "USD", symbol: "SYN", iconKey: "", sortOrder: 0, quoteSource: "manual", createdAt: "2026-01-01", updatedAt: "2026-01-01",
};
const firstInstrument = { ...instrument, id: "synthetic-first", name: "Synthetic First Holding", shortName: "SYN1", symbol: "SYN1" };
const amounts = { quantity: "3", totalCost: { amount: "30", currency: "USD" }, currentValue: { amount: "36", currency: "USD" }, unrealizedGain: { amount: "6", currency: "USD" } };
const group = (asset: typeof instrument) => ({ instrumentId: asset.id, name: asset.name, type: asset.type, symbol: asset.symbol, quoteCurrency: "USD", quantityUnit: "shares", archived: false,
  amounts, holdings: [{ holdingId: `position-${asset.id}`, accountId: "synthetic-broker", accountName: "Synthetic Broker", amounts }] });
const page = (cursor = "") => ({
  items: [{ instrumentId: cursor ? instrument.id : firstInstrument.id, displayName: cursor ? instrument.name : firstInstrument.name, instrumentType: instrument.type, currency: "USD", status: "pending", rules: [{
    ruleId: "synthetic-rule", name: "Synthetic threshold", severity: "warning", enabled: true, episodeId: "synthetic-episode", episodeRevision: 1,
    triggeredOn: "2026-10-15", firstHitOn: "2026-10-15", firstDetectedAt: "2026-10-15T12:00:00Z", latestClosedOn: "2026-10-15",
    window: { truth: "true", windowStart: "2026-10-15", windowEnd: "2026-10-15", hitCount: 1, falseCount: 0, unknownCount: 0, consecutiveCount: 1 },
    status: "pending", actionable: true, triggerPolicy: { kind: "single" }, coverage: [],
  }] }],
  nextCursor: cursor ? "" : "synthetic-page-two",
  summary: { totalGroups: 1, matchedGroups: 1, pendingGroups: 1, acknowledgedGroups: 0, ignoredGroups: 0, unknownGroups: 0, pausedGroups: 0, actionableRules: 1 },
  reportRevision: "synthetic-review-v2", projectionStatus: "complete", publishedOn: "2026-10-15",
});
const responses: Record<number, (args: unknown[]) => unknown> = {
  1866289316: () => ({ available: true }),
  3744917947: () => ({ schema_version: 1, appearance: "system", accent: "indigo", language: "en", timezone: "UTC", weekStart: "monday", dateFormat: "iso", timeFormat: "24h", currency: "USD", decimalSeparator: ".", groupingSeparator: ",", decimalPlaces: 2, windowWidth: 1280, windowHeight: 900, fxProvider: "frankfurter" }),
  3178430509: () => ({ household: { id: "synthetic-household", name: "Synthetic Household", baseCurrency: "USD", createdAt: "2026-01-01", updatedAt: "2026-01-01" }, members: [], institutions: [], groups: [] }),
  3181417489: () => ({ currency: "USD", accountCount: 1, complete: true, missingInputs: [], assets: "36", liabilities: "0", netWorth: "36", assetsByType: [], liabilitiesByType: [], byMember: [], byInstitution: [], byGroup: [], byAccountType: [] }),
  3771424334: () => ({ range: "30d", currency: "USD", points: [] }),
  247455835: () => ({ healthy: true, issueCount: 0, executableCount: 0, prerequisiteCount: 0, snapshotDays: 0, issues: [] }),
  1188744533: () => ({ asOf: "2026-10-16T12:00:00Z", localDate: "2026-10-16", timezone: "UTC", baseCurrency: "USD", assumptions: [], buckets: ["2026-10-16", "2026-10-23", "2026-11-15"].map(horizonOn => ({ horizonOn, status: "unavailable", fullAvailable: null, knownAvailableSubtotal: null, appliedReserveSubtotal: null, fullUnreserved: null, knownUnreservedSubtotal: null, unknownSourceCount: 0, excludedSourceCount: 0, estimatedSourceCount: 0, nativeCurrencyGroups: [], warnings: [] })), sources: [], unresolvedReservations: [] }),
  1263833643: () => ({ localDate: "2026-10-16", timezone: "UTC", asOf: "2026-10-16T12:00:00Z", completeness: "complete", counts: { total: 0, attention: 0, scheduled: 0, eligible: 0, dataChecks: 0, scopeNotes: 0 }, items: [], dataChecks: [], scopeNotes: [] }),
  3406455045: (args) => page(((args[0] as { cursor?: string } | undefined)?.cursor) ?? ""),
  3109784617: () => [],
  1180557780: () => ({}),
  3717597159: () => [firstInstrument, instrument],
  1074245374: () => [{ key: "mutual_fund", instruments: [group(firstInstrument), group(instrument)], subtotals: [] }],
  762255110: () => null,
  2727928460: () => ({ item: null, points: [], unavailableReason: "insufficient_daily_history" }),
  4025508000: () => ({ items: [] }),
  1600073255: () => [{ account: { id: "synthetic-broker", name: "Synthetic Broker", accountType: "brokerage", balanceSheetRole: "asset", trackingMode: "holdings", defaultCurrency: "USD", sortOrder: 0, includeInNetWorth: true, includeInPortfolio: true, includeInLiquidAssets: false, createdAt: "2026-01-01", updatedAt: "2026-01-01" }, ownership: [{ memberId: "synthetic-member", shareBps: 10000 }] }],
};

setTransport({ async call(objectID, method, _windowName, rpcArgs) {
  if (objectID !== 0 || method !== 0) return undefined;
  const request = rpcArgs as { methodID?: number; args?: unknown[] };
  calls.push(request);
  return responses[request.methodID ?? -1]?.(request.args ?? []) ?? null;
} });
(window as Window & { __syntheticCalls?: typeof calls; __integrationStatus?: string }).__syntheticCalls = calls;
(window as Window & { __integrationStatus?: string }).__integrationStatus = "transport-ready";

const [{ default: App }, { AppProviders, queryClient }, { reviewV2Key }] = await Promise.all([import("@/App"), import("@/app/providers"), import("@/queries/investmentReviewV2")]);
(window as Window & { __invalidateReview?: () => Promise<unknown> }).__invalidateReview = () => queryClient.invalidateQueries({ queryKey: reviewV2Key });
createRoot(document.getElementById("root")!).render(<AppProviders><App /></AppProviders>);
