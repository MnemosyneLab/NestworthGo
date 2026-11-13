const iso = (date: Date) => date.toISOString().slice(0, 10);

/** Match the read model's civil day, Monday, or calendar month keys. */
export function assetTrendPeriods(from: string, to: string, granularity: string) {
  if (!from || !to) return [];
  const start = Date.parse(from);
  const end = Date.parse(to);
  if (!Number.isFinite(start) || !Number.isFinite(end) || start > end) return [];
  const date = new Date(start);
  if (granularity === "month") date.setUTCDate(1);
  else if (granularity === "week") date.setUTCDate(date.getUTCDate() - (date.getUTCDay() + 6) % 7);
  const periods: { key: string; date: string; start: number; end: number }[] = [];
  while (date.getTime() <= end) {
    const periodStart = date.getTime();
    const key = granularity === "month" ? iso(date).slice(0, 7) : iso(date);
    if (granularity === "month") date.setUTCMonth(date.getUTCMonth() + 1);
    else date.setUTCDate(date.getUTCDate() + (granularity === "week" ? 7 : 1));
    periods.push({ key, date: iso(new Date(Math.max(start, periodStart))), start: Math.max(start, periodStart), end: Math.min(end + 86_399_999, date.getTime()) });
  }
  return periods;
}

/** Merge adjacent empty buckets so long gaps need just one shaded interval. */
export function assetTrendGaps(periods: ReturnType<typeof assetTrendPeriods>, values: Array<string | null | undefined>) {
  const gaps: Array<[number, number]> = [];
  periods.forEach(({ start, end }, index) => {
    if (values[index] != null) return;
    const previous = gaps[gaps.length - 1];
    if (previous?.[1] === start) previous[1] = end;
    else gaps.push([start, end]);
  });
  return gaps;
}
