import { useTranslation } from "react-i18next";
import { useOverview } from "@/queries/portfolio";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { formatAmount, formatPercent } from "@/lib/money";
import type { BreakdownDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

function BreakdownList({ title, items, currency }: { title: string; items: BreakdownDTO[]; currency: string }) {
  if (items.length === 0) {
    return null;
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        {items.map((item) => (
          <div key={item.key} className="flex items-center justify-between text-sm">
            <span className="text-foreground">{item.label}</span>
            <span className="flex items-center gap-2 text-muted-foreground">
              <span>{formatAmount(item.amount, currency)}</span>
              <Badge variant="secondary">{formatPercent(item.shareBps)}</Badge>
            </span>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

/**
 * OverviewPage implements the net worth / assets / liabilities /
 * breakdown views end to end through PortfolioService (implementation
 * plan Phase 4), including the "incomplete" state
 * domain.OverviewResult.Complete=false can produce: the frontend never
 * invents, hides, or zeroes a value the backend marked unavailable
 * (migration plan Sec8's acceptance criterion) — it shows an explicit
 * banner naming which inputs are missing instead.
 */
export function OverviewPage() {
  const { t } = useTranslation();
  const overview = useOverview();

  if (overview.isLoading) {
    return <p className="text-sm text-muted-foreground">{t("common.loading", { defaultValue: "Loading…" })}</p>;
  }
  if (overview.isError || !overview.data) {
    return <p role="alert" className="text-sm text-destructive">{t("accounts.loadError")}</p>;
  }

  const data = overview.data;
  const currency = data.currency || "USD";

  return (
    <div className="flex flex-col gap-6" data-testid="overview-page">
      {!data.complete && (data.missingInputs?.length ?? 0) > 0 && (
        <div role="alert" className="rounded-md border border-warning bg-warning/10 p-4 text-sm text-warning-foreground">
          <p className="font-medium">{t("overview.incomplete", { defaultValue: "Some values are incomplete" })}</p>
          <ul className="mt-1 list-inside list-disc">
            {(data.missingInputs ?? []).map((missing, index) => (
              <li key={`${missing.kind}-${missing.accountId}-${index}`}>
                {missing.instrumentName || missing.kind} — {missing.accountId}
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle>{t("overview.netWorth", { defaultValue: "Net worth" })}</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-semibold" data-testid="overview-net-worth">
              {formatAmount(data.netWorth, currency)}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>{t("overview.assets", { defaultValue: "Assets" })}</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-semibold text-success" data-testid="overview-assets">
              {formatAmount(data.assets, currency)}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>{t("overview.liabilities", { defaultValue: "Liabilities" })}</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-semibold text-destructive" data-testid="overview-liabilities">
              {formatAmount(data.liabilities, currency)}
            </p>
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <BreakdownList title={t("overview.byCategory", { defaultValue: "By category" })} items={data.byCategory ?? []} currency={currency} />
        <BreakdownList title={t("overview.byMember", { defaultValue: "By member" })} items={data.byMember ?? []} currency={currency} />
        <BreakdownList
          title={t("overview.byInstitution", { defaultValue: "By institution" })}
          items={data.byInstitution ?? []}
          currency={currency}
        />
        <BreakdownList title={t("overview.byGroup", { defaultValue: "By group" })} items={data.byGroup ?? []} currency={currency} />
      </div>
    </div>
  );
}
