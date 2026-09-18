import { useTranslation } from "react-i18next";
import { formatAmount } from "@/lib/money";
import { formatTimestamp } from "@/lib/time";
import { useSettings } from "@/queries/settings";

export function MetalConversionDetails({ evidence }: { evidence?: string }) {
  const { t, i18n } = useTranslation();
  const settings = useSettings();
  if (!evidence) return null;
  let value: Record<string, unknown>;
  try {
    const parsed: unknown = JSON.parse(evidence);
    if (!parsed || typeof parsed !== "object") return null;
    value = parsed as Record<string, unknown>;
  } catch { return null; }
  if (typeof value.rawPrice !== "string" || typeof value.fxRate !== "string" || typeof value.currency !== "string") return null;
  return <details className="text-xs text-muted-foreground">
    <summary className="cursor-pointer">{t("metals.conversion")}</summary>
    <div className="mt-1 flex flex-col gap-1">
      <span>{t("metals.original")}: {formatAmount(value.rawPrice, "USD")} / oz t</span>
      <span>{t("metals.exchangeRate")}: 1 USD = {value.fxRate} {value.currency}</span>
      {typeof value.rawQuotedAt === "string" && <span>{t("metals.rawTime")}: {formatTimestamp(value.rawQuotedAt, settings.data?.timezone, i18n.language)}</span>}
      {typeof value.fxQuotedAt === "string" && <span>{t("metals.fxTime")}: {formatTimestamp(value.fxQuotedAt, settings.data?.timezone, i18n.language)}</span>}
    </div>
  </details>;
}
