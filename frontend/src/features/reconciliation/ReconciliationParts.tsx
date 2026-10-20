import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Notice } from "@/components/ui/notice";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ReconciliationSession } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";

type BadgeVariant = "default" | "secondary" | "success" | "warning" | "outline";

export { Notice };

export function sessionStatusVariant(status: string): BadgeVariant {
  if (status === "completed") return "success";
  if (status === "paused") return "warning";
  return "default";
}

export function outcomeVariant(outcome: string): BadgeVariant {
  if (outcome === "confirmed") return "success";
  if (outcome === "unresolved") return "warning";
  if (outcome === "pending") return "outline";
  return "secondary";
}

export function StatusBadge({ variant, children }: { variant: BadgeVariant; children: ReactNode }) {
  return (
    <Badge variant={variant} className="shrink-0 whitespace-nowrap">
      {children}
    </Badge>
  );
}

const segments = [
  { key: "confirmed", className: "bg-success" },
  { key: "skipped", className: "bg-muted-foreground/45" },
  { key: "unresolved", className: "bg-warning" },
] as const;

/** Progress strip plus the localized count sentence; the strip is decorative. */
export function Counts({ counts }: { counts: ReconciliationSession["counts"] }) {
  const { t } = useTranslation();
  const total = counts.confirmed + counts.pending + counts.skipped + counts.unresolved;
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      {total > 0 && (
        <div className="flex h-1.5 w-full overflow-hidden rounded-full bg-muted" aria-hidden="true">
          {segments.map(({ key, className }) =>
            counts[key] > 0 ? (
              <span key={key} className={className} style={{ width: `${(counts[key] / total) * 100}%` }} />
            ) : null,
          )}
        </div>
      )}
      <p role="status" className="num text-xs text-muted-foreground">
        {t("reconciliation.counts", counts as unknown as Record<string, number>)}
      </p>
    </div>
  );
}

/** A labelled form row: keeps label, control and its error message together. */
export function FormField({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn("flex flex-col gap-1.5", className)}>{children}</div>;
}

export const PAGE_SIZE = 25;

/** Previous/Next controls; hidden while everything fits on one page. */
export function Pager({ page, total, onPage }: { page: number; total: number; onPage: (page: number) => void }) {
  const { t } = useTranslation();
  if (total <= PAGE_SIZE) return null;
  return (
    <div className="flex items-center justify-end gap-2 pt-1">
      <Button variant="outline" size="sm" disabled={page === 0} onClick={() => onPage(page - 1)}>
        {t("reconciliation.previous")}
      </Button>
      <Button variant="outline" size="sm" disabled={(page + 1) * PAGE_SIZE >= total} onClick={() => onPage(page + 1)}>
        {t("reconciliation.next")}
      </Button>
    </div>
  );
}
