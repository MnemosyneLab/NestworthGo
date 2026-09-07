export function ymd(date: Date): string {
  return [date.getFullYear(), String(date.getMonth() + 1).padStart(2, "0"), String(date.getDate()).padStart(2, "0")].join("-");
}

export function ymdInTimeZone(date: Date, timeZone?: string): string {
  if (!timeZone) return ymd(date);
  const parts = new Intl.DateTimeFormat("en-CA", { timeZone, year: "numeric", month: "2-digit", day: "2-digit" }).formatToParts(date);
  const values = Object.fromEntries(parts.filter((part) => part.type !== "literal").map((part) => [part.type, part.value]));
  return `${values.year}-${values.month}-${values.day}`;
}

export function parseYmd(value: string): Date {
  const [year, month, day] = value.split("-").map(Number);
  return new Date(year, month - 1, day);
}

export function currentMonth(timeZone?: string): string {
  return ymdInTimeZone(new Date(), timeZone).slice(0, 7);
}

/** AnalysisService accepts closed local dates only; the current day remains a
 * muted calendar cell instead of being sent as an open-ended query date. */
export function lastClosedDate(timeZone?: string): string {
  const value = parseYmd(ymdInTimeZone(new Date(), timeZone));
  value.setDate(value.getDate() - 1);
  return ymd(value);
}

export function monthStart(month: string): Date {
  const [year, value] = month.split("-").map(Number);
  return new Date(year, value - 1, 1);
}

export function monthEnd(month: string): Date {
  const value = monthStart(month);
  return new Date(value.getFullYear(), value.getMonth() + 1, 0);
}

export function addMonths(month: string, amount: number): string {
  const value = monthStart(month);
  value.setMonth(value.getMonth() + amount);
  return `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, "0")}`;
}

export function monthLabel(month: string, locale: string, short = false): string {
  return monthStart(month).toLocaleDateString(locale, { month: short ? "short" : "long", year: "numeric" });
}

export function yearLabel(year: number, locale: string): string {
  return new Date(year, 0, 1).toLocaleDateString(locale, { year: "numeric" });
}

export function monthDays(month: string, weekStartsOn = 1): Array<{ date: string; inMonth: boolean }> {
  const first = monthStart(month);
  const last = monthEnd(month);
  const leading = (first.getDay() - weekStartsOn + 7) % 7;
  const total = leading + last.getDate();
  const cells = Math.ceil(total / 7) * 7;
  const result: Array<{ date: string; inMonth: boolean }> = [];
  for (let index = 0; index < cells; index += 1) {
    const date = new Date(first);
    date.setDate(index - leading + 1);
    result.push({ date: ymd(date), inMonth: date.getMonth() === first.getMonth() });
  }
  return result;
}

export function yearMonths(year: number): string[] {
  return Array.from({ length: 12 }, (_, index) => `${year}-${String(index + 1).padStart(2, "0")}`);
}

export function isToday(date: string, timeZone?: string): boolean {
  return date === ymdInTimeZone(new Date(), timeZone);
}

export function isFuture(date: string, timeZone?: string): boolean {
  return date > ymdInTimeZone(new Date(), timeZone);
}

export function isCurrentYear(month: string): boolean {
  return Number(month.slice(0, 4)) === new Date().getFullYear();
}
