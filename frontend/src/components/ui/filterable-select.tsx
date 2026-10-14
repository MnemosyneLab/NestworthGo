import * as React from "react";
import { ChevronDown } from "lucide-react";
import { selectFieldClass } from "./select";
import { cn } from "@/lib/utils";

export type FilterableSelectOption = {
  value: string;
  label: string;
  /** Additional searchable identity text; never rendered in the option. */
  searchKeywords?: string;
  disabled?: boolean;
};

export type FilterableSelectProps = {
  id?: string;
  form?: string;
  value: string;
  options: FilterableSelectOption[];
  onValueChange: (value: string) => void;
  placeholder?: string;
  noOptionsLabel?: string;
  disabled?: boolean;
  className?: string;
  "aria-label"?: string;
  "aria-describedby"?: string;
  "aria-invalid"?: React.AriaAttributes["aria-invalid"];
};

/** A small keyboard-accessible combobox for long closed catalogs such as IANA timezones. */
export function FilterableSelect({
  id,
  form,
  value,
  options,
  onValueChange,
  placeholder,
  noOptionsLabel = "No matches",
  disabled = false,
  className,
  "aria-label": ariaLabel,
  "aria-describedby": describedBy,
  "aria-invalid": invalid,
}: FilterableSelectProps) {
  const rootRef = React.useRef<HTMLDivElement>(null);
  const generatedId = React.useId();
  const inputId = id ?? generatedId;
  const listboxId = `${inputId}-options`;
  const selected = options.find((option) => option.value === value);
  const [query, setQuery] = React.useState("");
  const [open, setOpen] = React.useState(false);
  const [activeValue, setActiveValue] = React.useState<string | undefined>();
  const composing = React.useRef(false);
  const isOpen = open && !disabled;
  if (disabled && open) setOpen(false);
  const beginSearch = () => {
    if (disabled) return;
    setQuery("");
    setActiveValue(options.find(option => option.value === value && !option.disabled)?.value ?? options.find(option => !option.disabled)?.value);
    setOpen(true);
  };
  const close = () => { setOpen(false); setQuery(""); };


  React.useEffect(() => {
    const handlePointerDown = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("pointerdown", handlePointerDown);
    return () => document.removeEventListener("pointerdown", handlePointerDown);
  }, []);

  const filtered = options.filter((option) => {
    const normalizedQuery = query.trim().toLocaleLowerCase();
    return !normalizedQuery || `${option.label} ${option.value} ${option.searchKeywords ?? ""}`.toLocaleLowerCase().includes(normalizedQuery);
  });

  const choose = (option: FilterableSelectOption | undefined) => {
    if (!option || option.disabled) {
      return;
    }
    onValueChange(option.value);
    setQuery("");
    setOpen(false);
  };

  const foundActive = filtered.findIndex(option => option.value === activeValue && !option.disabled);
  const activeIndex = foundActive >= 0 ? foundActive : filtered.findIndex(option => !option.disabled);
  const active = filtered[activeIndex]?.disabled ? undefined : filtered[activeIndex];
  const optionId = (option: FilterableSelectOption) => `${listboxId}-${encodeURIComponent(option.value)}`;
  React.useEffect(() => {
    if (isOpen && active) document.getElementById(optionId(active))?.scrollIntoView?.({ block: "nearest" });
  });

  return (
    <div ref={rootRef} className="relative">
      <input
        id={inputId}
        form={form}
        role="combobox"
        aria-label={ariaLabel}
        aria-controls={listboxId}
        aria-expanded={isOpen}
        aria-activedescendant={isOpen && active ? optionId(active) : undefined}
        aria-describedby={describedBy}
        aria-invalid={invalid}
        aria-autocomplete="list"
        className={cn(
          selectFieldClass, "flex w-full pr-8 py-1 placeholder:text-muted-foreground",
          className,
        )}
        value={isOpen ? query : selected?.label ?? ""}
        placeholder={placeholder}
        disabled={disabled}
        autoComplete="off"
        onFocus={beginSearch}
        onClick={() => { if (!isOpen) beginSearch(); }}
        onCompositionStart={() => { composing.current = true; }}
        onCompositionEnd={() => { composing.current = false; }}
        onChange={(event) => {
          setQuery(event.target.value);
          setOpen(true);
          setActiveValue(undefined);
        }}
        onKeyDown={(event) => {
          if (composing.current || event.nativeEvent.isComposing || event.keyCode === 229) {
            if (["Enter", "Escape", "ArrowDown", "ArrowUp"].includes(event.key)) { event.stopPropagation(); if (event.key === "Enter") event.preventDefault(); }
            return;
          }
          if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            if (!isOpen) { beginSearch(); return; }
            const step = event.key === "ArrowDown" ? 1 : -1;
            let next = active ? activeIndex + step : step > 0 ? 0 : filtered.length - 1;
            while (next >= 0 && next < filtered.length && filtered[next].disabled) next += step;
            if (filtered[next]) setActiveValue(filtered[next].value);
          } else if (event.key === "Enter" && isOpen) {
            event.preventDefault(); event.stopPropagation(); choose(active);
          } else if (event.key === "Escape" && isOpen) {
            event.preventDefault(); event.stopPropagation(); close();
          } else if (event.key === "Tab") close();
        }}
        onBlur={close}
      />
      <ChevronDown aria-hidden="true" className="pointer-events-none absolute right-3 top-2.5 size-4 text-muted-foreground" />
      {isOpen && (
        <ul
          id={listboxId}
          role="listbox"
          className="absolute z-50 mt-1 max-h-60 w-full overflow-y-auto rounded-xl border border-border bg-card p-1 text-sm shadow-lg"
        >
          {filtered.length === 0 ? (
            <li role="option" aria-disabled="true" className="rounded-md px-3 py-2 text-muted-foreground">
              {noOptionsLabel}
            </li>
          ) : filtered.map((option, index) => (
            <li
              key={option.value}
              id={optionId(option)}
              role="option"
              aria-selected={option.value === value}
              aria-disabled={option.disabled || undefined}
              className={cn(
                "cursor-pointer rounded-lg px-3 py-2 text-foreground",
                index === activeIndex && "bg-primary/10",
                option.disabled && "cursor-not-allowed opacity-50",
              )}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => choose(option)}
              onMouseEnter={() => setActiveValue(option.value)}
            >
              {option.label}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
