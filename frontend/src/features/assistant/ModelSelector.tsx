import { useTranslation } from "react-i18next";
import { Brain, Cpu } from "lucide-react";
import { ChoiceMenu } from "@/components/ui/choice-menu";
import { SecurityOperationRegion } from "@/lib/businessCapture";
import type { ModelOption, Selection } from "./modelSelection";

/** Model and reasoning-effort chips for the composer. The backend catalog owns
 * which efforts exist; a removed model stays visible (warning) but cannot send. */
export function ModelSelector({ models, catalog, selection, disabled, onChange }: {
  models: string[]; catalog: ModelOption[]; selection: Selection;
  disabled: boolean; onChange: (selection: Selection) => void;
}) {
  const { t } = useTranslation();
  const efforts = catalog.find(option => option.id === selection.model)?.efforts ?? [];
  const valid = models.includes(selection.model);
  const modelOptions = [
    ...(valid ? [] : [{ value: selection.model, label: selection.model === "unconfigured" ? t("aiUsage.chooseModel") : selection.model, disabled: true }]),
    ...models.map(id => ({ value: id, label: id })),
  ];
  const effortOptions = [
    { value: "", label: t(efforts.length ? "aiModels.default" : "aiModels.unspecified") },
    ...efforts.map(effort => ({ value: effort, label: t(`aiModels.efforts.${effort}`) })),
  ];
  // `contents` lets the chips wrap together with sibling chips in the composer row.
  return <SecurityOperationRegion className="contents">
    <ChoiceMenu label={t("aiChat.model")} icon={Cpu} tone={valid ? "default" : "warning"} className="max-w-48" value={selection.model} options={modelOptions}
      disabled={disabled} onChange={model => onChange({ model, reasoningEffort: "" })} />
    <ChoiceMenu label={t("aiModels.reasoning")} icon={Brain} value={selection.reasoningEffort} options={effortOptions}
      disabled={disabled || !valid || efforts.length === 0} onChange={reasoningEffort => onChange({ ...selection, reasoningEffort })} />
    {!valid && <p role="status" className="w-full text-xs text-muted-foreground">{t("aiModels.chooseEnabled")}</p>}
  </SecurityOperationRegion>;
}
