import * as React from "react";
import { AlertDialog as BaseAlertDialog } from "@base-ui/react/alert-dialog";
import { cn } from "@/lib/utils";
import { buttonVariants } from "@/components/ui/button";

// AlertDialog is the confirmation pattern for high-risk actions (Undo, Fix,
// Archive, Restore): show explicit impact and a toast after execution.
const AlertDialog = BaseAlertDialog.Root;
const AlertDialogTrigger = BaseAlertDialog.Trigger;

function AlertDialogContent({ className, children, ...props }: React.ComponentProps<typeof BaseAlertDialog.Popup>) {
  return (
    <BaseAlertDialog.Portal>
      <BaseAlertDialog.Backdrop className="fixed inset-0 z-50 bg-black/40 transition-opacity data-[starting-style]:opacity-0 data-[ending-style]:opacity-0" />
      <BaseAlertDialog.Popup
        className={cn(
          "fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-lg border border-border bg-card p-6 shadow-lg outline-none transition-all data-[starting-style]:scale-95 data-[starting-style]:opacity-0 data-[ending-style]:scale-95 data-[ending-style]:opacity-0",
          className,
        )}
        {...props}
      >
        {children}
      </BaseAlertDialog.Popup>
    </BaseAlertDialog.Portal>
  );
}

function AlertDialogHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("mb-4 flex flex-col gap-1.5", className)} {...props} />;
}

function AlertDialogFooter({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("mt-6 flex flex-row justify-end gap-2", className)} {...props} />;
}

function AlertDialogTitle({ className, ...props }: React.ComponentProps<typeof BaseAlertDialog.Title>) {
  return <BaseAlertDialog.Title className={cn("text-lg font-semibold text-foreground", className)} {...props} />;
}

function AlertDialogDescription({ className, ...props }: React.ComponentProps<typeof BaseAlertDialog.Description>) {
  return <BaseAlertDialog.Description className={cn("text-sm text-muted-foreground", className)} {...props} />;
}

function AlertDialogCancel({ className, ...props }: React.ComponentProps<typeof BaseAlertDialog.Close>) {
  return <BaseAlertDialog.Close className={cn(buttonVariants({ variant: "outline" }), className)} {...props} />;
}

function AlertDialogAction({ className, ...props }: React.ComponentProps<typeof BaseAlertDialog.Close>) {
  return <BaseAlertDialog.Close className={cn(buttonVariants({ variant: "destructive" }), className)} {...props} />;
}

export {
  AlertDialog,
  AlertDialogTrigger,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogFooter,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogCancel,
  AlertDialogAction,
};
