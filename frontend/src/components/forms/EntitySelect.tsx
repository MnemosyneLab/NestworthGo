import { useRef } from "react";
import { EntityIcon, type EntityIconKind } from "@/components/icons/EntityIcon";
import { Label } from "@/components/ui/label";
import { selectFieldClass } from "@/components/ui/select";
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
    const details = detailsRef.current;
    if (details) {
      details.open = false;
      details.querySelector("summary")?.focus();
    }
  };

  return <div className="flex flex-col gap-1.5">
    <Label htmlFor={id}>{label}</Label>
    <details ref={detailsRef} className="relative min-w-0" onKeyDown={event => {
      if (event.key !== "Escape" || !event.currentTarget.open) return;
      event.preventDefault();
      event.stopPropagation();
      event.currentTarget.open = false;
      event.currentTarget.querySelector("summary")?.focus();
    }}>
      <summary id={id} aria-label={label} className={cn(selectFieldClass, "flex cursor-pointer list-none items-center gap-2 [&::-webkit-details-marker]:hidden")}>
        {selected && <EntityIcon iconKey={selected.iconKey} kind={kind} />}
        <span className="min-w-0 flex-1 truncate">{selected?.name ?? emptyLabel}</span>
        <span aria-hidden="true" className="select-chevron" />
      </summary>
      <div role="listbox" aria-label={label} className="select-panel absolute z-50 mt-1 w-full">
        <button type="button" role="option" aria-selected={!value} onClick={() => choose("")} className="select-option w-full text-start">
          <span aria-hidden="true" className={cn("size-3.5 shrink-0", !value && "select-check")} />
          <span className="min-w-0">{emptyLabel}</span>
        </button>
        {options.map((option) => <button key={option.id} type="button" role="option" aria-selected={value === option.id} onClick={() => choose(option.id)} className="select-option w-full text-start">
          <span aria-hidden="true" className={cn("size-3.5 shrink-0", value === option.id && "select-check")} />
          <EntityIcon iconKey={option.iconKey} kind={kind} />
          <span className="min-w-0">{option.name}</span>
        </button>)}
      </div>
    </details>
  </div>;
}
