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
};

/** Translate stable analytics reason strings while keeping unknown diagnostics visible. */
export function analysisReason(t: (key: string) => string, reason?: string | null): string | undefined {
  if (!reason) return undefined;
  const key = reasonKeys[reason.trim().toLowerCase()];
  return key ? t(`insights.analysisReasons.${key}`) : reason;
}
