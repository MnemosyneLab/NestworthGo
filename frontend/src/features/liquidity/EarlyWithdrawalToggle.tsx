import { Zap } from "lucide-react";
import { useTranslation } from "react-i18next";
import { InfoPopover } from "@/components/ui/info-popover";
import { cn } from "@/lib/utils";

/** Native checkbox (no canonical wrapper exists) styled as a pill so the on state is obvious. */
export function EarlyWithdrawalToggle({ checked, onChange }: { checked: boolean; onChange: (checked: boolean) => void }) {
  const { t } = useTranslation();
  return <div className="flex items-center gap-1">
    <label className={cn(
      "inline-flex h-9 cursor-pointer items-center gap-2 rounded-xl border px-3 text-sm font-medium transition-colors focus-within:ring-2 focus-within:ring-primary/50",
      checked ? "border-warning bg-warning/18 text-warning-foreground" : "border-border bg-card text-foreground hover:border-primary/40",
    )}>
      <input type="checkbox" className="size-4 accent-primary" checked={checked} onChange={(event) => onChange(event.target.checked)} />
      <Zap className="size-3.5" aria-hidden="true" />
      {t("availableFunds.considerEarly")}
    </label>
    <InfoPopover label={t("availableFunds.earlyOptions")}>{t("availableFunds.earlyInfo")}</InfoPopover>
  </div>;
}
