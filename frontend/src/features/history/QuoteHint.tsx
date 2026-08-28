import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { useSettings } from "@/queries/settings";
import { formatTimestamp } from "@/lib/time";
import { displayError } from "@/lib/display";
import type { FXQuoteDTO, InstrumentQuoteDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

export function QuoteHint({
  quote,
  quoteLabel,
  manual,
  onUpdate,
  isUpdating,
  loading = false,
  error,
}: {
  quote?: InstrumentQuoteDTO | FXQuoteDTO | null;
  quoteLabel?: string;
  manual?: boolean;
  onUpdate: () => void;
  isUpdating: boolean;
  loading?: boolean;
  error?: unknown;
}) {
  const { t, i18n } = useTranslation();
  const settings = useSettings();
  if (loading) {
    return <p className="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">{t("common.loading")}</p>;
  }
  if (!quote) {
    return (
      <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
        <span>{t("marketData.noQuoteYet")}</span>
        {!manual && <Button type="button" variant="outline" size="sm" onClick={onUpdate} disabled={isUpdating}>{isUpdating ? t("common.pending") : t("history.updateQuote")}</Button>}
        {manual && <span className="basis-full">{t("history.manualQuoteHelp")}</span>}
        {Boolean(error) && <span role="alert" className="basis-full text-destructive">{displayError(error, t("history.actionError"))}</span>}
      </div>
    );
  }
  return (
    <p className="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
      {quoteLabel ?? t("marketData.noQuoteYet")} · {t("marketData.quotedAsOf", { time: formatTimestamp(quote.quotedAt, settings.data?.timezone, i18n.language) })}
      {Boolean(error) && <span role="alert" className="ml-2 text-destructive">{displayError(error, t("history.actionError"))}</span>}
    </p>
  );
}
