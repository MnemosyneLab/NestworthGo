import type { ReactNode } from "react";
import { AlertCircle, Inbox, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";

export function LoadingState({ label }: { label: string }) {
  return (
    <div className="flex min-h-40 flex-col items-center justify-center gap-4 rounded-xl border border-border bg-card/60 p-8 text-center" role="status" aria-live="polite">
      <div className="flex w-full max-w-sm flex-col gap-3" aria-hidden="true">
        <div className="h-3 w-2/5 rounded bg-muted motion-safe:animate-pulse" />
        <div className="h-8 w-4/5 rounded bg-muted motion-safe:animate-pulse" />
        <div className="h-3 w-3/5 rounded bg-muted motion-safe:animate-pulse" />
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
    <div className="flex min-h-40 flex-col items-center justify-center gap-3 rounded-xl border border-destructive/30 bg-destructive/5 p-8 text-center" role="alert">
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
}: {
  title: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex min-h-40 flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-border bg-card/40 p-8 text-center">
      <Inbox className="size-5 text-muted-foreground" aria-hidden="true" />
      <h2 className="font-medium text-foreground">{title}</h2>
      {description ? <p className="max-w-md text-sm leading-6 text-muted-foreground">{description}</p> : null}
      {action && <div className="mt-2">{action}</div>}
    </div>
  );
}
