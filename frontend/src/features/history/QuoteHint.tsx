import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { useSettings } from "@/queries/settings";
import { formatQuoteAsOf } from "@/lib/time";
import { displayEnum, displayError } from "@/lib/display";
import type { FXQuoteDTO, InstrumentQuoteDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

export function QuoteHint({
  quote,
  quoteLabel,
  sourceKind,
  onUpdate,
  isUpdating,
  loading = false,
  error,
}: {
  quote?: InstrumentQuoteDTO | FXQuoteDTO | null;
  quoteLabel?: string;
  sourceKind?: string;
  onUpdate: () => void;
  isUpdating: boolean;
  loading?: boolean;
  error?: unknown;
}) {
  const { t, i18n } = useTranslation();
  const settings = useSettings();
  const source = quote?.sourceKind ? displayEnum(t, "portfolio", quote.sourceKind) : "";
  const sourceLabel = quote?.sourceKey && quote.sourceKey !== quote.sourceKind ? `${source} · ${quote.sourceKey}` : source;
  if (loading) {
    return <p className="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">{t("common.loading")}</p>;
  }
  if (!quote) {
    return (
      <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
        <span>{t("marketData.noQuoteYet")}</span>
        {sourceKind !== "manual" && sourceKind !== "agent" && <Button type="button" variant="outline" size="sm" onClick={onUpdate} disabled={isUpdating}>{isUpdating ? t("common.pending") : t("history.updateQuote")}</Button>}
        {sourceKind === "manual" && <span className="basis-full">{t("history.manualQuoteHelp")}</span>}
        {sourceKind === "agent" && <span className="basis-full">{t("history.agentQuoteHelp")}</span>}
        {Boolean(error) && <span role="alert" className="basis-full text-destructive">{displayError(error, t("history.actionError"))}</span>}
      </div>
    );
  }
  return (
    <p className="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
      {quoteLabel ?? t("marketData.noQuoteYet")} · {t("marketData.quotedAsOf", { time: formatQuoteAsOf(quote, settings.data?.timezone, i18n.language) })}
      {sourceLabel && <span> · {sourceLabel}</span>}
      {Boolean(error) && <span role="alert" className="ml-2 text-destructive">{displayError(error, t("history.actionError"))}</span>}
    </p>
  );
}
