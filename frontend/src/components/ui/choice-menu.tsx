import { useBusinessPortal } from "@/lib/businessCapture";
import { Menu } from "@base-ui/react/menu";
import { Check, ChevronDown, type LucideIcon } from "lucide-react";
import { useState } from "react";
import { cn } from "@/lib/utils";

export type ChoiceMenuOption = { value: string; label: string; description?: string; disabled?: boolean };

/** Compact pill chooser for toolbars and composers with a short option set
 * (model, effort, permission). Forms keep NativeSelect/FilterableSelect. The
 * selected option is exposed as a menu radio item, never by color alone. */
export function ChoiceMenu({ label, value, options, onChange, icon: Icon, tone = "default", disabled = false, className, align = "start" }: {
  /** Localized name of the setting; the selected option is appended for assistive technology. */
  label: string;
  value: string;
  options: readonly ChoiceMenuOption[];
  onChange: (value: string) => void;
  icon?: LucideIcon;
  tone?: "default" | "warning";
  disabled?: boolean;
  className?: string;
  align?: "start" | "end";
}) {
  const [open, setOpen] = useState(false);
  // Forget an open menu immediately when its owner blocks interaction.
  if (disabled && open) setOpen(false);
  const capture = useBusinessPortal();
  const current = options.find(option => option.value === value);
  const text = current?.label ?? value;
  return (
    <Menu.Root open={open && !disabled} onOpenChange={next => setOpen(next && !disabled)}>
      <Menu.Trigger
        disabled={disabled}
        aria-label={`${label}: ${text}`}
        title={current?.description ? `${label}: ${text} · ${current.description}` : `${label}: ${text}`}
        className={cn(
          "inline-flex min-h-11 min-w-0 max-w-full items-center gap-1.5 rounded-full border px-3 text-xs font-medium transition-[color,background-color,border-color,box-shadow] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 disabled:cursor-not-allowed disabled:opacity-50 xl:min-h-8",
          tone === "warning"
            ? "border-warning/40 bg-warning/15 text-warning-foreground hover:bg-warning/25"
            : "border-border bg-card text-foreground hover:border-foreground/20 hover:bg-muted/60",
          className,
        )}
      >
        {Icon && <Icon aria-hidden="true" className="size-3.5 shrink-0" />}
        <span className="min-w-0 truncate">{text}</span>
        <ChevronDown aria-hidden="true" className="size-3 shrink-0 opacity-60" />
      </Menu.Trigger>
      {open && capture.origin}<Menu.Portal>
        <Menu.Positioner side="top" sideOffset={6} align={align} className="z-50">
          <Menu.Popup {...capture.props} aria-label={label} className="max-h-[min(24rem,70dvh)] min-w-48 max-w-[min(22rem,calc(100vw-2rem))] overflow-y-auto rounded-xl border border-border bg-card p-1 text-foreground shadow-lg outline-none">
            <Menu.RadioGroup value={value} onValueChange={next => { if (!disabled && typeof next === "string" && next !== value) onChange(next); }}>
              {options.map(option => (
                <Menu.RadioItem key={option.value} value={option.value} disabled={disabled || option.disabled} closeOnClick
                  className="flex cursor-default items-start gap-2 rounded-lg px-3 py-2 text-sm outline-none data-[highlighted]:bg-primary/10 data-[disabled]:opacity-50">
                  <span className="mt-0.5 flex size-4 shrink-0 items-center justify-center">
                    <Menu.RadioItemIndicator keepMounted={false}><Check aria-hidden="true" className="size-4 text-primary" /></Menu.RadioItemIndicator>
                  </span>
                  <span className="min-w-0 flex-1 break-words">
                    <span className="block">{option.label}</span>
                    {option.description && <span className="mt-0.5 block text-xs text-muted-foreground">{option.description}</span>}
                  </span>
                </Menu.RadioItem>
              ))}
            </Menu.RadioGroup>
          </Menu.Popup>
        </Menu.Positioner>
      </Menu.Portal>
    </Menu.Root>
  );
}
