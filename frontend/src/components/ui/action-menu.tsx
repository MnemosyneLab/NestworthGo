import { Menu } from "@base-ui/react/menu";
import { Ellipsis } from "lucide-react";
import { useState } from "react";
import { Button } from "./button";

export type ActionMenuItem = { id: string; label: string; onSelect: () => void };

export function ActionMenu({ label, items }: { label: string; items: ActionMenuItem[] }) {
  const [open, setOpen] = useState(false);
  if (!items.length) return null;
  return (
    <Menu.Root open={open} onOpenChange={setOpen}>
      <Menu.Trigger render={<Button variant="ghost" size="icon" aria-label={label} />}>
        <Ellipsis aria-hidden="true" />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Positioner sideOffset={4} align="end" className="z-50">
          <Menu.Popup className="min-w-44 rounded-lg border border-border bg-card p-1 text-foreground shadow-lg outline-none">
            {items.map((item) => (
              <Menu.Item key={item.id} className="cursor-default rounded-md px-3 py-2 text-sm outline-none data-[highlighted]:bg-muted" onClick={() => {
                setOpen(false);
                item.onSelect();
              }}>{item.label}</Menu.Item>
            ))}
          </Menu.Popup>
        </Menu.Positioner>
      </Menu.Portal>
    </Menu.Root>
  );
}
