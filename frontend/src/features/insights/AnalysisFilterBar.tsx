import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { FilterableSelect } from "@/components/ui/filterable-select";
import { useCatalog } from "@/queries/catalog";
import { useAccounts } from "@/queries/accounts";
import { useMembers } from "@/queries/directory";
import { useInstruments } from "@/queries/investments";
import { displayEnum } from "@/lib/display";
import { instrumentDisplayLabel } from "@/lib/instrumentDisplay";
import { useBootstrap } from "@/queries/household";
import { useHistoryOrigin } from "@/queries/history";
import type { AnalysisNavigationContext } from "@/app/navigation";
import type { AnalysisSessionState } from "@/stores/analysis";
import { subMonths, subDays } from "date-fns";
import { lastClosedDate, parseYmd, ymd } from "@/features/insights/calendar";
import { activeReturnTrendRange, returnTrendRange, originLocalDate } from "@/features/insights/analysisRequest";

export function AnalysisFilterBar({
  session,
  onChange,
  onReset,
  resolvedRange,
  returnPresets = false,
}: {
  session: AnalysisSessionState;
  returnPresets?: boolean;
  resolvedRange?: { from: string; to: string };
  onChange: (value: AnalysisNavigationContext) => void;
  onReset: () => void;
}) {
  const { t } = useTranslation();
  const [selectedPreset, setSelectedPreset] = useState<string | null>(null);
  const bootstrap = useBootstrap();
  const origin = useHistoryOrigin();
  const accounts = useAccounts();
  const instruments = useInstruments();
  const catalog = useCatalog();
  const members = useMembers();

  const selectedFilters = [
    session.moreFilters.accountId && (accounts.data ?? []).find((item) => item.account.id === session.moreFilters.accountId)?.account.name,
    session.moreFilters.currency,
    typeof session.moreFilters.assetClass === "string" && (session.moreFilters.assetClass === "cash" ? t("insights.cash") : displayEnum(t, "enum", session.moreFilters.assetClass)),
    session.moreFilters.memberId && (members.data ?? []).find((item) => item.id === session.moreFilters.memberId)?.name,
    session.moreFilters.instrumentId && (instruments.data ?? []).find((item) => item.id === session.moreFilters.instrumentId)?.name,
  ].filter((value): value is string => typeof value === "string" && value.length > 0);

  const update = (value: AnalysisNavigationContext) => onChange(value);
  const scope = session.scope ?? "portfolio";
  const closedDate = lastClosedDate(origin.data?.timezone);
  const originDate = origin.data?.startedAt ? originLocalDate(origin.data.startedAt, origin.data.timezone) : undefined;
  const fromMax = session.to && session.to < closedDate ? session.to : closedDate;
  const toMin = session.from && (!originDate || session.from > originDate) ? session.from : originDate;

  const presets = ["1w", "1m", "3m", "6m", "1y"] as const;
  const presetRange = (preset: typeof presets[number]) => {
    const end = parseYmd(closedDate);
    const start = preset === "1w" ? subDays(end, 6) : subDays(subMonths(end, preset === "1y" ? 12 : Number(preset.slice(0, -1))), -1);
    const from = ymd(start);
    return { from: originDate && from < originDate ? originDate : from, to: closedDate };
  };

  return (
    <section aria-label={t("insights.filters")} className="flex flex-col gap-2 rounded-xl border border-border bg-card/60 p-3">
      <div className="flex flex-wrap items-end gap-3">
        <div className="flex min-w-40 flex-1 flex-col gap-1.5">
          <Label htmlFor="analysis-scope">{t("insights.scope")}</Label>
          <NativeSelect
            id="analysis-scope"
            value={scope}
            onChange={(event) => {
              const next = event.target.value as AnalysisNavigationContext["scope"];
              update({ scope: next, scopeId: "" });
            }}
          >
            <option value="portfolio">{t("analytics.scopePortfolio")}</option>
            <option value="account">{t("analytics.scopeAccount")}</option>
            <option value="instrument">{t("analytics.scopeInstrument")}</option>
          </NativeSelect>
        </div>

        {scope !== "portfolio" && (
          <div className="flex min-w-52 flex-1 flex-col gap-1.5">
            <Label htmlFor="analysis-scope-id">{t("analytics.scopeSelection")}</Label>
            {scope === "account" ? (
              <NativeSelect
                id="analysis-scope-id"
                value={session.scopeId}
                onChange={(event) => update({ scopeId: event.target.value })}
              >
                <option value="">{t("common.selectOption")}</option>
                {(accounts.data ?? []).map((record) => <option key={record.account.id} value={record.account.id}>{record.account.name}</option>)}
              </NativeSelect>
            ) : (
              <FilterableSelect
                id="analysis-scope-id"
                value={session.scopeId}
                options={(instruments.data ?? []).filter((instrument) => !instrument.archivedAt).map((instrument) => ({ value: instrument.id, label: instrumentDisplayLabel(instrument, instrument.name) }))}
                onValueChange={(scopeId) => update({ scopeId })}
                placeholder={t("common.selectOption")}
                noOptionsLabel={t("common.noMatches")}
              />
            )}
          </div>
        )}

        <div className="flex min-w-40 flex-1 flex-col gap-1.5">
          <Label htmlFor="analysis-from">{t("insights.from")}</Label>
          <DatePicker id="analysis-from" value={session.from || resolvedRange?.from || ""} min={originDate} max={fromMax} onChange={(from) => update({ from, to: session.to || resolvedRange?.to })} placeholder={t("common.selectOption")} />
        </div>
        <div className="flex min-w-40 flex-1 flex-col gap-1.5">
          <Label htmlFor="analysis-to">{t("insights.to")}</Label>
          <DatePicker id="analysis-to" value={session.to || resolvedRange?.to || ""} min={toMin} max={closedDate} onChange={(to) => update({ to, from: session.from || resolvedRange?.from })} placeholder={t("common.selectOption")} />
        </div>
        <Button type="button" variant="outline" onClick={onReset}>{t("insights.reset")}</Button>
      </div>

      <div className="flex flex-wrap items-center gap-2" role="group" aria-label={t("rangeShortcuts.label")}>
        <span className="mr-1 text-sm text-muted-foreground">{t("rangeShortcuts.label")}</span>
        {!returnPresets && presets.map((preset) => {
          const range = presetRange(preset);
          const active = selectedPreset === preset && session.from === range.from && session.to === range.to;
          return <Button key={preset} type="button" size="sm" variant={active ? "default" : "outline"} aria-pressed={active} disabled={Boolean(originDate && originDate > closedDate)} onClick={() => { setSelectedPreset(preset); update(range); }}>{t(`rangeShortcuts.${preset}`)}</Button>;
        })}
        {returnPresets && (["30d", "ytd", "1y", "3y", "all"] as const).map((range) => {
          const active = origin.data && activeReturnTrendRange(session, origin.data.startedAt, origin.data.timezone) === range;
          return <Button key={range} type="button" size="sm" variant={active ? "default" : "outline"} aria-pressed={Boolean(active)} disabled={!origin.data || Boolean(originDate && originDate > closedDate)} onClick={() => { if (origin.data) update(returnTrendRange(range, origin.data.startedAt, origin.data.timezone)); }}>{t(`insights.range${range === "30d" ? "30d" : range === "ytd" ? "Ytd" : range === "1y" ? "1y" : range === "3y" ? "3y" : "All"}`)}</Button>;
        })}
        <span className="text-xs text-muted-foreground">{t("rangeShortcuts.help")}</span>
      </div>
      <details className="border-t border-border/70 pt-2">
        <summary className="cursor-pointer text-sm font-medium text-foreground">{t("insights.moreFilters")}<span className="ml-2 font-normal text-muted-foreground">{[
          session.valuation === "native" ? t("insights.valuationNative") : t("insights.valuationBase", { currency: bootstrap.data?.household?.baseCurrency ?? "—" }),
          t(session.includeCash ? "insights.cashIncluded" : "insights.cashExcluded"),
          ...selectedFilters,
        ].join(" · ")}</span></summary>
        <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <div className="flex min-w-36 flex-1 flex-col gap-1.5">
            <Label htmlFor="analysis-valuation">{t("insights.valuation")}</Label>
            <NativeSelect id="analysis-valuation" value={session.valuation} onChange={(event) => update({ valuation: event.target.value as "base" | "native" })}>
              <option value="base">{t("insights.valuationBase", { currency: bootstrap.data?.household?.baseCurrency ?? "—" })}</option>
              <option value="native">{t("insights.valuationNative")}</option>
            </NativeSelect>
          </div>

          <div className="flex min-w-36 flex-1 flex-col gap-1.5">
            <Label htmlFor="analysis-cash">{t("insights.cash")}</Label>
            <NativeSelect id="analysis-cash" value={session.includeCash ? "include" : "exclude"} onChange={(event) => update({ includeCash: event.target.value === "include" })}>
              <option value="include">{t("insights.cashIncluded")}</option>
              <option value="exclude">{t("insights.cashExcluded")}</option>
            </NativeSelect>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="analysis-filter-account">{t("analytics.scopeAccount")}</Label>
            <NativeSelect
              id="analysis-filter-account"
              value={typeof session.moreFilters.accountId === "string" ? session.moreFilters.accountId : ""}
              onChange={(event) => update({ moreFilters: { accountId: event.target.value || undefined } })}
            >
              <option value="">{t("common.all")}</option>
              {(accounts.data ?? []).filter((record) => !record.account.archivedAt).map((record) => <option key={record.account.id} value={record.account.id}>{record.account.name}</option>)}
            </NativeSelect>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="analysis-filter-currency">{t("insights.currency")}</Label>
            <NativeSelect
              id="analysis-filter-currency"
              value={typeof session.moreFilters.currency === "string" ? session.moreFilters.currency : ""}
              onChange={(event) => update({ moreFilters: { currency: event.target.value || undefined } })}
            >
              <option value="">{t("common.all")}</option>
              {(catalog.data?.currencies ?? []).map((currency) => <option key={currency} value={currency}>{currency}</option>)}
            </NativeSelect>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="analysis-filter-asset-class">{t("insights.assetClass")}</Label>
            <NativeSelect
              id="analysis-filter-asset-class"
              value={typeof session.moreFilters.assetClass === "string" ? session.moreFilters.assetClass : ""}
              onChange={(event) => update({ moreFilters: { assetClass: event.target.value || undefined } })}
            >
              <option value="">{t("common.all")}</option>
              <option value="cash">{t("insights.cash")}</option>
              {(catalog.data?.instrumentTypes ?? []).map((assetClass) => <option key={assetClass} value={assetClass}>{assetClass}</option>)}
            </NativeSelect>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="analysis-filter-member">{t("insights.member")}</Label>
            <NativeSelect
              id="analysis-filter-member"
              value={typeof session.moreFilters.memberId === "string" ? session.moreFilters.memberId : ""}
              onChange={(event) => update({ moreFilters: { memberId: event.target.value || undefined } })}
            >
              <option value="">{t("common.all")}</option>
              {(members.data ?? []).filter((member) => !member.archivedAt).map((member) => <option key={member.id} value={member.id}>{member.name}</option>)}
            </NativeSelect>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="analysis-filter-instrument">{t("insights.instrument")}</Label>
            <FilterableSelect
              id="analysis-filter-instrument"
              value={typeof session.moreFilters.instrumentId === "string" ? session.moreFilters.instrumentId : ""}
              options={[{ value: "", label: t("common.all") }, ...(instruments.data ?? []).filter((instrument) => !instrument.archivedAt).map((instrument) => ({ value: instrument.id, label: instrumentDisplayLabel(instrument, instrument.name) }))]}
              onValueChange={(instrumentId) => update({ moreFilters: { instrumentId: instrumentId || undefined } })}
              placeholder={t("common.all")}
              noOptionsLabel={t("common.noMatches")}
            />
          </div>
        </div>
      </details>
    </section>
  );
}
