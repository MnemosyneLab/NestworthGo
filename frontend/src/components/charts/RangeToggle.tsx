import { useTranslation } from "react-i18next";
import { SegmentedControl } from "@/components/ui/segmented-control";

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
    <SegmentedControl
      label={label}
      value={value}
      onChange={onChange}
      disabled={disabled}
      options={ranges.map((id) => ({ value: id, label: t(TREND_RANGE_LABELS[id] ?? id) }))}
    />
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
    <SegmentedControl
      label={t("charts.sourceFilter")}
      value={value}
      onChange={onChange}
      options={filters.map((id) => ({ value: id, label: t(SOURCE_FILTER_LABELS[id] ?? id) }))}
    />
  );
}
