import { createContext, useContext, type ReactNode } from "react";
import { createPortal } from "react-dom";

interface PageChromeContextValue {
  activePageId: string;
  target: HTMLDivElement | null;
}

const PageChromeContext = createContext<PageChromeContextValue | null>(null);

export function PageChromeProvider({
  activePageId,
  target,
  children,
}: {
  activePageId: string;
  target: HTMLDivElement | null;
  children: ReactNode;
}) {
  return <PageChromeContext.Provider value={{ activePageId, target }}>{children}</PageChromeContext.Provider>;
}

export function PageChrome({
  pageId,
  title,
  actions,
  enabled = true,
}: {
  pageId: string;
  title?: ReactNode;
  actions?: ReactNode;
  enabled?: boolean;
}) {
  const context = useContext(PageChromeContext);
  const content = (
    <>
      {title !== undefined && <h1 className="min-w-0 truncate text-base font-semibold tracking-tight text-foreground">{title}</h1>}
      {actions && <div className="flex min-w-0 flex-wrap items-center gap-2">{actions}</div>}
    </>
  );

  if (!enabled) {
    return null;
  }

  // Pages are also tested in isolation without AppShell. Keep the same
  // semantic heading and action order in that environment.
  if (!context) {
    return <div className="flex min-w-0 flex-wrap items-center gap-3">{content}</div>;
  }
  if (context.activePageId !== pageId) {
    return null;
  }
  if (!context.target) {
    return <div className="flex min-w-0 flex-wrap items-center gap-3">{content}</div>;
  }
  return createPortal(content, context.target);
}
