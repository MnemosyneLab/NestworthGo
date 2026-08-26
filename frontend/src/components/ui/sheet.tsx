import * as React from "react";
import { Dialog as BaseDialog } from "@base-ui/react/dialog";
import { X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";

// Sheet is a side panel built on the same Dialog primitive as
// components/ui/dialog.tsx (Base UI does not ship a lightweight desktop
// side-panel primitive separate from its gesture-driven mobile Drawer), so
// it shares Root/Trigger/Close and only changes the popup's position and
// enter/exit animation.
const Sheet = BaseDialog.Root;
const SheetTrigger = BaseDialog.Trigger;
const SheetClose = BaseDialog.Close;

type Side = "right" | "left";

const sideClasses: Record<Side, string> = {
  right:
    "right-0 top-0 h-full w-full max-w-md border-l data-[starting-style]:translate-x-full data-[ending-style]:translate-x-full",
  left: "left-0 top-0 h-full w-full max-w-md border-r data-[starting-style]:-translate-x-full data-[ending-style]:-translate-x-full",
};

function SheetContent({
  className,
  children,
  side = "right",
  ...props
}: React.ComponentProps<typeof BaseDialog.Popup> & { side?: Side }) {
  const { t } = useTranslation();

  return (
    <BaseDialog.Portal>
      <BaseDialog.Backdrop className="fixed inset-0 z-50 bg-black/40 transition-opacity data-[starting-style]:opacity-0 data-[ending-style]:opacity-0" />
      <BaseDialog.Popup
        className={cn(
          "fixed z-50 flex flex-col gap-4 border-border bg-card p-6 shadow-lg outline-none transition-transform duration-200",
          sideClasses[side],
          className,
        )}
        {...props}
      >
        {children}
        <BaseDialog.Close className="absolute right-4 top-4 rounded-sm text-muted-foreground opacity-70 transition-opacity hover:opacity-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50">
          <X className="size-4" />
          <span className="sr-only">{t("common.close")}</span>
        </BaseDialog.Close>
      </BaseDialog.Popup>
    </BaseDialog.Portal>
  );
}

function SheetHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("flex flex-col gap-1.5", className)} {...props} />;
}

function SheetFooter({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("mt-auto flex flex-row justify-end gap-2 pt-4", className)} {...props} />;
}

function SheetTitle({ className, ...props }: React.ComponentProps<typeof BaseDialog.Title>) {
  return <BaseDialog.Title className={cn("text-lg font-semibold text-foreground", className)} {...props} />;
}

function SheetDescription({ className, ...props }: React.ComponentProps<typeof BaseDialog.Description>) {
  return <BaseDialog.Description className={cn("text-sm text-muted-foreground", className)} {...props} />;
}

export { Sheet, SheetTrigger, SheetClose, SheetContent, SheetHeader, SheetFooter, SheetTitle, SheetDescription };
