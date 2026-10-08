import { useRef, type FocusEvent, type ReactNode } from "react";
import { tabbable } from "tabbable";

/** Keep sequential keyboard navigation inside the desktop workspace. */
export function AppFocusBoundary({ children, className }: { children: ReactNode; className?: string }) {
  const root = useRef<HTMLDivElement>(null);

  const enter = (edge: "start" | "end", event: FocusEvent<HTMLSpanElement>) => {
    const container = root.current;
    // Portal dialogs own their focus scope while the workspace is inert.
    if (!container || container.closest('[inert], [aria-hidden="true"]')) return;
    const stops = tabbable(container).filter((element) => !element.hasAttribute("data-app-focus-guard"));
    const fromInside = event.relatedTarget instanceof Node && container.contains(event.relatedTarget);
    // A boundary reached from inside wraps; entry from the native host starts
    // at the nearest real control. No key/blur handlers compete with widgets,
    // native menus, file pickers, or Portal focus management.
    const useLast = edge === "start" ? fromInside : !fromInside;
    (useLast ? stops[stops.length - 1] : stops[0])?.focus();
  };

  return <div ref={root} className={className}>
    <span data-app-focus-guard="start" tabIndex={0} aria-hidden="true" className="pointer-events-none fixed size-px overflow-hidden opacity-0" onFocus={(event) => enter("start", event)} />
    {children}
    <span data-app-focus-guard="end" tabIndex={0} aria-hidden="true" className="pointer-events-none fixed size-px overflow-hidden opacity-0" onFocus={(event) => enter("end", event)} />
  </div>;
}
