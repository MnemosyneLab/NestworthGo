import * as React from "react";
import { useTranslation } from "react-i18next";
import { Popover, PopoverList, PopoverTrigger } from "./popover";
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
  options: readonly FilterableSelectOption[];
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
  noOptionsLabel,
  disabled = false,
  className,
  "aria-label": ariaLabel,
  "aria-describedby": describedBy,
  "aria-invalid": invalid,
}: FilterableSelectProps) {
  const { t } = useTranslation();
  const generatedId = React.useId();
  const inputId = id ?? generatedId;
  const listboxId = `${inputId}-options`;
  const selected = options.find((option) => option.value === value);
  const [query, setQuery] = React.useState("");
  const [open, setOpen] = React.useState(false);
  const [activeValue, setActiveValue] = React.useState<string | undefined>();
  const composing = React.useRef(false);
  const isOpen = open && !disabled;
  const beginSearch = () => {
    if (disabled) return;
    setQuery("");
    setActiveValue(options.find(option => option.value === value && !option.disabled)?.value ?? options.find(option => !option.disabled)?.value);
    setOpen(true);
  };
  const close = () => {
    setOpen(false);
    setQuery("");
    composing.current = false;
  };
  if (disabled && open) setOpen(false);
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const filtered = normalizedQuery ? options.filter((option) =>
    `${option.label} ${option.value} ${option.searchKeywords ?? ""}`.toLocaleLowerCase().includes(normalizedQuery),
  ) : options;

  const choose = (option: FilterableSelectOption | undefined) => {
    if (!option || option.disabled) {
      return;
    }
    onValueChange(option.value);
    close();
  };

  const foundActive = filtered.findIndex(option => option.value === activeValue && !option.disabled);
  const activeIndex = foundActive >= 0 ? foundActive : filtered.findIndex(option => !option.disabled);
  const active = filtered[activeIndex]?.disabled ? undefined : filtered[activeIndex];
  const optionId = (option: FilterableSelectOption) => `${listboxId}-${encodeURIComponent(option.value)}`;
  const activeId = active ? optionId(active) : undefined;
  React.useEffect(() => {
    if (isOpen && activeId) document.getElementById(activeId)?.scrollIntoView?.({ block: "nearest" });
  }, [isOpen, activeId]);

  return (
    <Popover open={isOpen} onOpenChange={(next, details) => {
      if (!next && details.reason === "escape-key" && composing.current) details.cancel();
      else if (!next) close();
    }}>
      <div className={cn("relative min-w-0 max-w-full", className)}>
        <PopoverTrigger
          nativeButton={false}
          render={<input
            form={form}
            disabled={disabled}
            value={isOpen ? query : selected?.label ?? ""}
            placeholder={isOpen ? t("common.searchOptions") : placeholder}
            autoComplete="off"
            onCompositionStart={() => { composing.current = true; }}
            onCompositionEnd={() => { composing.current = false; }}
            onChange={event => {
              setQuery(event.target.value);
              setOpen(true);
              setActiveValue(undefined);
            }}
          />}
          id={inputId}
          role="combobox"
          aria-haspopup="listbox"
          aria-label={ariaLabel}
          aria-controls={isOpen ? listboxId : undefined}
          aria-expanded={isOpen}
          aria-activedescendant={isOpen ? activeId : undefined}
          aria-describedby={describedBy}
          aria-invalid={invalid}
          aria-autocomplete="list"
          className={cn(selectFieldClass, "w-full pe-9 placeholder:text-muted-foreground")}
          disabled={disabled}
          onFocus={beginSearch}
          onClick={event => {
            event.preventBaseUIHandler();
            if (!isOpen) beginSearch();
          }}
          onKeyDown={(event) => {
            // The editable trigger owns input keystrokes and option navigation.
            event.preventBaseUIHandler();
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
        <span aria-hidden="true" className="select-chevron pointer-events-none absolute end-3 top-1/2 -translate-y-1/2" />
        {isOpen && <PopoverList id={listboxId}>
          {filtered.length === 0 ? (
            <div role="status" className="px-2 py-1.5 text-muted-foreground">
              {noOptionsLabel ?? t("common.noMatches")}
            </div>
          ) : filtered.map((option, index) => (
            <div
              key={option.value}
              id={optionId(option)}
              role="option"
              aria-selected={option.value === value}
              aria-disabled={option.disabled || undefined}
              className={cn(
                "select-option cursor-default",
                option.disabled && "cursor-not-allowed",
              )}
              data-highlighted={index === activeIndex || undefined}
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => choose(option)}
              onMouseEnter={() => setActiveValue(option.value)}
            >
              <span aria-hidden="true" className={cn("size-3.5 shrink-0", option.value === value && "select-check")} />
              <span className="min-w-0">{option.label}</span>
            </div>
          ))}
        </PopoverList>}
      </div>
    </Popover>
  );
}
