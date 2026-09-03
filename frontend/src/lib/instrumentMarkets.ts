/** Catalog trading-market codes mapped to the country/region they belong to.
 * BSE is Beijing Stock Exchange, alongside SSE and SZSE. */
export const INSTRUMENT_MARKET_COUNTRIES: Record<string, string> = {
  ASX: "AU",
  SSE: "CN",
  SZSE: "CN",
  BSE: "CN",
  EURONEXT: "EU",
  XETRA: "EU",
  LSE: "GB",
  HKEX: "HK",
  TSE: "JP",
  SGX: "SG",
  TWSE: "TW",
  NASDAQ: "US",
  NYSE: "US",
  AMEX: "US",
  KRX: "KR",
  SIX: "CH",
};

export function countryForMarket(marketCode: string): string | undefined {
  return INSTRUMENT_MARKET_COUNTRIES[marketCode];
}

/** Markets shown for a country. An already-selected code stays visible even
 * when it is outside the filtered set, so edit forms do not hide saved values. */
export function marketsForCountry(
  marketCodes: readonly string[],
  countryCode: string,
  selectedMarket = "",
): string[] {
  if (!countryCode) {
    return [...marketCodes];
  }
  return marketCodes.filter((code) => INSTRUMENT_MARKET_COUNTRIES[code] === countryCode || code === selectedMarket);
}
