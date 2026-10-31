import type { ReactNode } from "react";
import { AlertCircle, CheckCircle2, Inbox, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

function Bar({ className }: { className: string }) {
  return <div className={`rounded-lg bg-muted motion-safe:animate-pulse ${className}`} />;
}

/** Skeleton loaders match the final layout: card (hero number), list (icon rows) or table (header + rows). */
export function LoadingState({ label, variant = "card" }: { label: string; variant?: "card" | "list" | "table" }) {
  return (
    <div className="flex min-h-40 flex-col gap-4 rounded-xl border border-border bg-card/60 p-6" role="status" aria-live="polite">
      <div className="flex w-full flex-col gap-3" aria-hidden="true">
        {variant === "card" && (
          <>
            <Bar className="h-3 w-1/4" />
            <Bar className="h-9 w-2/5" />
            <Bar className="h-3 w-3/5" />
          </>
        )}
        {variant === "list" &&
          [0, 1, 2, 3].map((row) => (
            <div key={row} className="flex items-center gap-3">
              <Bar className="size-10 shrink-0 rounded-xl" />
              <div className="flex flex-1 flex-col gap-2">
                <Bar className="h-3 w-2/5" />
                <Bar className="h-3 w-1/4" />
              </div>
              <Bar className="h-4 w-20" />
            </div>
          ))}
        {variant === "table" && (
          <>
            <Bar className="h-4 w-full" />
            {[0, 1, 2, 3, 4].map((row) => (
              <Bar key={row} className="h-8 w-full" />
            ))}
          </>
        )}
      </div>
      <span className="text-sm text-muted-foreground">{label}</span>
    </div>
  );
}

export function ErrorState({
  title,
  description,
  onRetry,
  retryLabel,
}: {
  title: string;
  description: string;
  onRetry?: () => void | Promise<unknown>;
  retryLabel?: string;
}) {
  return (
    <div className="flex min-h-36 flex-col items-center justify-center gap-3 rounded-xl border border-destructive/30 bg-destructive/5 px-6 py-8 text-center" role="alert">
      <AlertCircle className="size-5 text-destructive" aria-hidden="true" />
      <div className="flex flex-col gap-1">
        <h2 className="font-medium text-foreground">{title}</h2>
        <p className="max-w-md text-sm text-muted-foreground">{description}</p>
      </div>
      {onRetry && (
        <Button type="button" variant="outline" onClick={() => void onRetry()}>
          <RefreshCw className="size-4" aria-hidden="true" />
          {retryLabel}
        </Button>
      )}
    </div>
  );
}

export function EmptyState({
  title,
  description,
  action,
  variant = "empty",
  size = "default",
}: {
  title: string;
  description?: string;
  action?: ReactNode;
  variant?: "empty" | "success";
  /** `compact` sits inline inside an existing card instead of drawing its own box. */
  size?: "default" | "compact";
}) {
  const success = variant === "success";
  const icon = success
    ? <CheckCircle2 className={cn("shrink-0 text-success", size === "compact" ? "size-4" : "size-5")} aria-hidden="true" />
    : <Inbox className={cn("shrink-0 text-muted-foreground", size === "compact" ? "size-4" : "size-5")} aria-hidden="true" />;
  if (size === "compact") {
    return (
      <div className="flex items-start gap-2.5 py-1 text-sm">
        <span className="mt-0.5">{icon}</span>
        <div className="flex min-w-0 flex-col gap-0.5">
          <p className="text-foreground">{title}</p>
          {description ? <p className="text-muted-foreground">{description}</p> : null}
          {action && <div className="mt-1.5">{action}</div>}
        </div>
      </div>
    );
  }
  return (
    <div className="flex min-h-36 flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-border bg-card/40 px-6 py-8 text-center">
      <span className={cn("mb-1 flex size-10 items-center justify-center rounded-full", success ? "bg-success/12" : "bg-muted")}>{icon}</span>
      <h2 className="font-medium text-foreground">{title}</h2>
      {description ? <p className="max-w-md text-sm leading-6 text-muted-foreground">{description}</p> : null}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}
