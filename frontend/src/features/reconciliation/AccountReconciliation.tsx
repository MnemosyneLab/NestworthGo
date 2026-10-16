import { ReconciliationCoverageNotice } from "./ReconciliationCoverageNotice";
import { useTranslation } from "react-i18next";
import { useObjectNavigation } from "@/app/NavigationContext";
import { useReconciliationLastSuccess } from "@/queries/reconciliation";
import { Button } from "@/components/ui/button";
import { useReconciliationTimestamp } from "./useReconciliationTimestamp";

export function AccountReconciliation({ accountId, archived = false }: { accountId: string; archived?: boolean }) {
  const { t } = useTranslation();
  const formatTimestamp = useReconciliationTimestamp();
  const navigation = useObjectNavigation();
  const last = useReconciliationLastSuccess(accountId);
  const checkedAccount = last.data?.accounts?.find(
    (a) => a.accountId === accountId,
  );
  return (
    <section className="flex flex-wrap items-center gap-3 rounded-xl border border-border p-4">
      <p>
        {t("reconciliation.lastSuccess")}:{" "}
        {last.isError
          ? t("reconciliation.error")
          : last.isLoading
            ? t("reconciliation.loading")
            : last.data?.completedAt
              ? formatTimestamp(last.data.completedAt)
              : t("reconciliation.never")}
      </p>
      {checkedAccount && (
        <ReconciliationCoverageNotice account={checkedAccount} completed />
      )}
      {checkedAccount?.recheckRequired && (
        <p role="status" className="text-sm text-muted-foreground">
          {t("reconciliation.laterBookkeeping")}
        </p>
      )}
      {!archived && <Button
        variant="outline"
        onClick={() => navigation?.open({ page: "reconciliation", accountId })}
      >
        {t("reconciliation.open")}
      </Button>}
      {last.data && (
        <Button
          variant="ghost"
          onClick={() =>
            navigation?.open({
              page: "reconciliation",
              accountId,
              sessionId: last.data?.id,
            })
          }
        >
          {t("reconciliation.lastRecord")}
        </Button>
      )}
    </section>
  );
}
