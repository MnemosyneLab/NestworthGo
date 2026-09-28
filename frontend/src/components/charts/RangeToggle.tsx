import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";

export const TREND_RANGE_LABELS: Record<string, string> = {
  "1d": "rangeShortcuts.1d",
  "1w": "rangeShortcuts.1w",
  "1m": "rangeShortcuts.1m",
  "3m": "rangeShortcuts.3m",
  "6m": "rangeShortcuts.6m",
  "30d": "analytics.range30",
  ytd: "analytics.rangeYtd",
  "1y": "rangeShortcuts.1y",
  all: "analytics.rangeAll",
};

export const SOURCE_FILTER_LABELS: Record<string, string> = {
  all: "charts.sourceAll",
  manual: "charts.sourceManual",
  provider: "charts.sourceProvider",
};

export function RangeToggle({
  ranges,
  value,
  onChange,
  label,
  disabled = false,
}: {
  disabled?: boolean;
  ranges: readonly string[];
  value: string;
  onChange: (value: string) => void;
  label: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-1 rounded-lg bg-muted p-1" role="group" aria-label={label}>
      {ranges.map((id) => (
        <Button key={id} type="button" variant={value === id ? "default" : "ghost"} aria-pressed={value === id} disabled={disabled} size="sm" onClick={() => onChange(id)}>
          {t(TREND_RANGE_LABELS[id] ?? id)}
        </Button>
      ))}
    </div>
  );
}

export function SourceFilterToggle({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const filters = ["all", "manual", "provider"];
  return (
    <div className="flex flex-wrap items-center gap-2" role="group" aria-label={t("charts.sourceFilter")}>
      {filters.map((id) => (
        <Button key={id} type="button" variant={value === id ? "default" : "ghost"} aria-pressed={value === id} size="sm" onClick={() => onChange(id)}>
          {t(SOURCE_FILTER_LABELS[id] ?? id)}
        </Button>
      ))}
    </div>
  );
}
