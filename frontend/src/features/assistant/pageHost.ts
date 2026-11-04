import type { NavigationIntentV1, NavigationResult, PageContextV1, PagePublication } from "./contracts";
import { localId } from "./contracts";

const TTL = 120_000;
const safeText = (value: string) => value.slice(0, 240);
type Principal = Pick<PageContextV1, "householdId" | "ledgerIncarnation" | "assistantSessionId" | "permissionRevision" | "surface">;

/** Host-owned, memory-only, bounded context. No DOM, field values, notes,
 * tokens, provider state, arbitrary JSON or URLs enter this mapping. */
export class PageHost {
  private current: PageContextV1 | null = null;
  private targets = new Map<string, PagePublication["targets"][number]["destination"]>();
  private consumed = new Set<string>();
  private pageVersion = 0;
  constructor(private readonly principal: Principal, private readonly now = Date.now, private readonly viewKey?: string) {}

  revoke() { this.current = null; this.targets.clear(); this.consumed.clear(); this.pageVersion++; }
  publish(publication: PagePublication): PageContextV1 | null {
    this.revoke();
    if ((this.viewKey !== undefined && publication.viewKey !== this.viewKey) || publication.state !== "available" || publication.metrics.length > 32 || publication.targets.length > 64 ||
      !Number.isFinite(publication.observedAt) || publication.observedAt <= 0 || this.now() - publication.observedAt >= TTL || publication.observedAt > this.now()) return null;
    const contextId = localId();
    const context: PageContextV1 = {
      ...this.principal, version: 1, contextId, pageVersion: this.pageVersion,
      page: safeText(publication.page), capturedAt: new Date(publication.observedAt).toISOString(),
      expiresAt: new Date(Math.min(this.now() + TTL, publication.observedAt + TTL)).toISOString(),
      scope: { kind: publication.scope.kind, label: safeText(publication.scope.label) },
      range: { timezone: safeText(publication.timezone) },
      valuation: { basis: publication.basis, currency: publication.currency?.slice(0, 3) },
      filters: publication.filters?.includeArchived === undefined ? {} : { includeArchived: publication.filters.includeArchived === true },
      metrics: publication.metrics.map((m, i) => ({ ref: `${contextId}:m${i}`, label: safeText(m.label),
        value: m.value !== null && /^-?\d+(?:\.\d+)?$/.test(m.value) && m.value.length <= 128 ? m.value : null,
        unit: m.unit.slice(0, 16), basis: m.basis, status: m.value === null ? "unavailable" : m.status,
        missingReasons: m.missingReasons.slice(0, 16).map(safeText), evidenceRefs: [`${contextId}:e${i}`] })),
      targets: publication.targets.map((t, i) => { const ref = `${contextId}:t${i}`; this.targets.set(ref, structuredClone(t.destination)); return { ref, label: safeText(t.label), kind: "account" }; }),
    };
    context.metrics = context.metrics.map(metric => metric.value === null ? { ...metric, status: "unavailable" } : metric);
    const counts = new Map<string, number>();
    for (const target of context.targets) counts.set(target.label, (counts.get(target.label) ?? 0) + 1);
    context.targets = context.targets.map((target, index) => counts.get(target.label)! > 1 ? { ...target, label: `${target.label} (${index + 1})` } : target);
    if (new TextEncoder().encode(JSON.stringify(context)).length > 64 * 1024) { this.revoke(); return null; }
    this.current = structuredClone(context);
    return context;
  }
  snapshot(): PageContextV1 | null {
    return this.current && Date.parse(this.current.expiresAt) > this.now() ? structuredClone(this.current) : null;
  }
  navigate(intent: NavigationIntentV1, explicitUserTurnId: string, blocked: boolean,
    dispatch: (destination: PagePublication["targets"][number]["destination"]) => void): NavigationResult {
    const current = this.snapshot();
    if (!current || intent.contextId !== current.contextId || intent.pageVersion !== current.pageVersion) return "stale_context";
    if (intent.version !== 1 || intent.action !== "open_object" || !intent.intentId || intent.intentId.length > 128 || !intent.userTurnId || intent.userTurnId.length > 128 || intent.userTurnId !== explicitUserTurnId) return "invalid_target";
    if (this.consumed.has(intent.intentId)) return "duplicate";
    if (blocked) return "blocked";
    const target = this.targets.get(intent.targetRef);
    if (!target) return "invalid_target";
    // This stage accepts only a host-issued, read-only account destination.
    // A compromised/future publisher cannot hide a writer in navigation.
    if (!("mobileAccountId" in target ? this.principal.surface === "mobile" && Object.keys(target).length === 1 && Boolean(target.mobileAccountId) :
      this.principal.surface === "desktop" && target.page === "accounts" && "accountId" in target && Boolean(target.accountId) && Object.keys(target).every(key => key === "page" || key === "accountId"))) return "invalid_target";
    this.consumed.add(intent.intentId);
    if (this.consumed.size > 128) this.consumed.delete(this.consumed.values().next().value!);
    dispatch(structuredClone(target));
    return "opened";
  }
}
