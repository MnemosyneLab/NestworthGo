/**
 * localDateInTimeZone is the History start date Go will persist: clock
 * now, formatted as YYYY-MM-DD in the confirmed IANA timezone. Invalid
 * zones return undefined so the form does not invent a date.
 */
export function localDateInTimeZone(timeZone: string, now = new Date()): string | undefined {
  const trimmed = timeZone.trim();
  if (!trimmed) {
    return undefined;
  }
  try {
    const formatted = new Intl.DateTimeFormat("en-CA", {
      timeZone: trimmed,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    }).format(now);
    const normalized = formatted.replace(/\//g, "-");
    return /^\d{4}-\d{2}-\d{2}$/.test(normalized) ? normalized : undefined;
  } catch {
    return undefined;
  }
}
