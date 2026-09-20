import type { ReactNode } from "react";

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
