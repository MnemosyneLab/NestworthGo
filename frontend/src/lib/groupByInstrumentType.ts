/** Matches domain.AllInstrumentTypes order. */
export const INSTRUMENT_TYPE_ORDER = [
  "stock",
  "etf",
  "mutual_fund",
  "crypto",
  "bond",
  "precious_metal",
  "bank_investment_product",
  "other",
] as const;

export type TypedInstrument = {
  type?: string | null;
  quoteCurrency?: string | null;
  name: string;
};

export type InstrumentTypeGroup<T> = {
  key: string;
  items: T[];
};

export type FxPairLike = {
  currencyA: string;
  currencyB: string;
};

function instrumentTypeKey(type: string | null | undefined): string {
  const trimmed = type?.trim();
  return trimmed || "other";
}

function compareByCurrencyThenName(left: TypedInstrument, right: TypedInstrument): number {
  const currency = (left.quoteCurrency ?? "").localeCompare(right.quoteCurrency ?? "");
  if (currency !== 0) {
    return currency;
  }
  return left.name.localeCompare(right.name);
}

/**
 * Groups instruments by type using catalog/domain order. Unknown types
 * follow known types alphabetically. Within a type, sort by quote
 * currency then name.
 */
export function groupByInstrumentType<T extends TypedInstrument>(
  items: readonly T[],
  typeOrder: readonly string[] = INSTRUMENT_TYPE_ORDER,
): InstrumentTypeGroup<T>[] {
  const buckets = new Map<string, T[]>();
  for (const item of items) {
    const key = instrumentTypeKey(item.type);
    const existing = buckets.get(key);
    if (existing) {
      existing.push(item);
    } else {
      buckets.set(key, [item]);
    }
  }
  const orderIndex = new Map(typeOrder.map((type, index) => [type, index]));
  return [...buckets.keys()]
    .sort((left, right) => {
      const leftIndex = orderIndex.get(left);
      const rightIndex = orderIndex.get(right);
      if (leftIndex !== undefined && rightIndex !== undefined) {
        return leftIndex - rightIndex;
      }
      if (leftIndex !== undefined) {
        return -1;
      }
      if (rightIndex !== undefined) {
        return 1;
      }
      return left.localeCompare(right);
    })
    .map((key) => ({
      key,
      items: [...(buckets.get(key) ?? [])].sort(compareByCurrencyThenName),
    }));
}

export function sortFxPairs<T extends FxPairLike>(pairs: readonly T[]): T[] {
  return [...pairs].sort((left, right) => {
    const first = left.currencyA.localeCompare(right.currencyA);
    if (first !== 0) {
      return first;
    }
    return left.currencyB.localeCompare(right.currencyB);
  });
}
