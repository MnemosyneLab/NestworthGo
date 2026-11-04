import { describe, expect, it, vi } from "vitest";
import { PageHost } from "./pageHost";
import { isClosedStatus, type NavigationIntentV1, type PagePublication } from "./contracts";
import { overviewPublication } from "./publications";

const now = 1_800_000_000_000;
const principal = { householdId: "synthetic-household", ledgerIncarnation: "incarnation", assistantSessionId: "session", permissionRevision: "permission", surface: "desktop" as const };
export const publication: PagePublication = { viewKey: "accounts:", page: "accounts", scope: { kind: "household", label: "Synthetic accounts" },
  timezone: "Asia/Shanghai", basis: "native", observedAt: now, state: "available", currency: "CNY",
  metrics: [{ label: "Cash", value: "0", unit: "CNY", basis: "account_value", status: "available", missingReasons: [] }],
  targets: [{ label: "Synthetic bank", destination: { page: "accounts", accountId: "bank-a" } }, { label: "Synthetic bank", destination: { page: "accounts", accountId: "bank-b" } }] };

describe("bounded local page host", () => {
  it("publishes detached exact facts, opaque refs and separate identical-name choices", () => {
    const host = new PageHost(principal, () => now);
    const context = host.publish({ ...publication, apiKey: "SYNTHETIC-SECRET", note: "untrusted instructions" } as PagePublication)!;
    expect(context.metrics[0].value).toBe("0");
    expect(context.targets.map(t => t.label)).toEqual(["Synthetic bank (1)", "Synthetic bank (2)"]);
    expect(JSON.stringify(context)).not.toMatch(/bank-a|bank-b|SYNTHETIC-SECRET|untrusted instructions/);
    context.metrics[0].value = "99999";
    expect(host.snapshot()?.metrics[0].value).toBe("0");
  });
  it("consumes an explicit user choice once and preserves the navigation guard", () => {
    const host = new PageHost(principal, () => now); const context = host.publish(publication)!; const dispatch = vi.fn();
    const intent: NavigationIntentV1 = { version: 1, intentId: "intent", userTurnId: "turn", contextId: context.contextId, pageVersion: context.pageVersion, action: "open_object", targetRef: context.targets[1].ref };
    expect(host.navigate(intent, "another-turn", false, dispatch)).toBe("invalid_target");
    expect(host.navigate(intent, "turn", true, dispatch)).toBe("blocked");
    expect(host.navigate(intent, "turn", false, dispatch)).toBe("opened");
    expect(dispatch).toHaveBeenCalledExactlyOnceWith({ page: "accounts", accountId: "bank-b" });
    expect(host.navigate(intent, "turn", false, dispatch)).toBe("duplicate");
    const next = host.publish(publication)!;
    expect(next.pageVersion).toBeGreaterThan(context.pageVersion);
    expect(host.navigate(intent, "turn", false, dispatch)).toBe("stale_context");
    host.revoke(); expect(host.snapshot()).toBeNull();
  });
  it("rejects mutation flags even inside host-issued destinations", () => {
    const host = new PageHost(principal, () => now);
    const context = host.publish({ ...publication, targets: [{ label: "Unsafe", destination: { page: "accounts", accountId: "bank", updateValue: true } }] })!;
    const dispatch = vi.fn();
    expect(host.navigate({ version: 1, intentId: "i", userTurnId: "t", contextId: context.contextId, pageVersion: context.pageVersion, action: "open_object", targetRef: context.targets[0].ref }, "t", false, dispatch)).toBe("invalid_target");
    expect(dispatch).not.toHaveBeenCalled();
  });
  it("expires without renewal and refuses stale, oversized and unavailable publications", () => {
    let clock = now; const host = new PageHost(principal, () => clock); host.publish(publication);
    clock += 120_000; expect(host.snapshot()).toBeNull(); expect(host.publish(publication)).toBeNull();
    expect(host.publish({ ...publication, observedAt: clock, metrics: Array(33).fill(publication.metrics[0]) })).toBeNull();
    expect(host.publish({ ...publication, observedAt: clock, targets: Array(65).fill(publication.targets[0]) })).toBeNull();
    expect(host.publish({ ...publication, observedAt: clock, state: "unavailable" })).toBeNull();
  });
  it("keeps missing values unavailable and never silently rounds exact decimals", () => {
    const host = new PageHost(principal, () => now);
    const context = host.publish({ ...publication, metrics: [
      { ...publication.metrics[0], value: "0.000000000000000123" },
      { ...publication.metrics[0], value: "NaN" }, { ...publication.metrics[0], value: null },
    ] })!;
    expect(context.metrics.map(m => m.value)).toEqual(["0.000000000000000123", null, null]);
    expect(context.metrics[1].status).toBe("unavailable");
  });
});

it("does not infer assistant authority from future status values or key presence", () => {
  const status = { version: 1, surface: "desktop", pageGuide: true, provider: "unavailable", credentials: "unavailable", conversations: "memory", modelRuns: false, financialWrites: false, reason: "activation_incomplete" };
  expect(isClosedStatus(status, "desktop")).toBe(true);
  for (const override of [{ version: 2 }, { surface: "mobile" }, { modelRuns: true }, { financialWrites: true }, { credentials: "present" }, { provider: "ready" }]) expect(isClosedStatus({ ...status, ...override }, "desktop")).toBe(false);
});

it("uses the page's display authority, exact subtotal and liability sign without inventing totals", () => {
  const quality = { availability: "partial", coverage: "unavailable", includedCount: 2, valuedCount: 1, unpricedCount: 1, provisionalCount: 0, carriedCount: 0, staleCount: 0 };
  const known = { total: null, knownSubtotal: { amount: "0.004", currency: "CNY" }, quality };
  const absent = { total: null, knownSubtotal: null, quality };
  const context = overviewPublication({ currency: "CNY", complete: false, netWorth: "99999", assets: "99999", liabilities: "99999", displayValuation: { netWorth: absent, assets: known, liabilities: known } }, { observedAt: now, loading: false, error: false, timezone: "UTC", label: "Overview", labels: ["Net worth", "Assets", "Liabilities"] });
  expect(context.metrics.map(m => [m.value, m.status])).toEqual([[null, "unavailable"], ["0.004", "incomplete"], ["-0.004", "incomplete"]]);
});
