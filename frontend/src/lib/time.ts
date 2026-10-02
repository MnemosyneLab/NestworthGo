import i18n from "@/i18n";

export function systemTimeZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
}

export function resolvedTimeZone(timezone: string | undefined): string {
  const value = timezone?.trim();
  return !value || value === "system" ? systemTimeZone() : value;
}

export function timeZoneOptions(): string[] {
  const supportedValuesOf = (Intl as typeof Intl & { supportedValuesOf?: (key: string) => string[] }).supportedValuesOf;
  const values = supportedValuesOf ? supportedValuesOf("timeZone") : [];
  return [...new Set(["UTC", ...values])].sort((left, right) => left.localeCompare(right));
}

export function localDateTimeInTimeZone(timezone: string, now = new Date()): { date: string; time: string } | undefined {
  const zone = resolvedTimeZone(timezone);
  try {
    const parts = new Intl.DateTimeFormat("en-CA", {
      timeZone: zone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
    }).formatToParts(now);
    const values = Object.fromEntries(parts.filter((part) => part.type !== "literal").map((part) => [part.type, part.value]));
    if (!values.year || !values.month || !values.day || !values.hour || !values.minute) {
      return undefined;
    }
    return { date: `${values.year}-${values.month}-${values.day}`, time: `${values.hour}:${values.minute}` };
  } catch {
    return undefined;
  }
}

export function formatTimestamp(value: string | number | Date, timezone: string | undefined, language = i18n.language || "en"): string {
  const parsed = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return String(value);
  }
  try {
    return new Intl.DateTimeFormat(language, {
      dateStyle: "medium",
      timeStyle: "short",
      timeZone: resolvedTimeZone(timezone),
    }).format(parsed);
  } catch {
    return parsed.toISOString();
  }
}

/** Exact form timestamps must show the precision retained by the submitted command. */
export function formatExactTimestamp(value: string, timezone: string, language = i18n.language || "en"): string {
  const instant = new Date(value);
  const milliseconds = String(instant.getUTCMilliseconds()).padStart(3, "0");
  return new Intl.DateTimeFormat(language, {
    timeZone: resolvedTimeZone(timezone), year: "numeric", month: "short", day: "2-digit",
    hour: "2-digit", minute: "2-digit", second: "2-digit", hourCycle: "h23",
  }).formatToParts(instant).map((part) => part.type === "second" ? `${part.value}.${milliseconds}` : part.value).join("");
}

/** Inspect local seconds, including historical zones with second-based offsets. */
export function hasSubMinuteTime(value: string, timezone: string): boolean {
  const instant = new Date(value);
  // Zone offsets do not change the fractional second; local whole seconds can differ.
  return instant.getUTCMilliseconds() !== 0 || new Intl.DateTimeFormat("en", {
    timeZone: resolvedTimeZone(timezone), second: "2-digit",
  }).formatToParts(instant).some((part) => part.type === "second" && Number(part.value) !== 0);
}

/** Date-labelled quotes have a source date, but no observed time of day. */
export function formatQuoteAsOf(
  quote: { quotedAt: string; effectiveDate?: string; timestampBasis?: string },
  timezone: string | undefined,
  language = i18n.language || "en",
): string {
  if (quote.timestampBasis === "date_label" && /^\d{4}-\d{2}-\d{2}$/.test(quote.effectiveDate ?? "")) {
    return quote.effectiveDate!;
  }
  return formatTimestamp(quote.quotedAt, timezone, language);
}
