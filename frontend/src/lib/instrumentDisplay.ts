/** Shared identity fields used to render an instrument in lists and sentences. */
export type InstrumentIdentityFields = {
  metalTemplate?: string;
  quantityUnit?: string;
  quoteCurrency?: string | null;
  providerKey?: string | null;
  providerSymbol?: string | null;
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

/** Detailed identity for discovery/context such as reservation sources. */
export function instrumentSelectionLabel(instrument: InstrumentIdentityFields, fallback = ""): string {
  const primary = instrumentDisplayLabel(instrument, fallback);
  const identity = [instrument.symbol, instrument.name, instrument.marketCode, instrument.quoteCurrency, instrument.isin, instrument.quantityUnit, instrument.providerKey, instrument.providerSymbol]
    .map(value => trimInstrumentText(value)).filter((value): value is string => Boolean(value));
  return [...new Set([primary, ...identity].filter(Boolean))].join(" · ");
}

/** Compact saved-instrument choices. Only exact collisions receive a discriminator. */
export function instrumentSelectionLabels<T extends { id: string } & InstrumentIdentityFields>(
  instruments: readonly T[], fallback = "",
): Map<string, string> {
  const groups = new Map<string, T[]>();
  for (const instrument of instruments) {
    const label = [...new Set([instrumentDisplayLabel(instrument, fallback), trimInstrumentText(instrument.symbol)].filter(Boolean))].join(" · ");
    const group = groups.get(label) ?? [];
    if (!group.some((item) => item.id === instrument.id)) group.push(instrument);
    groups.set(label, group);
  }
  const labels = new Map<string, string>();
  for (const [label, group] of groups) {
    if (group.length === 1) {
      labels.set(group[0].id, label);
      continue;
    }
    // Choose the shortest single field that distinguishes all IDs, never full names.
    const candidates = (["marketCode", "quoteCurrency", "quantityUnit", "providerSymbol", "providerKey", "isin"] as const)
      .map((field) => group.map((item) => trimInstrumentText(item[field])))
      .filter((values) => values.every(Boolean) && new Set(values).size === group.length)
      .sort((a, b) => a.reduce((sum, value) => sum + value.length, 0) - b.reduce((sum, value) => sum + value.length, 0));
    let discriminators = candidates[0];
    if (!discriminators) {
      let length = 1;
      while (new Set(group.map((item) => item.id.slice(0, length))).size < group.length) length++;
      discriminators = group.map((item) => `ID ${item.id.slice(0, length)}`);
    }
    group.forEach((item, index) => labels.set(item.id, `${label} [${discriminators[index]}]`));
  }
  // Include the full ID when a literal alias collides with a decorated choice.
  for (;;) {
    const counts = new Map<string, number>();
    for (const label of labels.values()) counts.set(label, (counts.get(label) ?? 0) + 1);
    const duplicates = [...labels].filter(([, label]) => (counts.get(label) ?? 0) > 1);
    if (!duplicates.length) break;
    for (const [id, label] of duplicates) labels.set(id, `${label} [ID ${id}]`);
  }
  return labels;
}
