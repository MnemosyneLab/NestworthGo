import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { useMarketDataHealth } from "@/queries/marketdata";

export function DataHealthIndicator({
  onOpen,
  testId = "data-health-indicator",
}: {
  onOpen?: () => void;
  testId?: string;
}) {
  const { t } = useTranslation();
  const health = useMarketDataHealth();
  if (!health.data) {
    return null;
  }
  const count = health.data.issueCount;
  const label = count === 0 ? t("dataHealth.indicatorHealthy") : t("dataHealth.indicatorIssues", { count });
  if (!onOpen) {
    return (
      <p className="text-sm text-muted-foreground" data-testid={testId}>
        {label}
      </p>
    );
  }
  return (
    <Button type="button" variant="outline" size="sm" onClick={onOpen} data-testid={testId}>
      {label}
    </Button>
  );
}
