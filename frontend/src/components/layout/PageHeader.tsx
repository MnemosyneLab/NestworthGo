import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export function PageIntro({ description, status }: { description?: ReactNode; status?: ReactNode }) {
  if (!description && !status) {
    return null;
  }

  return (
    <div className="flex flex-col gap-3 border-b border-border pb-4 sm:flex-row sm:items-center sm:justify-between">
      {description && <p className="max-w-2xl text-sm leading-6 text-muted-foreground">{description}</p>}
      {status && <div className="flex min-w-0 flex-wrap items-center gap-2">{status}</div>}
    </div>
  );
}

export function PageHeader({
  title,
  description,
  actions,
  status,
}: {
  title: ReactNode;
  description?: string;
  actions?: ReactNode;
  status?: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-4 border-b border-border pb-5 sm:flex-row sm:items-start sm:justify-between">
      <div className="min-w-0">
        <h1 className="text-balance text-2xl font-semibold tracking-tight text-foreground">{title}</h1>
        {description && <p className="mt-1 max-w-2xl text-sm leading-6 text-muted-foreground">{description}</p>}
        {status && <div className="mt-3">{status}</div>}
      </div>
      {actions && <div className={cn("flex shrink-0 flex-wrap items-center gap-2", "sm:pt-1")}>{actions}</div>}
    </div>
  );
}
