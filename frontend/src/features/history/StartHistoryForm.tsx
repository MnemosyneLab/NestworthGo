import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { LoadingState } from "@/components/layout/PageState";
import { displayError } from "@/lib/display";
import { formatAmount } from "@/lib/money";
import { useStartHistory, useStartingPointDraft } from "@/queries/history";
import { localDateInTimeZone } from "@/features/history/historyStartDate";

/** StartHistoryForm is the existing Start History flow, reused from History
 * and from Account detail when an event action needs History first. Cancel
 * writes nothing. */
export function StartHistoryForm({
  onStarted,
  onCancel,
  compact = false,
}: {
  onStarted?: () => void;
  onCancel?: () => void;
  compact?: boolean;
}) {
  const { t } = useTranslation();
  const startHistory = useStartHistory();
  const draft = useStartingPointDraft();
  const [timezone, setTimezone] = useState(() => Intl.DateTimeFormat().resolvedOptions().timeZone);
  const startDate = localDateInTimeZone(timezone);
  const holdings = draft.data ?? [];
  const [unitCosts, setUnitCosts] = useState<Record<string, string>>({});

  const costFor = (holdingId: string, fallback?: string) =>
    unitCosts[holdingId] !== undefined ? unitCosts[holdingId] : (fallback ?? "");
  const missingCost = holdings.some((holding) => !costFor(holding.holdingId, holding.unitCost).trim());

  const submit = () => {
    if (missingCost) {
      return;
    }
    const costOverrides =
      holdings.length === 0
        ? undefined
        : Object.fromEntries(holdings.map((holding) => [holding.holdingId, costFor(holding.holdingId, holding.unitCost).trim()]));
    startHistory.mutate(
      { timezone, costOverrides },
      {
        onSuccess: () => {
          toast.success(t("history.started"));
          onStarted?.();
        },
      },
    );
  };

  return (
    <div className={compact ? "flex flex-col gap-4" : "mx-auto flex w-full max-w-xl flex-col gap-5 rounded-lg border border-border bg-card p-6 text-center sm:p-8"}>
      <div className="flex flex-col gap-2">
        <h2 className={compact ? "text-base font-semibold text-foreground" : "text-lg font-semibold text-foreground"}>
          {t("history.startTitle")}
        </h2>
        <p className="text-sm leading-6 text-muted-foreground">{t("history.startDescription")}</p>
      </div>
      <div className="flex flex-col gap-1.5 text-left">
        <Label htmlFor="history-timezone">{t("history.timezone")}</Label>
        <Input
          id="history-timezone"
          value={timezone}
          onChange={(event) => setTimezone(event.target.value)}
          autoComplete="off"
          autoFocus={compact}
        />
      </div>
      <div className="flex flex-col gap-1.5 text-left">
        <Label htmlFor="history-start-date">{t("history.startDate")}</Label>
        <Input
          id="history-start-date"
          data-testid="history-start-date"
          value={startDate ?? t("history.startDateUnknown")}
          readOnly
        />
        <p id="history-start-date-help" className="text-xs leading-5 text-muted-foreground">
          {t("history.startDateHelp")}
        </p>
      </div>
      {draft.isLoading && <LoadingState label={t("history.startLoading")} />}
      {draft.isError && (
        <p role="alert" className="text-sm text-destructive">
          {t("history.loadError")}
        </p>
      )}
      {holdings.length > 0 && (
        <div className="flex flex-col gap-3 text-left">
          <div>
            <h3 className="text-sm font-medium text-foreground">{t("history.startCostTitle")}</h3>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">{t("history.startCostHelp")}</p>
          </div>
          {holdings.map((holding) => (
            <div key={holding.holdingId} className="flex flex-col gap-1.5">
              <Label htmlFor={`start-cost-${holding.holdingId}`}>
                {holding.instrumentName} · {formatAmount(holding.quantity)}
              </Label>
              <Input
                id={`start-cost-${holding.holdingId}`}
                inputMode="decimal"
                value={costFor(holding.holdingId, holding.unitCost)}
                onChange={(event) => setUnitCosts((current) => ({ ...current, [holding.holdingId]: event.target.value }))}
                placeholder={t("history.startCostDescription")}
              />
            </div>
          ))}
        </div>
      )}
      <div className={`flex gap-2 ${compact ? "" : "self-center"}`}>
        {onCancel && (
          <Button type="button" variant="outline" onClick={onCancel} disabled={startHistory.isPending}>
            {t("common.cancel")}
          </Button>
        )}
        <Button type="button" onClick={submit} disabled={startHistory.isPending || draft.isLoading || missingCost}>
          {startHistory.isPending ? t("common.pending") : t("history.startButton")}
        </Button>
      </div>
      {missingCost && (
        <p role="alert" className="text-sm text-destructive">
          {t("history.startCostRequired")}
        </p>
      )}
      {startHistory.isError && (
        <p role="alert" className="text-sm text-destructive">
          {displayError(startHistory.error, t("history.startError"))}
        </p>
      )}
    </div>
  );
}
