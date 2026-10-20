const reasonKeys: Record<string, string> = {
  "one or more asset days are incomplete": "assetDaysIncomplete",
  "one or more analysis days are incomplete": "analysisDaysIncomplete",
  "daily return is incomplete": "dailyReturnIncomplete",
  "group has no complete rated days": "groupIncomplete",
  "missing quote": "missingQuote",
  "no usable asset days are available for this period": "noAssetDays",
  "no complete analysis inputs are available for this period": "noCompleteInputs",
  "instrument price is unavailable": "instrumentPriceUnavailable",
  "an FX rate is unavailable for the activity": "fxRateUnavailable",
  "foreign-exchange rate is unavailable": "fxRateUnavailable",
  "missing account value or FX rate": "accountValueFxUnavailable",
  "missing instrument price or FX rate": "instrumentPriceOrFxUnavailable",
  "holding quote is unavailable": "holdingQuoteUnavailable",
  "current instrument price is unavailable": "currentInstrumentPriceUnavailable",
  "current foreign-exchange rate is unavailable": "currentFXUnavailable",
  "dividend income foreign-exchange rate is unavailable": "dividendFXUnavailable",
  "realized gain foreign-exchange rate is unavailable": "realizedFXUnavailable",
  "rate coverage is partial": "rateCoveragePartial",
  "one or more return days are incomplete": "returnDaysIncomplete",
  "return days are unavailable": "returnDaysUnavailable",
  "gain is unavailable": "gainUnavailable",
  "valuation available; return pending": "valuationAvailableReturnPending",
  "funding cash is recorded after the product start": "fundingCashAfterProductStart",
};

/** Translate stable analytics sentences. Unknown machine codes use a translated fallback. */
export function analysisReason(t: (key: string) => string, reason?: string | null): string | undefined {
  if (!reason) return undefined;
  const normalized = reason.trim().toLowerCase();
  const key = reasonKeys[normalized];
  if (key) return t(`insights.analysisReasons.${key}`);
  if (/^[a-z0-9]+(?:_[a-z0-9]+)+$/.test(normalized)) return t("common.unknownValue");
  return reason;
}

export function analysisCashLabel(t: (key: string) => string, key: string): string | undefined {
  if (key === "cash") return t("insights.cash");
  if (/^cash:[A-Z]{3}$/.test(key)) return `${t("insights.cash")} · ${key.slice(5)}`;
  return undefined;
}
