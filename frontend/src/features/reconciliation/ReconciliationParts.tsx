import type { ReactNode } from "react";
import { AlertTriangle, CircleAlert, Info } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ReconciliationSession } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";

type NoticeTone = "info" | "warning" | "danger";

const noticeStyle: Record<NoticeTone, string> = {
  info: "border-border bg-muted/50 text-muted-foreground",
  warning: "border-warning/40 bg-warning/10 text-warning-foreground",
  danger: "border-destructive/30 bg-destructive/5 text-destructive",
};
const noticeIcon = { info: Info, warning: AlertTriangle, danger: CircleAlert } as const;

/** Inline message block. Pass `role` when the text must be announced (status/alert). */
export function Notice({
  tone = "info",
  role,
  children,
  className,
}: {
  tone?: NoticeTone;
  role?: "status" | "alert";
  children: ReactNode;
  className?: string;
}) {
  const Icon = noticeIcon[tone];
  return (
    <div
      role={role}
      className={cn("flex items-start gap-2 rounded-xl border px-3 py-2 text-sm leading-6", noticeStyle[tone], className)}
    >
      <Icon className="mt-1 size-4 shrink-0" aria-hidden="true" />
      <div className="min-w-0 flex-1 break-words">{children}</div>
    </div>
  );
}

type BadgeVariant = "default" | "secondary" | "success" | "warning" | "outline";

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
