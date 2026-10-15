import { useTranslation } from "react-i18next";
import { useObjectNavigation } from "@/app/NavigationContext";
import { useReconciliationLastSuccess } from "@/queries/reconciliation";
import { Button } from "@/components/ui/button";
import { formatTimestamp as timestamp } from "@/lib/time";
const formatTimestamp = (value: string) => timestamp(value, undefined);
export function AccountReconciliation({ accountId }: { accountId: string }) {
  const { t } = useTranslation();
  const navigation = useObjectNavigation();
  const last = useReconciliationLastSuccess(accountId);
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
      <Button
        variant="outline"
        onClick={() => navigation?.open({ page: "reconciliation", accountId })}
      >
        {t("reconciliation.open")}
      </Button>
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
          {t("reconciliation.history")}
        </Button>
      )}
    </section>
  );
}
