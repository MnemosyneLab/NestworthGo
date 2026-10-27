import { useId, useLayoutEffect, useRef, useState, type ReactNode, type MutableRefObject } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { StatusBadge } from "./ReconciliationParts";
import type { ReconciliationAccount } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";

/** Presentation of the existing attestation/coverage state, never financial inference. */
export function accountIsVerified(account: ReconciliationAccount): boolean {
  const items = account.items ?? [];
  return account.outcome === "confirmed" && items.length > 0 &&
    items.some((item) => item.kind === "scope" && item.outcome === "confirmed") &&
    items.every((item) => item.outcome === "confirmed" && !item.recheckRequired) &&
    account.counts.confirmed === items.length &&
    account.counts.pending + account.counts.skipped + account.counts.unresolved === 0 &&
    !account.recheckRequired && !account.financialRevisionChanged &&
    !account.inventoryCoverageGap && !account.uncoveredArchivedHoldings?.length;
}

type Entry = { verified: boolean; expanded: boolean };
type View = { order: string[]; entries: Record<string, Entry> };
function reconcile(previous: View, accounts: ReconciliationAccount[]): View {
  const ids = new Set(accounts.map((account) => account.accountId));
  const order = previous.order.filter((id) => ids.has(id));
  for (const account of accounts) if (!order.includes(account.accountId)) order.push(account.accountId);
  let changed = order.length !== previous.order.length || order.some((id, i) => id !== previous.order[i]);
  const entries: View["entries"] = {};
  for (const account of accounts) {
    const old = previous.entries[account.accountId];
    const verified = accountIsVerified(account);
    entries[account.accountId] = !old || old.verified !== verified ? { verified, expanded: !verified } : old;
    changed ||= entries[account.accountId] !== old;
  }
  return changed ? { order, entries } : previous;
}

function scrollingParent(element: HTMLElement): HTMLElement | null {
  for (let parent = element.parentElement; parent; parent = parent.parentElement) {
    if (/(auto|scroll)/.test(getComputedStyle(parent).overflowY) && parent.scrollHeight > parent.clientHeight) return parent;
  }
  return element.ownerDocument.scrollingElement as HTMLElement | null;
}

