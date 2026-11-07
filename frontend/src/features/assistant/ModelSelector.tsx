import { useTranslation } from "react-i18next";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { SecurityOperationRegion } from "@/lib/businessCapture";
import type { ModelOption, Selection } from "./modelSelection";

export function ModelSelector({ models, catalog, selection, disabled, onChange }: {
  models: string[]; catalog: ModelOption[]; selection: Selection;
  disabled: boolean; onChange: (selection: Selection) => void;
}) {
  const { t } = useTranslation();
  const efforts = catalog.find(option => option.id === selection.model)?.efforts ?? [];
  const valid = models.includes(selection.model);
  return <SecurityOperationRegion className="space-y-1">
    <div className="flex gap-2">
      <div className="min-w-0 flex-1"><Label htmlFor="ai-request-model" className="sr-only">{t("aiChat.model")}</Label>
        <NativeSelect id="ai-request-model" className="w-full min-w-0 text-xs" value={selection.model} disabled={disabled} onChange={event => onChange({model:event.target.value, reasoningEffort:""})}>
          {!valid && <option value={selection.model} disabled>{selection.model === "unconfigured" ? t("aiUsage.chooseModel") : selection.model}</option>}
          {models.map(id => <option key={id} value={id}>{id}</option>)}
        </NativeSelect>
      </div>
      <div className="min-w-0 flex-1"><Label htmlFor="ai-reasoning" className="sr-only">{t("aiModels.reasoning")}</Label>
        <NativeSelect id="ai-reasoning" className="w-full min-w-0 text-xs" value={selection.reasoningEffort} disabled={disabled || !valid || efforts.length === 0} onChange={event => onChange({...selection, reasoningEffort:event.target.value})}>
          <option value="">{t(efforts.length ? "aiModels.default" : "aiModels.unspecified")}</option>
          {efforts.map(effort => <option key={effort} value={effort}>{t(`aiModels.efforts.${effort}`)}</option>)}
        </NativeSelect>
      </div>
    </div>
    {!valid && <p role="status" className="text-xs text-muted-foreground">{t("aiModels.chooseEnabled")}</p>}
  </SecurityOperationRegion>;
}
