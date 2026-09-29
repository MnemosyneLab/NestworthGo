import * as React from "react";

export function cssColor(variable: string, fallback: string): string {
  if (typeof document === "undefined") {
    return fallback;
  }
  const value = getComputedStyle(document.documentElement).getPropertyValue(variable).trim();
  return value || fallback;
}

export const THEME_CHANGE_EVENT = "nestworth:theme-change";

function subscribeThemeChange(callback: () => void): () => void {
  window.addEventListener(THEME_CHANGE_EVENT, callback);
  return () => window.removeEventListener(THEME_CHANGE_EVENT, callback);
}

let themeVersion = 0;
function getThemeVersion(): number {
  return themeVersion;
}

/** Bumps the shared theme version. Called by useTheme after the theme class or accent changes. */
export function notifyThemeChange(): void {
  themeVersion += 1;
  if (typeof window !== "undefined") {
    window.dispatchEvent(new Event(THEME_CHANGE_EVENT));
  }
}

/**
 * Re-renders the calling component whenever the appearance or accent changes,
 * so chart options that resolve CSS variables at render time stay in sync.
 */
export function useThemeVersion(): number {
  return React.useSyncExternalStore(subscribeThemeChange, getThemeVersion, getThemeVersion);
}

/** Resolves a `var(--token)` series color to its current computed value; other strings pass through. */
export function resolveSeriesColor(color: string): string {
  const match = /^var\((--[\w-]+)\)$/.exec(color);
  return match ? cssColor(match[1], color) : color;
}

export function chartTheme() {
  return {
    foreground: cssColor("--color-foreground", "#2a3a3d"),
    muted: cssColor("--color-muted-foreground", "#5f7276"),
    border: cssColor("--color-border", "#d9e6e5"),
    primary: cssColor("--color-primary", "#1f8a80"),
    accent: cssColor("--color-accent", "#f0a04b"),
    success: cssColor("--color-success", "#2f9e6a"),
    destructive: cssColor("--color-destructive", "#e0523f"),
    warning: cssColor("--color-warning", "#e8b83a"),
    gainPositive: cssColor("--color-gain-positive", "#2f9e6a"),
    gainNegative: cssColor("--color-gain-negative", "#e0523f"),
    palette: [
      cssColor("--color-brand-from", "#2fa596"),
      cssColor("--color-cat-1", "#4f8ff0"),
      cssColor("--color-cat-6", "#f0a04b"),
      cssColor("--color-cat-5", "#9a7be0"),
      cssColor("--color-cat-7", "#e0559f"),
      cssColor("--color-cat-4", "#f0d040"),
      cssColor("--color-cat-8", "#4aa0b8"),
      cssColor("--color-cat-3", "#f0665a"),
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
