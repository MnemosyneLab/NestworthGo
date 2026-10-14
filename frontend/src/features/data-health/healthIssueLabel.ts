import type { HealthIssueDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata/models";

type Translate = (key: string, options?: Record<string, unknown>) => string;

export function healthIssueLabel(t: Translate, issue: HealthIssueDTO, accountNames: Record<string, string>): string {
  const label = issue.label || (issue.currencyA ? [issue.currencyA, issue.currencyB].filter(Boolean).join(" / ") : undefined) || (issue.kind.startsWith("snapshot") ? t("dataHealth.snapshotTarget") : t("portfolio.unknownInstrument"));
  if (!(issue.kind.startsWith("snapshot") || issue.kind === "valuation_provisional") || !issue.accountId) return label;
  return `${accountNames[issue.accountId] ?? t("dataHealth.unknownAccount")} · ${label}`;
}
