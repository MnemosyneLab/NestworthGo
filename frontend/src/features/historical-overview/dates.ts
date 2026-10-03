// A date label is civil data, not midnight in the browser's timezone.
// Keep label arithmetic in UTC; Go owns the household's actual day boundary.
export function parseCivilDate(value: string): Date | null {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return null;
  const date = new Date(`${value}T00:00:00Z`);
  if (!Number.isFinite(date.getTime()) || date.toISOString().slice(0, 10) !== value) return null;
  return date;
}

export function shiftCivilDate(value: string, days: number): string {
  const date = parseCivilDate(value);
  if (!date) return "";
  date.setUTCDate(date.getUTCDate() + days);
  return date.toISOString().slice(0, 10);
}

export function previousMonthEnd(value: string): string {
  const date = parseCivilDate(value);
  if (!date) return "";
  date.setUTCDate(0);
  return date.toISOString().slice(0, 10);
}

export function isClosedDate(value: string, first: string, last: string): boolean {
  return Boolean(parseCivilDate(value) && first && last && value >= first && value <= last);
}
