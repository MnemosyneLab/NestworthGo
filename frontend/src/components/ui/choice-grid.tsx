import * as React from "react";
import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import { TONE_CLASSES, type Tone } from "@/lib/tone";

export interface ChoiceOption {
  value: string;
  label: string;
  icon: LucideIcon;
  tone: Tone;
}

/**
 * ChoiceGrid is a single-choice radio group rendered as icon tiles. It uses a
 * roving tabindex (one tab stop) with arrow-key navigation, like native radios.
 */
export function ChoiceGrid({
  options = [],
  groups,
  value,
  onChange,
  label,
  className,
}: {
  options?: readonly ChoiceOption[];
  groups?: ReadonlyArray<{ label: string; options: readonly ChoiceOption[] }>;
  value: string;
  onChange: (value: string) => void;
  label: string;
  className?: string;
}) {
  const labelId = React.useId();
  const refs = React.useRef<Array<HTMLButtonElement | null>>([]);
  const choices = groups?.flatMap(group => group.options) ?? options;
  const selectedIndex = Math.max(0, choices.findIndex(option => option.value === value));

  const move = (index: number) => {
    const next = (index + choices.length) % choices.length;
    onChange(choices[next].value);
    refs.current[next]?.focus();
  };

  const grid = cn("grid grid-cols-2 gap-2 sm:grid-cols-3", className);
  const renderOption = (option: ChoiceOption) => {
    const index = choices.indexOf(option);
    const checked = option.value === value;
    const Icon = option.icon;
    const tone = TONE_CLASSES[option.tone];
    return (
      <button
        key={option.value}
        ref={(node) => { refs.current[index] = node; }}
        type="button"
        role="radio"
        aria-checked={checked}
        tabIndex={index === selectedIndex ? 0 : -1}
        onClick={() => onChange(option.value)}
        onKeyDown={(event) => {
          if (event.key === "ArrowRight" || event.key === "ArrowDown") { event.preventDefault(); move(index + 1); }
          else if (event.key === "ArrowLeft" || event.key === "ArrowUp") { event.preventDefault(); move(index - 1); }
        }}
        className={cn(
          "flex items-center gap-2 rounded-xl border p-2 text-left text-xs font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50",
          checked ? "border-primary bg-primary/8 text-foreground ring-1 ring-primary/30" : "border-border bg-card text-muted-foreground hover:border-primary/40 hover:text-foreground",
        )}
      >
        <span aria-hidden="true" className={cn("inline-flex size-7 shrink-0 items-center justify-center rounded-lg [&_svg]:size-4", tone.soft, tone.text)}>
          <Icon />
        </span>
        <span className="min-w-0 leading-tight">{option.label}</span>
      </button>
    );
  };

  return (
    <div className="flex flex-col gap-1.5">
      <span id={labelId} className="text-sm font-medium leading-none">{label}</span>
      <div role="radiogroup" aria-labelledby={labelId} className={groups ? "flex flex-col gap-3" : grid}>
        {groups ? groups.map(group => (
          <div key={group.label} role="group" aria-label={group.label} className="space-y-1.5">
            <p className="text-xs font-medium text-muted-foreground" aria-hidden="true">{group.label}</p>
            <div className={grid}>{group.options.map(renderOption)}</div>
          </div>
        )) : options.map(renderOption)}
      </div>
    </div>
  );
}
