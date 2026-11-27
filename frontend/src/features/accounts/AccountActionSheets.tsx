import { useEffect, useRef, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { LoadingState, ErrorState } from "@/components/layout/PageState";
import { Button } from "@/components/ui/button";
import { hasUnresolvedSheetConfirmation, useSheetConfirmation } from "@/components/ui/sheet-confirmation";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { usePreviewCashBalance, useConfirmCashBalance } from "@/queries/accounts";
import { useSupportedCurrencies } from "@/queries/settings";
import { useHistoryOrigin } from "@/queries/history";
import { RecordChangeForm, type RecordChangeLock } from "@/features/history/RecordChangeForm";
import { PositionImportForm } from "@/features/investments/PositionImportForm";
import { emptyChangeRequest } from "@/features/history/activityToCommand";
import { ChangeCommandKind, type ChangeCommandRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";
import type { AccountRecordDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { formatExactTimestamp } from "@/lib/time";
import { displayError } from "@/lib/display";
import { toast } from "sonner";
import { compareCanonical, formatAuditMoney, formatAmount } from "@/lib/money";

export type AccountAction =
  | "record"
  | "cash"
  | "deposit"
  | "withdraw"
  | "fx"
  | "transfer"
  | "buy"
  | "sell"
  | "dividend"
  | "position"
  | "simple"
  | "settings"
  | null;

export function AccountActionSheet({
  action,
  holdingId,
  record,
  valuation,
  onClose,
  onRecorded,
  finalFocus,
}: {
  action: Exclude<AccountAction, "settings" | null>;
  holdingId?: string;
  record: AccountRecordDTO;
  valuation?: import("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models").AccountValuationDTO;
  onClose: () => void;
  onRecorded: () => void;
  finalFocus?: RefObject<HTMLElement> | (() => HTMLElement | null | undefined);
}) {
  const { t } = useTranslation();
  const origin = useHistoryOrigin();
  const account = record.account;
  let recordKind = ChangeCommandKind.ChangeMoneyAdded;
  if (account.trackingMode === "manual_value") recordKind = ChangeCommandKind.ChangeValueUpdate;
  else if (account.balanceSheetRole === "liability" && account.accountType !== "credit_card") recordKind = ChangeCommandKind.ChangeDebtDraw;
  else if (account.trackingMode === "holdings" && account.accountType !== "cash_on_hand") recordKind = ChangeCommandKind.ChangeTrade;
  const title =
    action === "record"
      ? t("history.recordTitle")
      : action === "cash"
      ? t("accounts.reconcileBalance")
      : action === "deposit" || action === "withdraw"
        ? t("accounts.depositOrWithdraw")
        : action === "fx"
          ? t("accounts.convert")
          : action === "transfer"
            ? t("accounts.transfer")
            : action === "buy"
              ? t("accounts.buyInvestment")
              : action === "sell"
                ? t("accounts.sellInvestment")
                : action === "dividend"
                  ? t("accounts.cashDividend")
                  : action === "position"
                  ? t("accounts.recordPosition")
                  : record.account.trackingMode === "manual_value"
                    ? t("accounts.updateValue")
                    : t("accounts.updateBalance");

  return (
    <Sheet open onOpenChange={(open) => !open && onClose()}>
      <SheetContent size="lg" finalFocus={finalFocus}>
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
        </SheetHeader>
        <div>
          {record.account.archivedAt && !hasUnresolvedSheetConfirmation() ? (
            <p className="text-sm text-muted-foreground">{t("accounts.archivedReadOnly")}</p>
          ) : !origin.data && origin.isLoading && !hasUnresolvedSheetConfirmation() ? (
            <LoadingState label={t("common.loading")} />
          ) : !origin.data && !hasUnresolvedSheetConfirmation() ? (
            <ErrorState title={t("accounts.loadError")} description={displayError(origin.error, t("accounts.loadError"))} onRetry={() => origin.refetch()} retryLabel={t("common.retryAction")} />
          ) : action === "record" ? (
            <RecordChangeForm
              initial={{
                ...emptyChangeRequest(recordKind, record.account.defaultCurrency),
                accountId: record.account.id,
                settlementAccountId: recordKind === ChangeCommandKind.ChangeTrade ? record.account.id : undefined,
                debtAccountId: recordKind === ChangeCommandKind.ChangeDebtDraw ? record.account.id : undefined,
                reason: record.account.accountType === "credit_card" ? "card_purchase" : "other",
              }}
              lock={{ accountId: record.account.id }}
              prefill={{ account: record.account }}
              onRecorded={onRecorded}
            />
          ) : action === "cash" ? (
            <CashBalanceForm record={record} valuation={valuation} onDone={onRecorded} />
          ) : action === "position" ? (
            <PositionImportForm account={record.account} onRecorded={onRecorded} />
          ) : action === "simple" ? (
            <SimpleValueForm record={record} onDone={onRecorded} />
          ) : (
            <LockedChangeForm action={action} holdingId={holdingId} record={record} onDone={onRecorded} />
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}

function reviewedCashAmount(amount: string, currency: string, signed = false): string {
  const sign = signed && compareCanonical(amount, "0.005") > 0 ? "+" : "";
  return `${sign}${formatAuditMoney(amount, currency)}`;
}

function CashBalanceForm({
  record,
  valuation,
  onDone,
}: {
  record: AccountRecordDTO;
  valuation?: import("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models").AccountValuationDTO;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const currencies = useSupportedCurrencies();
  const preview = usePreviewCashBalance();
  const confirm = useConfirmCashBalance(record.account.id);
  const sheetConfirmation = useSheetConfirmation();
  const [uncertain, setUncertain] = useState(Boolean(sheetConfirmation?.identity));
  const [planUnavailable, setPlanUnavailable] = useState(false);
  const retainedReview = sheetConfirmation?.recovery as import("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding/models").CashBalancePreviewDTO | null;
  const [reviewed, setReviewed] = useState<import("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding/models").CashBalancePreviewDTO | null>(retainedReview);
  const [reason, setReason] = useState(retainedReview?.reason ?? "");
  const sequence = useRef(0);
  const busy = useRef(false);
  const clearPreview = () => { sequence.current += 1; setReviewed(null); };
  useEffect(() => () => { sequence.current += 1; }, []);
  const [currency, setCurrency] = useState(retainedReview?.currency ?? record.account.defaultCurrency);
  const [amount, setAmount] = useState(retainedReview?.target ?? "");
  const [error, setError] = useState<string | undefined>();
  const options = currencies.data ?? [record.account.defaultCurrency];
  const currentCash = (valuation?.components ?? []).find((component) => !component.instrumentId && component.nativeCurrency === currency);

  const submit = () => {
    if (busy.current || uncertain || !amount.trim()) return;
    busy.current = true;
    setError(undefined);
    const current = ++sequence.current;
    preview.mutate({ accountId: record.account.id, amount: amount.trim(), currency, reason }, {
      onSuccess: (result) => { if (current === sequence.current) setReviewed(result); },
      onError: (cause) => { if (current === sequence.current) setError(displayError(cause, t("accounts.saveError"))); },
      onSettled: () => { busy.current = false; },
    });
  };
  const commit = (event: React.MouseEvent<HTMLButtonElement>) => {
    if (event.detail > 1) return;
    if (!reviewed || busy.current) return;
    busy.current = true;
    const planId = sheetConfirmation?.identity ?? reviewed.planId;
    sheetConfirmation?.begin(planId, reviewed);
    confirm.mutate(planId, {
      onSuccess: () => { if (sheetConfirmation && !sheetConfirmation.resolve()) return; toast.success(t("common.saved")); onDone(); },
      onError: (cause) => {
        setError(displayError(cause, t("accounts.saveError")));
        const wire = (cause as { wireError?: { code: string; field?: string } } | null)?.wireError;
        // Missing desktop plans cannot reach durable receipt lookup. Keep the
        // identity after transport/storage failures; only proven refusal unlocks.
        const refused = wire && (wire.code === "cash_balance_time_conflict" ||
          (wire.code === "stale_preview" && wire.field === "previewToken") ||
          ["validation", "invalid_change", "invalid_change_time", "insufficient_balance", "not_found", "conflict", "no_change"].includes(wire.code));
        setUncertain(!refused);
        setPlanUnavailable(wire?.code === "stale_preview" && !refused);
        if (refused) { sheetConfirmation?.resolve(); clearPreview(); }
      },
      onSettled: () => { busy.current = false; },
    });
  };

  return (
    <div className="flex flex-col gap-4" role="form" aria-label={t("accounts.reconcileBalance")}>
      <p className="text-sm text-muted-foreground">{t("accounts.cashReconcileHint")}</p>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="cash-currency">{t("accounts.cashCurrency")}</Label>
        <NativeSelect id="cash-currency" value={currency} disabled={confirm.isPending || uncertain} onChange={(event) => { setCurrency(event.target.value); clearPreview(); }}>
          {options.map((code) => (
            <option key={code} value={code}>
              {code}
            </option>
          ))}
        </NativeSelect>
      </div>
      <p className="text-xs text-muted-foreground">
        {currentCash ? t("accounts.currentBalance", { amount: formatAmount(currentCash.nativeAmount, currentCash.nativeCurrency) }) : t("accounts.noCash")}
      </p>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="cash-amount">{t("accounts.resultingBalance")}</Label>
        <Input id="cash-amount" inputMode="decimal" value={amount} disabled={confirm.isPending || uncertain} onChange={(event) => { setAmount(event.target.value); clearPreview(); }} autoFocus />
      </div>
      {<div className="flex flex-col gap-1.5"><Label htmlFor="cash-reason">{t("accounts.adjustmentReason")}</Label><Input id="cash-reason" value={reason} disabled={confirm.isPending || uncertain} onChange={(event) => { setReason(event.target.value); clearPreview(); }} /></div>}
      {reviewed && <div role="status" className="rounded-xl border border-border bg-muted p-3 text-sm">
        <p>{t("history.previewDescription")}</p>
        <dl>{([ ["accounts.balanceBefore", reviewedCashAmount(reviewed.current, reviewed.currency)], ["accounts.resultingBalance", reviewedCashAmount(reviewed.target, reviewed.currency)], ["accounts.adjustmentDelta", reviewedCashAmount(reviewed.delta, reviewed.currency, true)], ["history.currency", reviewed.currency], ["history.effectiveAt", `${formatExactTimestamp(reviewed.effectiveAt, reviewed.timezone || "UTC")} (${reviewed.timezone || "UTC"})`], ...(reviewed.reason ? [["accounts.adjustmentReason", reviewed.reason]] : []) ]).map(([label, value]) => <div key={label}><dt className="font-medium">{t(label)}</dt><dd>{value}</dd></div>)}</dl>
      </div>}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {uncertain && <p className="text-sm text-muted-foreground">{t(planUnavailable ? "history.repeatPlanUnavailableHelp" : "history.repeatRetryHelp")}</p>}
      <div className="flex gap-2">
        <Button type="button" onClick={reviewed ? commit : submit} disabled={preview.isPending || confirm.isPending || !amount.trim()}>{preview.isPending || confirm.isPending ? t("common.pending") : reviewed ? uncertain ? t("history.repeatRetryConfirm") : t("common.confirm") : t("common.preview")}</Button>
        {reviewed && <Button type="button" variant="outline" onClick={clearPreview} disabled={confirm.isPending || uncertain}>{t("common.backToDraft")}</Button>}
      </div>
    </div>
  );
}

function SimpleValueForm({ record, onDone }: { record: AccountRecordDTO; onDone: () => void }) {
  const initial: ChangeCommandRequest = {
      ...emptyChangeRequest(ChangeCommandKind.ChangeValueUpdate, record.account.defaultCurrency),
      accountId: record.account.id,
      currency: record.account.defaultCurrency,
    };
    return (
      <RecordChangeForm
        initial={initial}
        lock={{ kind: ChangeCommandKind.ChangeValueUpdate, hideKind: true, accountId: record.account.id }}
        onRecorded={onDone}
      />
    );
  }

function LockedChangeForm({
  action,
  holdingId,
  record,
  onDone,
}: {
  action: Exclude<AccountAction, "record" | "cash" | "position" | "simple" | "settings" | null>;
  holdingId?: string;
  record: AccountRecordDTO;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const confirmation = useSheetConfirmation();
  const currency = record.account.defaultCurrency;
  const isMoney = action === "deposit" || action === "withdraw";
  const [moneyKind, setMoneyKind] = useState(
    action === "withdraw" ? ChangeCommandKind.ChangeMoneyRemoved : ChangeCommandKind.ChangeMoneyAdded,
  );
  const kind =
    isMoney
      ? moneyKind
      : action === "fx"
        ? ChangeCommandKind.ChangeFXConversion
        : action === "transfer"
          ? ChangeCommandKind.ChangeCashTransfer
          : action === "dividend"
            ? ChangeCommandKind.ChangeCashDividend
            : ChangeCommandKind.ChangeTrade;
  const initial: ChangeCommandRequest = {
    ...emptyChangeRequest(kind, currency),
    accountId: record.account.id,
    settlementAccountId: record.account.id,
    fromAccountId: action === "transfer" ? record.account.id : undefined,
    holdingId: action === "dividend" ? holdingId : undefined,
    side: action === "sell" ? "sell" : "buy",
    currency,
  };
  const lock: RecordChangeLock = {
    kind,
    hideKind: true,
    accountId: action === "buy" || action === "sell" ? undefined : record.account.id,
    settlementAccountId: action === "buy" || action === "sell" ? record.account.id : undefined,
    holdingId: action === "dividend" ? holdingId : undefined,
  };
  return (
    <div className="flex flex-col gap-4">
      {isMoney && (
        <div className="flex flex-col gap-2" role="radiogroup" aria-label={t("accounts.depositOrWithdraw")}>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="radio"
              name="money-direction"
              checked={moneyKind === ChangeCommandKind.ChangeMoneyAdded}
              disabled={Boolean(confirmation?.identity)}
              onChange={() => { if (!confirmation?.read()) setMoneyKind(ChangeCommandKind.ChangeMoneyAdded); }}
            />
            {t("history.kind.money_added")}
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="radio"
              name="money-direction"
              checked={moneyKind === ChangeCommandKind.ChangeMoneyRemoved}
              disabled={Boolean(confirmation?.identity)}
              onChange={() => { if (!confirmation?.read()) setMoneyKind(ChangeCommandKind.ChangeMoneyRemoved); }}
            />
            {t("history.kind.money_removed")}
          </label>
        </div>
      )}
      <RecordChangeForm key={kind} initial={initial} lock={lock} onRecorded={onDone} />
    </div>
  );
}
