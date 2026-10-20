import { useTranslation } from "react-i18next";
import type { ReconciliationAccount } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import { formatAmount } from "@/lib/money";
import { Notice } from "./ReconciliationParts";

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
    <Notice tone="warning" role="status">
      <p>
        {t(
          `reconciliation.${completed ? "historicalCoverageGap" : "inventoryCoverageGap"}`,
        )}
      </p>
      {completed && (
        <>
          <ul className="num mt-1 list-disc pl-5">
            {account.uncoveredArchivedHoldings?.map((holding) => (
              <li key={holding.holdingId}>
                {holding.label} · {formatAmount(holding.quantity)}
              </li>
            ))}
          </ul>
          <p className="mt-1">{t("reconciliation.coverageNewCheck")}</p>
        </>
      )}
    </Notice>
  );
}
