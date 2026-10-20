// Presentation preferences mirrored from stored settings. Formatters read
// this module so every page shares one date, time, and number style without
// recomputing or persisting financial values.

export type DateFormat = "iso" | "day-first" | "month-first" | "localized";
export type TimeFormat = "24h" | "12h";

export type DisplayPreferences = {
  dateFormat: DateFormat;
  timeFormat: TimeFormat;
  timezone: string;
  decimalSeparator: "." | ",";
  groupingSeparator: string;
};

const defaults: DisplayPreferences = {
  dateFormat: "iso",
  timeFormat: "24h",
  timezone: "system",
  decimalSeparator: ".",
  groupingSeparator: ",",
};

let current: DisplayPreferences = { ...defaults };

export function displayPreferences(): DisplayPreferences {
  return current;
}

export function setDisplayPreferences(next: Partial<DisplayPreferences>): void {
  current = { ...current, ...next };
}

export function resetDisplayPreferences(): void {
  current = { ...defaults };
}

function isDateFormat(value: string | undefined): value is DateFormat {
  return value === "iso" || value === "day-first" || value === "month-first" || value === "localized";
}

function isTimeFormat(value: string | undefined): value is TimeFormat {
  return value === "24h" || value === "12h";
}

/** Apply the fields the shared formatters honor. Decimal places stay on the currency scale. */
export function applySettingsDisplayPreferences(settings: {
  dateFormat?: string;
  timeFormat?: string;
  timezone?: string;
  decimalSeparator?: string;
  groupingSeparator?: string;
} | null | undefined): void {
  if (!settings) return;
  const next: Partial<DisplayPreferences> = {};
  if (isDateFormat(settings.dateFormat)) next.dateFormat = settings.dateFormat;
  if (isTimeFormat(settings.timeFormat)) next.timeFormat = settings.timeFormat;
  if (settings.timezone) next.timezone = settings.timezone;
  if (settings.decimalSeparator === "." || settings.decimalSeparator === ",") next.decimalSeparator = settings.decimalSeparator;
  if (settings.groupingSeparator) next.groupingSeparator = settings.groupingSeparator;
  setDisplayPreferences(next);
}

/** Settings store "space" and "none"; money rendering needs the actual separator. */
export function groupingSeparatorChar(value = displayPreferences().groupingSeparator): string {
  if (value === "none") return "";
  if (value === "space") return " ";
  return value;
}
