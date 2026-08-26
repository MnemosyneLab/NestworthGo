type Translator = (key: string, options?: Record<string, unknown>) => string;

export function humanizeEnum(value: string): string {
  return value
    .replace(/[_-]+/g, " ")
    .replace(/\b\w/g, (character) => character.toUpperCase());
}

export function displayEnum(t: Translator, namespace: string, value: string | null | undefined): string {
  if (!value) {
    return "";
  }
  const candidates = [value, value.trim().toLowerCase().replace(/[\s-]+/g, "_")];
  for (const candidate of candidates) {
    const translated = t(`${namespace}.${candidate}`, { defaultValue: "" });
    if (translated) {
      return translated;
    }
  }
  return humanizeEnum(value);
}

export function displayError(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}
