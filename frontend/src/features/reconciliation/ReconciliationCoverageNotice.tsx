import { useTranslation } from "react-i18next";
import type { ReconciliationAccount } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import { formatAmount } from "@/lib/money";

// Current inventory context is deliberately separate from frozen check evidence.
export function ReconciliationCoverageNotice({
  account,
  completed,
}: {
  account: ReconciliationAccount;
  completed: boolean;
}) {
  const { t } = useTranslation();
  if (!account.inventoryCoverageGap) return null;
  return (
    <div role="status" className="text-sm text-muted-foreground">
      <p>
        {t(
          `reconciliation.${completed ? "historicalCoverageGap" : "inventoryCoverageGap"}`,
        )}
      </p>
      {completed && (
        <>
          <ul>
            {account.uncoveredArchivedHoldings?.map((holding) => (
              <li key={holding.holdingId}>
                {holding.label} · {formatAmount(holding.quantity)}
              </li>
            ))}
          </ul>
          <p>{t("reconciliation.coverageNewCheck")}</p>
        </>
      )}
    </div>
  );
}
