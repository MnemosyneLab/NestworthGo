import i18n from "@/i18n/runtime";
import { displayPreferences, type DateFormat, type TimeFormat } from "@/lib/displayPreferences";

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

function hourCycle(timeFormat: TimeFormat): "h12" | "h23" {
  return timeFormat === "12h" ? "h12" : "h23";
}

function renderCivilDate(year: number, month: number, day: number, language: string, dateFormat: DateFormat): string {
  const monthText = String(month).padStart(2, "0");
  const dayText = String(day).padStart(2, "0");
  switch (dateFormat) {
    case "day-first":
      return `${dayText}/${monthText}/${year}`;
    case "month-first":
      return `${monthText}/${dayText}/${year}`;
    case "localized": {
      const date = new Date(Date.UTC(year, month - 1, day));
      return new Intl.DateTimeFormat(language, { dateStyle: "medium", timeZone: "UTC" }).format(date);
    }
    default:
      return `${year}-${monthText}-${dayText}`;
  }
}

function formatZonedDate(date: Date, timeZone: string, language: string, dateFormat: DateFormat): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(date);
  const values = Object.fromEntries(parts.filter((part) => part.type !== "literal").map((part) => [part.type, part.value]));
  if (!values.year || !values.month || !values.day) return "";
  return renderCivilDate(Number(values.year), Number(values.month), Number(values.day), language, dateFormat);
}

function formatClock(date: Date, timeZone: string, language: string, timeFormat: TimeFormat, seconds = false): string {
  return new Intl.DateTimeFormat(language, {
    timeZone,
    hour: "2-digit",
    minute: "2-digit",
    second: seconds ? "2-digit" : undefined,
    hourCycle: hourCycle(timeFormat),
  }).format(date).replace(/\u202f/g, " ");
}

export function formatTimestamp(value: string | number | Date, timezone?: string, language = i18n.language || "en"): string {
  const parsed = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return String(value);
  }
  const preferences = displayPreferences();
  const zone = resolvedTimeZone(timezone ?? preferences.timezone);
  try {
    if (preferences.dateFormat === "localized") {
      return new Intl.DateTimeFormat(language, {
        dateStyle: "medium",
        timeStyle: "short",
        timeZone: zone,
        hourCycle: hourCycle(preferences.timeFormat),
      }).format(parsed).replace(/\u202f/g, " ");
    }
    const dateText = formatZonedDate(parsed, zone, language, preferences.dateFormat);
    if (!dateText) return parsed.toISOString();
    return `${dateText} ${formatClock(parsed, zone, language, preferences.timeFormat)}`;
  } catch {
    return parsed.toISOString();
  }
}

/** Exact form timestamps must show the precision retained by the submitted command. */
export function formatExactTimestamp(value: string, timezone: string, language = i18n.language || "en"): string {
  const instant = new Date(value);
  if (Number.isNaN(instant.getTime())) return value;
  const preferences = displayPreferences();
  const zone = resolvedTimeZone(timezone);
  const milliseconds = String(instant.getUTCMilliseconds()).padStart(3, "0");
  try {
    const dateText = preferences.dateFormat === "localized"
      ? new Intl.DateTimeFormat(language, { dateStyle: "medium", timeZone: zone }).format(instant)
      : formatZonedDate(instant, zone, language, preferences.dateFormat);
    const clock = new Intl.DateTimeFormat(language, {
      timeZone: zone,
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hourCycle: hourCycle(preferences.timeFormat),
    }).formatToParts(instant).map((part) => part.type === "second" ? `${part.value}.${milliseconds}` : part.value).join("").replace(/\u202f/g, " ");
    return `${dateText} ${clock}`;
  } catch {
    return instant.toISOString();
  }
}

/** Inspect local seconds, including historical zones with second-based offsets. */
export function hasSubMinuteTime(value: string, timezone: string): boolean {
  const instant = new Date(value);
  // Zone offsets do not change the fractional second; local whole seconds can differ.
  return instant.getUTCMilliseconds() !== 0 || new Intl.DateTimeFormat("en", {
    timeZone: resolvedTimeZone(timezone), second: "2-digit",
  }).formatToParts(instant).some((part) => part.type === "second" && Number(part.value) !== 0);
}

/** Localize a civil date using the same date style as timestamps, without
 * interpreting it in the viewer's timezone or adding a time of day. */
export function formatCivilDate(value: string, language = i18n.language || "en", dateFormat = displayPreferences().dateFormat): string {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return value;
  const [year, month, day] = match.slice(1).map(Number);
  const date = new Date(0);
  date.setUTCFullYear(year, month - 1, day);
  if (year < 1 || date.getUTCFullYear() !== year || date.getUTCMonth() !== month - 1 || date.getUTCDate() !== day) return value;
  try {
    return renderCivilDate(year, month, day, language, dateFormat);
  } catch {
    return value;
  }
}

/** Date-labelled quotes have a source date, but no observed time of day. */
export function formatQuoteAsOf(
  quote: { quotedAt: string; effectiveDate?: string; timestampBasis?: string },
  timezone: string | undefined,
  language = i18n.language || "en",
): string {
  if (quote.timestampBasis === "date_label" && /^\d{4}-\d{2}-\d{2}$/.test(quote.effectiveDate ?? "")) {
    return formatCivilDate(quote.effectiveDate!, language);
  }
  if (/^\d{4}-\d{2}-\d{2}$/.test(quote.quotedAt)) return formatCivilDate(quote.quotedAt, language);
  return formatTimestamp(quote.quotedAt, timezone, language);
}

/** Formats a civil YYYY-MM-DD range such as "Sep 15 – Oct 14, 2026" without any timezone shift. */
export function formatCivilDateRange(from: string, to: string, language = i18n.language || "en"): string {
  const parse = (value: string) => {
    const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
    return match ? new Date(Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3]))) : undefined;
  };
  const start = parse(from), end = parse(to);
  if (!start || !end) return `${from} – ${to}`;
  if (displayPreferences().dateFormat !== "localized") {
    const formattedFrom = formatCivilDate(from, language);
    const formattedTo = formatCivilDate(to, language);
    return from === to ? formattedFrom : `${formattedFrom} – ${formattedTo}`;
  }
  try {
    const formatter = new Intl.DateTimeFormat(language, { year: "numeric", month: "short", day: "numeric", timeZone: "UTC" }) as Intl.DateTimeFormat & { formatRange(start: Date, end: Date): string };
    return from === to ? formatter.format(start) : formatter.formatRange(start, end);
  } catch {
    return `${from} – ${to}`;
  }
}
