import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";

export const TREND_RANGE_LABELS: Record<string, string> = {
  "30d": "analytics.range30",
  "1y": "analytics.range1year",
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
}: {
  ranges: readonly string[];
  value: string;
  onChange: (value: string) => void;
  label: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-2" role="group" aria-label={label}>
      {ranges.map((id) => (
        <Button key={id} type="button" variant={value === id ? "default" : "outline"} size="sm" onClick={() => onChange(id)}>
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
        <Button key={id} type="button" variant={value === id ? "default" : "outline"} size="sm" onClick={() => onChange(id)}>
          {t(SOURCE_FILTER_LABELS[id] ?? id)}
        </Button>
      ))}
    </div>
  );
}
