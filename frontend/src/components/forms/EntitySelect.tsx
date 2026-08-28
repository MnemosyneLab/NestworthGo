import { useRef } from "react";
import { EntityIcon, type EntityIconKind } from "@/components/icons/EntityIcon";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

export type EntitySelectOption = { id: string; name: string; iconKey: string };

export function EntitySelect({ id, label, value, options, emptyLabel, kind, onChange }: {
  id: string;
  label: string;
  value: string;
  options: EntitySelectOption[];
  emptyLabel: string;
  kind: EntityIconKind;
  onChange: (value: string) => void;
}) {
  const detailsRef = useRef<HTMLDetailsElement>(null);
  const selected = options.find((option) => option.id === value);
  const choose = (next: string) => {
    onChange(next);
    if (detailsRef.current) detailsRef.current.open = false;
  };

  return <div className="flex flex-col gap-1.5">
    <Label htmlFor={id}>{label}</Label>
    <details ref={detailsRef} className="relative rounded-lg border border-border bg-card shadow-xs">
      <summary id={id} className="flex h-9 cursor-pointer list-none items-center gap-2 px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50">
        {selected && <EntityIcon iconKey={selected.iconKey} kind={kind} />}
        <span>{selected?.name ?? emptyLabel}</span>
      </summary>
      <div role="listbox" aria-label={label} className="absolute z-50 mt-1 max-h-60 w-full overflow-y-auto rounded-lg border border-border bg-card p-1 shadow-lg">
        <button type="button" role="option" aria-selected={!value} onClick={() => choose("")} className={cn("flex w-full items-center rounded-md px-3 py-2 text-left text-sm hover:bg-muted", !value && "bg-muted")}>{emptyLabel}</button>
        {options.map((option) => <button key={option.id} type="button" role="option" aria-selected={value === option.id} onClick={() => choose(option.id)} className={cn("flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm hover:bg-muted", value === option.id && "bg-muted")}>
          <EntityIcon iconKey={option.iconKey} kind={kind} />
          <span>{option.name}</span>
        </button>)}
      </div>
    </details>
  </div>;
}
