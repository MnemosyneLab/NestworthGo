import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, ChevronDown } from "lucide-react";
import { choicePillClass } from "@/components/ui/choice-menu";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { SecurityOperationRegion } from "@/lib/businessCapture";
import { cn } from "@/lib/utils";
import { effectiveEffort, selectableEfforts, type ModelOption, type Selection } from "./modelSelection";

/** One composer chip for the model and its reasoning effort ("GPT-6.1 Sol  Medium").
 * Its popover lists the enabled models and, for the chosen model, the efforts
 * from the backend catalog. A removed model stays visible (warning) but cannot send. */
export function ModelSelector({ models, catalog, selection, disabled, onChange }: {
  models: string[]; catalog: ModelOption[]; selection: Selection;
  disabled: boolean; onChange: (selection: Selection) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  // Forget an open popover immediately when its owner blocks interaction.
  if (disabled && open) setOpen(false);
  const efforts = selectableEfforts(catalog.find(option => option.id === selection.model)?.efforts);
  const valid = models.includes(selection.model);
  const modelText = selection.model === "unconfigured" ? t("aiUsage.chooseModel") : selection.model;
  const effort = effectiveEffort(efforts, selection.reasoningEffort);
  const effortText = !valid || !efforts.length ? "" : t(`aiModels.efforts.${effort}`);
  const name = `${t("aiChat.model")}: ${modelText}${effortText ? ` · ${t("aiModels.reasoning")}: ${effortText}` : ""}`;
  return <SecurityOperationRegion className="contents">
    <Popover open={open && !disabled} onOpenChange={next => setOpen(next && !disabled)}>
      <PopoverTrigger type="button" disabled={disabled} aria-label={name} title={name}
        className={cn(choicePillClass(valid ? "default" : "warning"), "max-w-full")}>
        <span className="min-w-0 truncate">{modelText}</span>
        {effortText && <span className="shrink-0 font-normal text-muted-foreground">{effortText}</span>}
        <ChevronDown aria-hidden="true" className="size-3 shrink-0 opacity-60" />
      </PopoverTrigger>
      <PopoverContent side="top" sideOffset={6} aria-label={t("aiChat.model")} className="w-[min(20rem,calc(100vw-2rem))] max-h-[min(28rem,70dvh)] overflow-y-auto p-1">
        <h3 className="px-3 pb-1 pt-2 text-xs font-medium text-muted-foreground">{t("aiUsage.chooseModel")}</h3>
        <div className="space-y-0.5">
          {!valid && <button type="button" disabled aria-pressed="true" className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm opacity-50">
            <Check aria-hidden="true" className="size-4 shrink-0" /><span className="min-w-0 flex-1 truncate">{modelText}</span>
          </button>}
          {models.map(id => <button key={id} type="button" aria-pressed={id === selection.model} disabled={disabled}
            className={cn("flex min-h-11 w-full items-center gap-2 rounded-lg px-3 text-left text-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 disabled:opacity-50 xl:min-h-9", id === selection.model && "bg-primary/10")}
            onClick={() => { if (id !== selection.model) onChange({ model: id, reasoningEffort: effectiveEffort(selectableEfforts(catalog.find(option => option.id === id)?.efforts), "") }); }}>
            <Check aria-hidden="true" className={cn("size-4 shrink-0 text-primary", id !== selection.model && "invisible")} />
            <span className="min-w-0 flex-1 truncate">{id}</span>
          </button>)}
        </div>
        {valid && efforts.length > 0 && <div className="mt-1 border-t border-border pt-1">
          <h3 className="px-3 pb-1 pt-2 text-xs font-medium text-muted-foreground">{t("aiModels.reasoning")}</h3>
          <div className="space-y-0.5">
            {efforts.map(id => <button key={id} type="button" aria-pressed={id === effort} disabled={disabled}
              className={cn("flex min-h-11 w-full items-center gap-2 rounded-lg px-3 text-left text-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 disabled:opacity-50 xl:min-h-9", id === effort && "bg-primary/10")}
              onClick={() => { if (id !== effort) onChange({ ...selection, reasoningEffort: id }); setOpen(false); }}>
              <Check aria-hidden="true" className={cn("size-4 shrink-0 text-primary", id !== effort && "invisible")} />
              <span className="min-w-0 flex-1 truncate">{t(`aiModels.efforts.${id}`)}</span>
            </button>)}
          </div>
        </div>}
      </PopoverContent>
    </Popover>
    {!valid && <p role="status" className="w-full text-xs text-muted-foreground">{t("aiModels.chooseEnabled")}</p>}
  </SecurityOperationRegion>;
}
