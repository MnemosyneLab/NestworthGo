import type { ComponentType } from "react";
import { Bitcoin, Boxes, ChevronDown, FileText, Gem, Landmark, LineChart, PieChart, Wallet } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import { ChangeText } from "@/components/ui/change-text";
import { displayEnum } from "@/lib/display";
import { formatAmount, formatSignedMoney } from "@/lib/money";
import { cn } from "@/lib/utils";
import { amountDirection, type HoldingGroupKey } from "./holdingGroups";
import type { HoldingSubtotalDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics/models";

const ICONS: Record<HoldingGroupKey, ComponentType<{ className?: string }>> = {
  stock: LineChart, fund: PieChart, bond: FileText, bank_wealth: Landmark,
  precious_metal: Gem, crypto: Bitcoin, cash_like: Wallet, other: Boxes,
};

type Metric = "totalCost" | "currentValue" | "unrealizedGain";
const METRICS: ReadonlyArray<{ key: Metric; label: string; reason: "costMissingReason" | "valueMissingReason" | "gainMissingReason" }> = [
  { key: "totalCost", label: "holdingGroups.cost", reason: "costMissingReason" },
  { key: "currentValue", label: "holdingGroups.value", reason: "valueMissingReason" },
  { key: "unrealizedGain", label: "holdingGroups.gain", reason: "gainMissingReason" },
];

/** One subtotal figure. Subtotals stay per native currency; they are never converted or merged. */
function SubtotalValue({ subtotal, metric }: { subtotal: HoldingSubtotalDTO; metric: (typeof METRICS)[number] }) {
  const { t } = useTranslation();
  const value = subtotal[metric.key];
  if (!value) {
    const reason = subtotal[metric.reason];
    return <span className="text-muted-foreground">—<span className="sr-only"> {t("accounts.noValue")}{reason ? ` · ${displayEnum(t, "portfolio.missingReason", reason)}` : ""}</span></span>;
  }
  if (metric.key === "unrealizedGain") {
    return <ChangeText direction={amountDirection(value.amount) ?? "flat"}>{formatSignedMoney(value.amount, value.currency)}</ChangeText>;
  }
  return <span>{formatAmount(value.amount, value.currency)}</span>;
}

/**
 * Collapsible group header: type icon, name, holding count and the per-currency
 * cost, value and gain subtotals. The whole row toggles through one button;
 * the figures are decorative to pointer input so they never block that click.
 */
export function HoldingGroupHeader({ groupKey, label, count, subtotals, expanded, controls, onToggle }: {
  groupKey: HoldingGroupKey;
  label: string;
  count: number;
  subtotals: readonly HoldingSubtotalDTO[];
  expanded: boolean;
  controls: string;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  const Icon = ICONS[groupKey];
  return (
    <div className="relative flex flex-wrap items-center gap-x-6 gap-y-3 px-4 py-3.5">
      <button
        type="button"
        aria-expanded={expanded}
        aria-controls={controls}
        aria-label={t(expanded ? "holdingGroups.collapse" : "holdingGroups.expand", { group: label })}
        onClick={onToggle}
        className="flex min-w-0 items-center gap-3 rounded-lg text-left after:absolute after:inset-0 after:rounded-2xl after:content-[''] focus-visible:outline-none focus-visible:after:ring-2 focus-visible:after:ring-primary/50"
      >
        <ChevronDown className={cn("size-4 shrink-0 text-muted-foreground transition-transform motion-reduce:transition-none", !expanded && "-rotate-90")} aria-hidden="true" />
        <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary"><Icon className="size-[1.125rem]" /></span>
        <span className="truncate text-base font-semibold">{label}</span>
        <Badge variant="secondary">{t("holdingGroups.count", { count })}</Badge>
      </button>
      {subtotals.length > 0 && <dl className="pointer-events-none relative ml-auto grid grid-cols-3 gap-x-6 text-right sm:gap-x-8">
        {METRICS.map((metric) => <div key={metric.key} className="min-w-0">
          <dt className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">{t(metric.label)}</dt>
          {subtotals.map((subtotal) => <dd key={subtotal.currency} className="num whitespace-nowrap text-sm font-semibold">
            <SubtotalValue subtotal={subtotal} metric={metric} />
          </dd>)}
        </div>)}
      </dl>}
    </div>
  );
}
