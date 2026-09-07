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
import { useSettings } from "@/queries/settings";
import type { AnalysisNavigationContext } from "@/app/navigation";
import type { AnalysisSessionState } from "@/stores/analysis";

export function AnalysisFilterBar({
  session,
  onChange,
  onReset,
}: {
  session: AnalysisSessionState;
  onChange: (value: AnalysisNavigationContext) => void;
  onReset: () => void;
}) {
  const { t } = useTranslation();
  const settings = useSettings();
  const accounts = useAccounts();
  const instruments = useInstruments();
  const catalog = useCatalog();
  const members = useMembers();

  const update = (value: AnalysisNavigationContext) => onChange(value);
  const scope = session.scope ?? "portfolio";

  return (
    <section aria-label={t("insights.filters")} className="flex flex-col gap-4 rounded-xl border border-border bg-card/60 p-4">
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
                options={(instruments.data ?? []).filter((instrument) => !instrument.archivedAt).map((instrument) => ({ value: instrument.id, label: instrument.name }))}
                onValueChange={(scopeId) => update({ scopeId })}
                placeholder={t("common.selectOption")}
                noOptionsLabel={t("common.noMatches")}
              />
            )}
          </div>
        )}

        <div className="flex min-w-36 flex-1 flex-col gap-1.5">
          <Label htmlFor="analysis-valuation">{t("insights.valuation")}</Label>
          <NativeSelect id="analysis-valuation" value={session.valuation} onChange={(event) => update({ valuation: event.target.value as "base" | "native" })}>
            <option value="base">{t("insights.valuationBase", { currency: settings.data?.currency ?? "" })}</option>
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
      </div>

      <div className="flex flex-wrap items-end gap-3">
        <div className="flex min-w-40 flex-1 flex-col gap-1.5">
          <Label htmlFor="analysis-from">{t("insights.from")}</Label>
          <DatePicker id="analysis-from" value={session.from} max={session.to || undefined} onChange={(from) => update({ from })} placeholder={t("common.selectOption")} />
        </div>
        <div className="flex min-w-40 flex-1 flex-col gap-1.5">
          <Label htmlFor="analysis-to">{t("insights.to")}</Label>
          <DatePicker id="analysis-to" value={session.to} min={session.from || undefined} onChange={(to) => update({ to })} placeholder={t("common.selectOption")} />
        </div>
        <Button type="button" variant="outline" onClick={onReset}>{t("insights.reset")}</Button>
      </div>

      <details className="rounded-lg border border-border/70 bg-muted/20 px-3 py-2">
        <summary className="cursor-pointer text-sm font-medium text-foreground">{t("insights.moreFilters")}</summary>
        <div className="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
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
              options={[{ value: "", label: t("common.all") }, ...(instruments.data ?? []).filter((instrument) => !instrument.archivedAt).map((instrument) => ({ value: instrument.id, label: instrument.name }))]}
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
