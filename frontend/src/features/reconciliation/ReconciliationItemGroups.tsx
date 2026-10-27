import { useId, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import type { ReconciliationItem } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";

function group(item: ReconciliationItem): string {
  return item.outcome === "confirmed" && !item.recheckRequired ? "confirmed" : item.outcome === "skipped" && !item.recheckRequired ? "skipped" : "pending";
}
export function ReconciliationItemGroups({ items, locked, children }: { items: ReconciliationItem[]; locked: boolean; children: (item: ReconciliationItem) => ReactNode }) {
  const { t } = useTranslation();
  const id = useId();
  const pending = useRef<HTMLHeadingElement>(null);
  const [expanded, setExpanded] = useState(false);
  const [groups, setGroups] = useState(() => Object.fromEntries(items.map(item => [item.id, group(item)])));
  useLayoutEffect(() => {
    if (locked) return;
    const next = Object.fromEntries(items.map(item => [item.id, group(item)]));
    if (Object.keys(next).length === Object.keys(groups).length && items.every(item => next[item.id] === groups[item.id])) return;
    const active = document.activeElement instanceof HTMLElement ? document.activeElement.closest("[data-reconciliation-item]") : null;
    const moved = active?.getAttribute("data-reconciliation-item");
    setGroups(next);
    if (moved && next[moved] === "confirmed" && groups[moved] !== "confirmed") {
      setExpanded(false);
      pending.current?.focus({ preventScroll: true });
    }
  }, [items, locked, groups]);
  // Each group preserves the original checkpoint order and stable item keys.
  const rows: ReactNode[] = [];
  for (const status of ["pending", "skipped", "confirmed"]) {
    const members = items.filter(item => (groups[item.id] ?? group(item)) === status);
    if (!members.length) continue;
    rows.push(status === "confirmed"
      ? <h5 key={status} className="px-5 py-2"><Button id={`${id}-toggle`} aria-expanded={expanded} aria-controls={`${id}-confirmed`} variant="ghost" size="sm" disabled={locked} onClick={() => setExpanded(value => !value)}>{t("reconciliation.confirmedItems", { count: members.length })}</Button></h5>
      : <h5 key={status} ref={status === "pending" ? pending : undefined} tabIndex={-1} className="px-5 py-3 text-sm font-semibold">{t(status === "skipped" ? "reconciliation.skippedItems" : "reconciliation.pendingItems", { count: members.length })}</h5>);
    // The hidden wrapper is separate from the toggle; warning/unfinished items always remain visible.
    rows.push(<div key={`items-${status}`} id={status === "confirmed" ? `${id}-confirmed` : undefined} hidden={status === "confirmed" && !expanded} aria-labelledby={status === "confirmed" ? `${id}-toggle` : undefined}>{members.map(item => <div key={item.id} data-reconciliation-item={item.id}>{children(item)}</div>)}</div>);
  }
  return <div className="divide-y divide-border">{rows}</div>;
}
