export function cssColor(variable: string, fallback: string): string {
  if (typeof document === "undefined") {
    return fallback;
  }
  const value = getComputedStyle(document.documentElement).getPropertyValue(variable).trim();
  return value || fallback;
}

export function chartTheme() {
  return {
    foreground: cssColor("--color-foreground", "#2a2f3a"),
    muted: cssColor("--color-muted-foreground", "#6b7280"),
    border: cssColor("--color-border", "#d7dbe7"),
    primary: cssColor("--color-primary", "#5b5bd6"),
    accent: cssColor("--color-accent", "#3aa8b8"),
    success: cssColor("--color-success", "#2f9e5c"),
    destructive: cssColor("--color-destructive", "#c4473a"),
    warning: cssColor("--color-warning", "#c9a227"),
    gainPositive: cssColor("--color-gain-positive", "#2f9e5c"),
    gainNegative: cssColor("--color-gain-negative", "#c4473a"),
    palette: [
      cssColor("--color-primary", "#5b5bd6"),
      cssColor("--color-accent", "#3aa8b8"),
      cssColor("--color-success", "#2f9e5c"),
      cssColor("--color-brand-from", "#5d5de0"),
      cssColor("--color-brand-to", "#8a5fd6"),
      cssColor("--color-data-remote", "#4d8fb5"),
      cssColor("--color-data-manual", "#6b5bd6"),
      cssColor("--color-warning", "#c9a227"),
    ],
  };
}

export function prefersReducedMotion(): boolean {
  return typeof window !== "undefined" && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

/** Escapes dynamic labels before they enter an ECharts HTML tooltip. */
export function escapeChartText(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

/** Joins escaped tooltip lines for ECharts HTML mode. */
export function joinTooltipLines(lines: Array<string | null | undefined>): string {
  return lines
    .filter((line): line is string => Boolean(line))
    .map(escapeChartText)
    .join("<br/>");
}

/** Converts a canonical decimal string to a finite Number for ECharts layout only. */
export function chartNumber(value: string | null | undefined): number | null {
  if (value == null || value === "") {
    return null;
  }
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}
