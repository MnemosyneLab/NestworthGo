import * as React from "react";
import { Dialog as BaseDialog } from "@base-ui/react/dialog";
import { X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";

// Sheet is a side panel built on Base UI's Dialog primitive (Base UI does
// not ship a lightweight desktop side-panel primitive separate from its
// gesture-driven mobile Drawer). It shares Root/Trigger/Close and only
// changes the popup's position and enter/exit animation.
const Sheet = BaseDialog.Root;
const SheetTrigger = BaseDialog.Trigger;
const SheetCloseDisabledContext = React.createContext(false);

type Side = "right" | "left";
type Size = "md" | "lg" | "xl";

const sideClasses: Record<Side, string> = {
  right:
    "right-0 top-0 h-full w-full border-l rounded-l-2xl data-[starting-style]:translate-x-full data-[ending-style]:translate-x-full",
  left: "left-0 top-0 h-full w-full border-r rounded-r-2xl data-[starting-style]:-translate-x-full data-[ending-style]:-translate-x-full",
};

const sizeClasses: Record<Size, string> = {
  md: "max-w-md",
  lg: "max-w-xl",
  xl: "max-w-3xl",
};

// Native WebView magnification can shrink and pan the visual viewport without
// resizing the layout viewport. Anchor sheets to the visible bounds so their
// right edge (including Close) doesn't end up outside the magnified window.
// Keep the user's scale: these are CSS-pixel bounds, not an inverse zoom.
function SheetViewport({ children }: { children: React.ReactNode }) {
  const ref = React.useRef<HTMLDivElement>(null);
  React.useLayoutEffect(() => {
    const element = ref.current;
    const viewport = element?.ownerDocument.defaultView?.visualViewport;
    if (!element || !viewport) return;
    const update = () => {
      element.style.left = `${viewport.offsetLeft}px`;
      element.style.top = `${viewport.offsetTop}px`;
      element.style.right = "auto";
      element.style.bottom = "auto";
      element.style.width = `${viewport.width}px`;
      element.style.height = `${viewport.height}px`;
    };
    update();
    viewport.addEventListener("resize", update);
    viewport.addEventListener("scroll", update);
    return () => {
      viewport.removeEventListener("resize", update);
      viewport.removeEventListener("scroll", update);
    };
  }, []);
  return <div ref={ref} className="pointer-events-none fixed inset-0 z-50 overflow-hidden">{children}</div>;
}

function SheetContent({
  className,
  children,
  side = "right",
  size = "md",
  closeDisabled = false,
  ...props
}: React.ComponentProps<typeof BaseDialog.Popup> & { side?: Side; size?: Size; closeDisabled?: boolean }) {
  return (
    <BaseDialog.Portal>
      <BaseDialog.Backdrop className="fixed inset-0 z-50 bg-foreground/35 backdrop-blur-[2px] transition-opacity data-[starting-style]:opacity-0 data-[ending-style]:opacity-0" />
      <SheetViewport>
        <BaseDialog.Popup
          className={cn(
            "pointer-events-auto absolute z-50 flex scroll-pb-24 scroll-pt-24 flex-col gap-4 overflow-y-auto overscroll-contain border-border bg-card p-6 shadow-lg outline-none transition-transform duration-200 ease-out",
            sideClasses[side],
            sizeClasses[size],
            className,
          )}
          {...props}
        >
          <SheetCloseDisabledContext.Provider value={closeDisabled}>
            {children}
          </SheetCloseDisabledContext.Provider>
        </BaseDialog.Popup>
      </SheetViewport>
    </BaseDialog.Portal>
  );
}

/** Header stays pinned while long sheet bodies scroll underneath. The negative
 * sticky offset cancels the popup's p-6, which sticky positioning otherwise
 * insets by, pushing the header over the first field. Keep Close in the same
 * painted, non-shrinking header, in normal flow: a separately positioned child
 * of the scrolling popup can disappear under native WebView magnification.
 * Every sheet must include a SheetHeader with its accessible title. */
function SheetHeader({ className, children, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  const { t } = useTranslation();
  const closeDisabled = React.useContext(SheetCloseDisabledContext);
  return <div className={cn("sticky -top-6 z-10 -mx-6 -mt-6 flex shrink-0 items-start gap-3 bg-card px-6 pb-3 pt-6", className)} {...props}>
    <div className="flex min-w-0 flex-1 flex-col gap-1.5">{children}</div>
    <BaseDialog.Close disabled={closeDisabled} className="inline-flex min-h-8 shrink-0 items-center justify-center gap-1.5 rounded-lg border border-border bg-card px-2 py-1 text-sm text-foreground transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 disabled:opacity-50">
      <X className="size-4" />
      <span>{t("common.close")}</span>
    </BaseDialog.Close>
  </div>;
}

/** Footer actions stay reachable at the bottom of long forms. */
function SheetFooter({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("sticky -bottom-6 z-10 -mx-6 -mb-6 mt-auto flex flex-row justify-end gap-2 border-t border-border bg-card px-6 pb-4 pt-4", className)} {...props} />;
}

function SheetTitle({ className, ...props }: React.ComponentProps<typeof BaseDialog.Title>) {
  return <BaseDialog.Title className={cn("text-lg font-semibold text-foreground", className)} {...props} />;
}

function SheetDescription({ className, ...props }: React.ComponentProps<typeof BaseDialog.Description>) {
  return <BaseDialog.Description className={cn("text-sm text-muted-foreground", className)} {...props} />;
}

export { Sheet, SheetTrigger, SheetContent, SheetHeader, SheetFooter, SheetTitle, SheetDescription };
