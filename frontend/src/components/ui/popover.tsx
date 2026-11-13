import { useBusinessPortal } from "@/lib/businessCapture";
import * as React from "react";
import { Popover as BasePopover } from "@base-ui/react/popover";
import { cn } from "@/lib/utils";

const Popover = BasePopover.Root;
const PopoverTrigger = BasePopover.Trigger;

function PopoverContent({
  className,
  side = "bottom",
  align = "start",
  sideOffset = 4,
  children,
  ...props
}: React.ComponentProps<typeof BasePopover.Popup> &
  Pick<React.ComponentProps<typeof BasePopover.Positioner>, "side" | "align" | "sideOffset">) {
  const capture = useBusinessPortal();
  return (
    <>{capture.origin}<BasePopover.Portal>
      <BasePopover.Positioner side={side} align={align} sideOffset={sideOffset} className="z-50">
        <BasePopover.Popup
          {...capture.props}
          className={cn(
            "z-50 origin-[var(--transform-origin)] rounded-xl border border-border bg-card p-2 text-foreground shadow-lg outline-none",
            "data-[starting-style]:scale-95 data-[starting-style]:opacity-0 data-[ending-style]:scale-95 data-[ending-style]:opacity-0",
            className,
          )}
          {...props}
        >
          {children}
        </BasePopover.Popup>
      </BasePopover.Positioner>
    </BasePopover.Portal></>
  );
}

/** Anchored content whose editable trigger owns focus and keyboard navigation. */
function PopoverList({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  const capture = useBusinessPortal();
  return (
    <>{capture.origin}<BasePopover.Portal>
      <BasePopover.Positioner side="bottom" align="start" sideOffset={4} className="z-50 data-[anchor-hidden]:invisible">
        <div
          {...capture.props}
          {...props}
          role="listbox"
          className="select-panel w-max min-w-[min(var(--available-width),max(11rem,var(--anchor-width)))] max-w-[min(var(--available-width),max(24rem,var(--anchor-width)))] [--select-available-height:var(--available-height)]"
        >
          {children}
        </div>
      </BasePopover.Positioner>
    </BasePopover.Portal></>
  );
}

export { Popover, PopoverTrigger, PopoverContent, PopoverList };
