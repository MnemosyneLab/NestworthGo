/** Shared identity fields used to render an instrument in lists and sentences. */
export type InstrumentIdentityFields = {
  metalTemplate?: string;
  quantityUnit?: string;
  quoteCurrency?: string | null;
  isin?: string | null;
  marketCode?: string | null;
  shortName?: string | null;
  name?: string | null;
  symbol?: string | null;
};

/** Match Go Unicode whitespace plus BOM, including NEL omitted by JS trim. */
export function trimInstrumentText(value?: string | null): string {
 return value?.replace(/^[\s\u0085]+|[\s\u0085]+$/gu, "") ?? "";
}

/** Universal primary label: trimmed short name, symbol/code, then full name. */
export function instrumentDisplayLabel(instrument: InstrumentIdentityFields, fallback = ""): string {
  const shortName = trimInstrumentText(instrument.shortName);
  if (shortName) return shortName;
  const symbol = trimInstrumentText(instrument.symbol);
  if (symbol) {
    return symbol;
  }
  const name = trimInstrumentText(instrument.name);
  if (name) {
    return name;
  }
  return fallback;
}

/** Keep a distinct full name below the universal primary label. */
export function instrumentSecondaryName(instrument: InstrumentIdentityFields): string | undefined {
  const name = trimInstrumentText(instrument.name);
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

/** Selectors keep identity text alongside the universal primary label. Values remain IDs. */
export function instrumentSelectionLabel(instrument: InstrumentIdentityFields, fallback = ""): string {
  const primary = instrumentDisplayLabel(instrument, fallback);
  const identity = [instrument.symbol, instrument.name, instrument.marketCode, instrument.quoteCurrency, instrument.isin]
    .map(value => trimInstrumentText(value)).filter((value): value is string => Boolean(value));
  return [...new Set([primary, ...identity].filter(Boolean))].join(" · ");
}
