import { ReconciliationCoverageNotice } from "./ReconciliationCoverageNotice";
import { useTranslation } from "react-i18next";
import { useObjectNavigation } from "@/app/NavigationContext";
import { useReconciliationLastSuccess } from "@/queries/reconciliation";
import { Button } from "@/components/ui/button";
import { useReconciliationTimestamp } from "./useReconciliationTimestamp";
import { Notice } from "./ReconciliationParts";

export function AccountReconciliation({ accountId, archived = false }: { accountId: string; archived?: boolean }) {
  const { t } = useTranslation();
  const formatTimestamp = useReconciliationTimestamp();
  const navigation = useObjectNavigation();
  const last = useReconciliationLastSuccess(accountId);
  const checkedAccount = last.data?.accounts?.find(
    (a) => a.accountId === accountId,
  );
  return (
    <section className="flex min-w-0 flex-col gap-3 rounded-2xl border border-border bg-card p-5 shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="num min-w-0 break-words text-sm font-medium">
          {t("reconciliation.lastSuccess")}:{" "}
          {last.isError
            ? t("reconciliation.error")
            : last.isLoading
              ? t("reconciliation.loading")
              : last.data?.completedAt
                ? formatTimestamp(last.data.completedAt)
                : t("reconciliation.never")}
        </p>
        <div className="flex flex-wrap items-center gap-2">
          {!archived && (
            <Button
              variant="outline"
              onClick={() =>
                navigation?.open({ page: "reconciliation", accountId })
              }
            >
              {t("reconciliation.open")}
            </Button>
          )}
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
        </div>
      </div>
      {checkedAccount && (
        <ReconciliationCoverageNotice account={checkedAccount} completed />
      )}
      {checkedAccount?.recheckRequired && (
        <Notice role="status">{t("reconciliation.laterBookkeeping")}</Notice>
      )}
    </section>
  );
}
