/** Shared identity fields used to render an instrument in lists and sentences. */
export type InstrumentIdentityFields = {
  metalTemplate?: string;
  quantityUnit?: string;
  quoteCurrency?: string | null;
  name?: string | null;
  symbol?: string | null;
};

/** Ticker when present, otherwise the saved name. Compact selects and
 * activity sentences use this so long ETF names do not dominate the UI. */
export function instrumentDisplayLabel(instrument: InstrumentIdentityFields, fallback = ""): string {
  if (instrument.metalTemplate) {
    return [instrument.name || fallback, `${instrument.quoteCurrency ?? ""}/${instrument.quantityUnit === "g" ? "g" : "oz t"}`].join(" · ");
  }
  const symbol = instrument.symbol?.trim();
  if (symbol) {
    return symbol;
  }
  const name = instrument.name?.trim();
  if (name) {
    return name;
  }
  return fallback;
}

/** Full name when it adds information beyond the ticker. Identical ticker/name
 * pairs, and instruments with no ticker, omit the subtitle. */
export function instrumentSecondaryName(instrument: InstrumentIdentityFields): string | undefined {
  const name = instrument.name?.trim();
  if (!name) {
    return undefined;
  }
  const primary = instrumentDisplayLabel(instrument);
  if (name.localeCompare(primary, undefined, { sensitivity: "accent" }) === 0) {
    return undefined;
  }
  return name;
}

export function instrumentDisplayLabels<T extends { id: string } & InstrumentIdentityFields>(
  instruments: readonly T[],
  fallback = "",
): Map<string, string> {
  return new Map(instruments.map((instrument) => [instrument.id, instrumentDisplayLabel(instrument, fallback)]));
}
