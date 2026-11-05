import type { NavigationTarget } from "@/app/navigation";

export type AssistantStatus = {
  version: 1; surface: "desktop" | "mobile"; pageGuide: boolean;
  provider: "unavailable"; credentials: "unavailable"; conversations: "memory";
  modelRuns: false; financialWrites: false; reason: "activation_incomplete";
};
export type Metric = {
  ref: string; label: string; value: string | null; unit: string;
  basis: "net_worth" | "assets" | "liabilities" | "account_value";
  status: "available" | "incomplete" | "unavailable" | "stale";
  missingReasons: string[]; evidenceRefs: string[];
};
export type PageContextV1 = {
  version: 1; contextId: string; householdId: string; ledgerIncarnation: string;
  assistantSessionId: string; permissionRevision: string; surface: "desktop" | "mobile";
  pageVersion: number; capturedAt: string; expiresAt: string; page: string;
  scope: { kind: "household" | "account"; label: string };
  range: { timezone: string }; valuation: { basis: "native" | "base"; currency?: string };
  filters: { includeArchived?: boolean };
  metrics: Metric[]; targets: { ref: string; kind: "account"; label: string }[];
};
// Business IDs and destinations stay in the host's private mapping, outside
// serializable context/evidence. Navigation never accepts a model-supplied route.
export type PagePublication = {
  viewKey: string; page: string; scope: PageContextV1["scope"];
  timezone: string; currency?: string; basis: "native" | "base";
  observedAt: number; state: "available" | "loading" | "unavailable";
  filters?: PageContextV1["filters"]; metrics: Omit<Metric, "ref" | "evidenceRefs">[];
  targets: { label: string; destination: NavigationTarget | { mobileAccountId: string } }[];
};
export type NavigationIntentV1 = {
  version: 1; intentId: string; userTurnId: string; contextId: string;
  pageVersion: number; action: "open_object"; targetRef: string;
};
export type NavigationResult = "opened" | "stale_context" | "blocked" | "invalid_target" | "duplicate" | "unverified";

// This installed boundary is deliberately closed. Unexpected/future values
// cannot turn on a provider, secret form, writer or context disclosure.
export function isClosedStatus(value: unknown, surface: AssistantStatus["surface"]): value is AssistantStatus {
  if (!value || typeof value !== "object") return false;
  const v = value as Record<string, unknown>;
  return v.version === 1 && v.surface === surface && v.pageGuide === true &&
    v.provider === "unavailable" && v.credentials === "unavailable" &&
    v.conversations === "memory" && v.modelRuns === false && v.financialWrites === false && v.reason === "activation_incomplete";
}

export function localId() {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, byte => byte.toString(16).padStart(2, "0")).join("");
}
