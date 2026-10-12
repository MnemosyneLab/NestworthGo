import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { CompositionChart } from "@/components/charts/CompositionChart";
import type { ChartCategory } from "@/components/charts/collapseCategories";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { formatAmount, formatPercent } from "@/lib/money";
import { Button } from "@/components/ui/button";
import type { AccountCompositionDTO, AccountCompositionItemDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

export function AccountCompositionPanel({ composition, updatedAt }: { composition: AccountCompositionDTO; updatedAt?: number }) {
  const { t } = useTranslation();
  const [mode, setMode] = useState<"type" | "detail">("type");
  const [expanded, setExpanded] = useState(false);
  const modeName = useId();
  if (composition.status === "unsupported") return null;

  const details = composition.details ?? [];
  const types = composition.types ?? [];
  const detailSlices = composition.detailSlices ?? [];
  const category = (item: AccountCompositionItemDTO, byType = false): ChartCategory => ({
    key: item.key,
    label: byType
      ? item.typeKey === "bank_investment_product" ? t("accounts.compositionView.bankProducts")
        : item.typeKey === "unknown" ? t("accounts.compositionView.unknownType") : t(`enum.${item.typeKey}`)
      : item.key === "account-composition:remainder" ? t("accounts.compositionView.remainder", { count: item.detailRefs?.length ?? 0 }) : item.label,
    amount: item.baseAmountExact || "0",
    identityLabel: byType ? undefined : item.identityLabel,
    shareBps: item.shareBps,
    nativeAmount: item.nativeAmount || undefined,
    nativeCurrency: item.nativeCurrency || undefined,
    available: item.valuedCount > 0,
    missingCount: item.missingCount,
  });
  const source = details.map((item) => category(item));
  const slices = mode === "type" ? types.map((item) => category(item, true)) : detailSlices.map((item) => category(item));
  const remainderCount = detailSlices.find((item) => item.key === "account-composition:remainder")?.detailRefs?.length ?? 0;
  const legendSliceKeys = mode === "detail" ? Object.fromEntries(detailSlices.flatMap((slice) =>
    (slice.detailRefs ?? []).map((ref) => [ref, slice.key]),
  )) : undefined;
  const canChart = composition.status === "ready" || composition.status === "partial";

  return (
    <Card data-testid="account-composition-panel">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-3">
        <CardTitle>{t("accounts.composition")}</CardTitle>
        <div role="radiogroup" aria-label={t("accounts.compositionView.grouping")} className="inline-flex flex-wrap gap-1 rounded-lg bg-muted p-1">
          {(["type", "detail"] as const).map((value) => (
            <label key={value} className="relative cursor-pointer">
              <input className="peer sr-only" type="radio" name={modeName} value={value} checked={mode === value} onChange={() => { setMode(value); setExpanded(false); }} />
              <span className="inline-flex h-9 items-center justify-center rounded-lg px-3.5 text-sm font-semibold text-muted-foreground transition-colors hover:text-foreground peer-checked:bg-card peer-checked:text-primary peer-checked:shadow-sm peer-focus-visible:ring-2 peer-focus-visible:ring-primary/50">
                {t(`accounts.compositionView.${value}`)}
              </span>
            </label>
          ))}
        </div>
      </CardHeader>
      <CardContent className="min-w-0">
        {canChart ? (
          <CompositionChart
            key={`${mode}:${updatedAt ?? ""}:${JSON.stringify(composition)}`}
            title={t("accounts.composition")}
            items={slices}
            legendItems={mode === "detail" && expanded ? source : slices}
            dataTableItems={source}
            precomputedCategories
            legendSliceKeys={legendSliceKeys}
            interactionResetKey={expanded ? "expanded" : "collapsed"}
            currency={composition.currency}
            centerValue={composition.totalExact || "0"}
            centerCaption={t("charts.valuedSubtotal")}
            ariaLabel={t("accounts.composition")}
            summary={t("accounts.compositionView.hint")}
            description={t("accounts.compositionView.hint")}
            complete={composition.complete}
            missingCount={composition.missingCount}
          />
        ) : (
          <>
            <p role="status" className="text-sm text-muted-foreground">{t(`accounts.compositionView.${composition.status}`)}</p>
            {!composition.complete && composition.missingCount > 0 && <p className="mt-2 text-sm text-warning-foreground">{t("charts.partialValued", { count: composition.missingCount })}</p>}
            <ul className="mt-3 flex flex-col gap-2 text-sm">
              {(mode === "detail" && expanded ? source : slices).map((row) => (
                <li key={row.key} data-testid={`account-composition-row-${row.key}`} className="flex min-w-0 flex-wrap items-start justify-between gap-2 rounded-xl px-2.5 py-2">
                  <span className="min-w-0 break-words [overflow-wrap:anywhere]">{row.label}{Boolean(row.missingCount) && <span className="block text-xs text-warning-foreground">{t("charts.partialValued", { count: row.missingCount })}</span>}</span>
                  <span className="num min-w-0 text-right text-muted-foreground [overflow-wrap:anywhere]">
                    {row.nativeAmount && row.nativeCurrency && row.nativeCurrency !== composition.currency && <span className="block">{formatAmount(row.nativeAmount, row.nativeCurrency)}</span>}
                    {row.available ? formatAmount(row.amount, composition.currency) : t("accounts.noValue")} · {row.available && composition.status === "empty" ? formatPercent(0) : "—"}
                  </span>
                </li>
              ))}
            </ul>
            {details.length > 0 && (
              <details className="mt-3">
                <summary className="cursor-pointer text-sm text-primary">{t("charts.viewDataTable")}</summary>
                <div className="mt-2 overflow-x-auto">
                  <table className="num w-full text-left text-sm">
                    <thead><tr>{[t("charts.category"), t("accounts.nativeAmount"), t("accounts.householdAmount"), t("charts.share")].map((label) => <th key={label} className="px-2 py-1.5">{label}</th>)}</tr></thead>
                    <tbody>{details.map((item) => (
                      <tr key={item.key}>
                        <td className="break-words px-2 py-1.5">{item.identityLabel || item.label}</td>
                        <td className="px-2 py-1.5">{item.nativeAmount ? formatAmount(item.nativeAmount, item.nativeCurrency) : t("accounts.noValue")}</td>
                        <td className="px-2 py-1.5">{item.baseAmountExact ? formatAmount(item.baseAmountExact, composition.currency) : t("accounts.noValue")}</td>
                        <td className="px-2 py-1.5">{item.valuedCount && composition.status === "empty" ? formatPercent(0) : "—"}</td>
                      </tr>
                    ))}</tbody>
                  </table>
                </div>
              </details>
            )}
          </>
        )}
        {mode === "detail" && remainderCount > 0 && (
          <Button className="mt-3" type="button" variant="outline" size="sm" aria-expanded={expanded} onClick={() => setExpanded((value) => !value)}>
            {expanded ? t("accounts.compositionView.collapse") : t("accounts.compositionView.expand", { count: details.length })}
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
