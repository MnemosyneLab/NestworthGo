import type { ReactNode } from "react";
import { AlertTriangle, CircleAlert, Info } from "lucide-react";
import { cn } from "@/lib/utils";

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
