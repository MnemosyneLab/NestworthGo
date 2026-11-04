import { Menu } from "@base-ui/react/menu";
import { Ellipsis, type LucideIcon } from "lucide-react";
import { useState } from "react";
import { Button } from "./button";

export type ActionMenuItem = { id: string; label: string; onSelect: () => void; disabled?: boolean };

export function ActionMenu({ label, items, sections = [], icon: Icon = Ellipsis, disabled = false }: { label: string; items: ActionMenuItem[]; sections?: Array<{ label: string; items: ActionMenuItem[] }>; icon?: LucideIcon; disabled?: boolean }) {
  const [open, setOpen] = useState(false);
  if (!items.length && !sections.some(section => section.items.length) && !disabled) return null;
  const renderItem = (item: ActionMenuItem) => <Menu.Item key={item.id} disabled={item.disabled} className="break-words cursor-default rounded-lg px-3 py-2 text-sm outline-none data-[highlighted]:bg-primary/10 data-[disabled]:opacity-50" onClick={() => {
    setOpen(false);
    item.onSelect();
  }}>{item.label}</Menu.Item>;
  return (
    <Menu.Root open={open} onOpenChange={setOpen}>
      <Menu.Trigger disabled={disabled} render={<Button variant="ghost" size="icon" aria-label={label} title={label} />}>
        <Icon aria-hidden="true" />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Positioner sideOffset={4} align="end" className="z-50">
          <Menu.Popup className="max-h-[min(28rem,70dvh)] max-w-[min(24rem,calc(100vw-2rem))] overflow-y-auto min-w-44 rounded-xl border border-border bg-card p-1 text-foreground shadow-lg outline-none">
            {items.map(renderItem)}
            {sections.filter(section => section.items.length).map(section => <Menu.Group key={section.label} className="mt-1 border-t border-border pt-1">
              <Menu.GroupLabel className="px-3 py-2 text-xs font-medium text-muted-foreground">{section.label}</Menu.GroupLabel>
              {section.items.map(renderItem)}
            </Menu.Group>)}
          </Menu.Popup>
        </Menu.Positioner>
      </Menu.Portal>
    </Menu.Root>
  );
}
