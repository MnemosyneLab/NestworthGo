type PlainObject = Record<string, unknown>;

function isPlainObject(value: unknown): value is PlainObject {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/** Recursively merges `override` into `base`, without mutating either. */
export function deepMerge<T extends PlainObject>(base: T, override: PlainObject): T {
  const result: PlainObject = { ...base };
  for (const key of Object.keys(override)) {
    const overrideValue = override[key];
    const baseValue = result[key];
    result[key] =
      isPlainObject(overrideValue) && isPlainObject(baseValue) ? deepMerge(baseValue, overrideValue) : overrideValue;
  }
  return result as T;
}
