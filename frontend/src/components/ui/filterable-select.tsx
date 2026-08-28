import * as React from "react";
import { cn } from "@/lib/utils";

export type FilterableSelectOption = {
  value: string;
  label: string;
  disabled?: boolean;
};

export type FilterableSelectProps = {
  id?: string;
  value: string;
  options: FilterableSelectOption[];
  onValueChange: (value: string) => void;
  placeholder?: string;
  noOptionsLabel?: string;
  disabled?: boolean;
  className?: string;
  "aria-label"?: string;
};

/** A small keyboard-accessible combobox for long closed catalogs such as IANA timezones. */
export function FilterableSelect({
  id,
  value,
  options,
  onValueChange,
  placeholder,
  noOptionsLabel = "No matches",
  disabled = false,
  className,
  "aria-label": ariaLabel,
}: FilterableSelectProps) {
  const rootRef = React.useRef<HTMLDivElement>(null);
  const generatedId = React.useId();
  const inputId = id ?? generatedId;
  const listboxId = `${inputId}-options`;
  const selected = options.find((option) => option.value === value);
  const [query, setQuery] = React.useState(selected?.label ?? value);
  const [open, setOpen] = React.useState(false);
  const [activeIndex, setActiveIndex] = React.useState(0);

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
    return !normalizedQuery || `${option.label} ${option.value}`.toLocaleLowerCase().includes(normalizedQuery);
  });

  const choose = (option: FilterableSelectOption | undefined) => {
    if (!option || option.disabled) {
      return;
    }
    onValueChange(option.value);
    setQuery(option.label);
    setOpen(false);
  };

  return (
    <div ref={rootRef} className="relative">
      <input
        id={id}
        role="combobox"
        aria-label={ariaLabel}
        aria-controls={listboxId}
        aria-expanded={open}
        aria-autocomplete="list"
        className={cn(
          "flex h-9 w-full rounded-lg border border-border bg-card px-3 py-1 text-sm text-foreground shadow-xs transition-[border-color,box-shadow] placeholder:text-muted-foreground focus-visible:border-primary/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 disabled:cursor-not-allowed disabled:opacity-50",
          className,
        )}
        value={open ? query : selected?.label ?? value}
        placeholder={placeholder}
        disabled={disabled}
        autoComplete="off"
        onFocus={(event) => {
          setQuery(selected?.label ?? value);
          setOpen(true);
          event.currentTarget.select();
        }}
        onChange={(event) => {
          setQuery(event.target.value);
          setOpen(true);
          setActiveIndex(0);
        }}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown") {
            event.preventDefault();
            setOpen(true);
            setActiveIndex((index) => Math.min(index + 1, Math.max(0, filtered.length - 1)));
          } else if (event.key === "ArrowUp") {
            event.preventDefault();
            setActiveIndex((index) => Math.max(0, index - 1));
          } else if (event.key === "Enter") {
            event.preventDefault();
            choose(filtered[activeIndex]);
          } else if (event.key === "Escape") {
            event.preventDefault();
            setOpen(false);
          }
        }}
        onBlur={() => {
          window.setTimeout(() => setOpen(false), 100);
        }}
      />
      {open && (
        <ul
          id={listboxId}
          role="listbox"
          className="absolute z-50 mt-1 max-h-60 w-full overflow-y-auto rounded-lg border border-border bg-card p-1 text-sm shadow-lg"
        >
          {filtered.length === 0 ? (
            <li role="option" aria-disabled="true" className="rounded-md px-3 py-2 text-muted-foreground">
              {noOptionsLabel}
            </li>
          ) : filtered.map((option, index) => (
            <li
              key={option.value}
              role="option"
              aria-selected={option.value === value}
              aria-disabled={option.disabled || undefined}
              className={cn(
                "cursor-pointer rounded-md px-3 py-2 text-foreground",
                index === activeIndex && "bg-muted",
                option.disabled && "cursor-not-allowed opacity-50",
              )}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => choose(option)}
              onMouseEnter={() => setActiveIndex(index)}
            >
              {option.label}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