export function ReconciliationAccountGroups({ accounts, locked, trigger, registerHeader, children, notices, actions }: {
  accounts: ReconciliationAccount[];
  locked: boolean;
  trigger: MutableRefObject<HTMLElement | null>;
  registerHeader: (id: string, element: HTMLButtonElement | null) => void;
  children: (account: ReconciliationAccount) => ReactNode;
  notices: (account: ReconciliationAccount) => ReactNode;
  actions?: (account: ReconciliationAccount) => ReactNode;
}) {
  const { t } = useTranslation();
  const id = useId();
  const root = useRef<HTMLDivElement>(null);
  const headers = useRef(new Map<string, HTMLButtonElement>());
  const wasLocked = useRef(false);
  const anchor = useRef<{ element: HTMLElement; top: number; scroll: HTMLElement | null; focus: boolean; view: View }>();
  const [view, setView] = useState(() => reconcile({ order: [], entries: {} }, accounts));

  useLayoutEffect(() => {
    if (locked) { wasLocked.current = true; return; }
    const next = reconcile(view, accounts);
    if (next !== view) {
      const active = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      const source = active && root.current?.contains(active) ? active : wasLocked.current ? trigger.current : null;
      const accountId = source?.closest("[data-reconciliation-account]")?.getAttribute("data-reconciliation-account");
      if (source && accountId) {
        let target = source;
        if (next.entries[accountId]?.verified !== view.entries[accountId]?.verified) {
          // Continue at the next unfinished account, or retain the reviewed header.
          const index = next.order.indexOf(accountId);
          const candidates = [...next.order.slice(index + 1), ...next.order.slice(0, index)];
          const targetId = next.entries[accountId]?.verified
            ? candidates.find((candidate) => !next.entries[candidate].verified) ?? accountId : accountId;
          target = headers.current.get(targetId) ?? source;
        }
        anchor.current = { element: target, top: Math.max(16, Math.min(source.getBoundingClientRect().top, window.innerHeight - 64)), scroll: scrollingParent(source), focus: true, view: next };
      }
      setView(next);
    }
    wasLocked.current = false;
  }, [accounts, locked, trigger, view]);

  useLayoutEffect(() => {
    const saved = anchor.current;
    if (!saved || locked || saved.view !== view) return;
    anchor.current = undefined;
    if (!saved.element.isConnected) return;
    if (saved.scroll) saved.scroll.scrollTop += saved.element.getBoundingClientRect().top - saved.top;
    if (saved.focus) {
      saved.element.focus({ preventScroll: true });
      trigger.current = saved.element;
    }
  }, [view, locked, trigger]);

  const byId = new Map(accounts.map((account) => [account.accountId, account]));
  const ordered = view.order.flatMap((accountId) => byId.has(accountId) ? [byId.get(accountId)!] : []);
  const toggle = (accountId: string) => setView((old) => ({ ...old, entries: { ...old.entries, [accountId]: { ...old.entries[accountId], expanded: !old.entries[accountId].expanded } } }));
  const all = (expanded: boolean) => setView((old) => ({ ...old, entries: Object.fromEntries(Object.entries(old.entries).map(([key, entry]) => [key, { ...entry, expanded }])) }));
  const rows: ReactNode[] = [];
  for (const verified of [false, true]) {
    const group = ordered.filter((account) => view.entries[account.accountId].verified === verified);
    rows.push(<h3 key={`heading-${verified}`} className="pt-2 text-base font-semibold">{t(verified ? "reconciliation.verifiedAccounts" : "reconciliation.unfinishedAccounts", { count: group.length })}</h3>);
    if (!group.length) rows.push(<p key={`empty-${verified}`} className="text-sm text-muted-foreground">{t(verified ? "reconciliation.noVerifiedAccounts" : "reconciliation.noUnfinishedAccounts")}</p>);
    for (const account of group) {
      const expanded = view.entries[account.accountId].expanded;
      const headerId = `${id}-${account.accountId}-header`;
      const bodyId = `${id}-${account.accountId}-body`;
      const progressId = `${id}-${account.accountId}-progress`;
      rows.push(<section key={account.accountId} data-reconciliation-account={account.accountId} aria-labelledby={headerId} className="min-w-0 overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
        <header className="flex min-w-0 flex-col gap-2 bg-muted/30 px-4 py-3">
          <div className="flex flex-wrap items-start gap-2">
            <h4 className="min-w-0 flex-1">
              <Button ref={(element) => { if (element) headers.current.set(account.accountId, element); else headers.current.delete(account.accountId); registerHeader(account.accountId, element); }}
                id={headerId} variant="ghost" disabled={locked} aria-expanded={expanded} aria-controls={bodyId} aria-describedby={progressId}
                aria-label={t(expanded ? "reconciliation.collapseAccount" : "reconciliation.expandAccount", { name: account.name, currency: account.currency })}
                onClick={() => toggle(account.accountId)} className="h-auto w-full min-w-0 justify-start gap-2 whitespace-normal px-1 py-1 text-left text-base">
                {expanded ? <ChevronDown aria-hidden="true" className="shrink-0" /> : <ChevronRight aria-hidden="true" className="shrink-0" />}
                <span className="min-w-0 break-words">{account.name} · {account.currency}</span>
              </Button>
            </h4>
            {actions?.(account)}
            <StatusBadge variant={verified ? "success" : "outline"}>{t(verified ? "reconciliation.accountVerified" : "reconciliation.accountNeedsReview")}</StatusBadge>
          </div>
          <p id={progressId} className="num break-words text-xs text-muted-foreground">{t("reconciliation.counts", account.counts as unknown as Record<string, number>)}</p>
          {notices(account)}
        </header>
        <div id={bodyId} aria-labelledby={headerId} hidden={!expanded} className="border-t border-border">{children(account)}</div>
      </section>);
    }
  }
  return <div ref={root} className="flex min-w-0 flex-col gap-3">
    <div className="flex flex-wrap gap-2">
      <Button variant="outline" size="sm" disabled={locked || ordered.every((account) => view.entries[account.accountId].expanded)} onClick={() => all(true)}>{t("reconciliation.expandAccounts")}</Button>
      <Button variant="ghost" size="sm" disabled={locked || ordered.every((account) => !view.entries[account.accountId].expanded)} onClick={() => all(false)}>{t("reconciliation.collapseAccounts")}</Button>
    </div>
    {rows}
  </div>;
}
