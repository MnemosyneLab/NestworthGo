import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { useAccounts } from "@/queries/accounts";
import { useInstruments, useAllHoldingsFlat } from "@/queries/investments";
import { usePreviewChange, usePreviewFixChange, useRecordChange, useFixChange } from "@/queries/history";
import { ChangeCommandKind, type ChangeCommandRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";
import type { EndpointViewDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { formatAmount } from "@/lib/money";
import { displayError } from "@/lib/display";
import { emptyChangeRequest } from "@/features/history/activityToCommand";

const KINDS: ChangeCommandKind[] = [
  ChangeCommandKind.ChangeMoneyAdded,
  ChangeCommandKind.ChangeMoneyRemoved,
  ChangeCommandKind.ChangeCashTransfer,
  ChangeCommandKind.ChangeFXConversion,
  ChangeCommandKind.ChangePositionTransfer,
  ChangeCommandKind.ChangePositionAdjustment,
  ChangeCommandKind.ChangeTrade,
  ChangeCommandKind.ChangeValueUpdate,
  ChangeCommandKind.ChangeDebtDraw,
  ChangeCommandKind.ChangeDebtPayment,
];

const MONEY_IN_REASONS = ["income", "contribution", "gift", "other", "reconciliation"] as const;
const MONEY_OUT_REASONS = ["expense", "fee", "tax", "other", "reconciliation"] as const;
const VALUE_UPDATE_REASONS = ["reconciliation", "other"] as const;

function AccountSelect({
  id,
  label,
  value,
  onChange,
  accounts,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  accounts: { id: string; name: string }[];
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <NativeSelect id={id} value={value} onChange={(event) => onChange(event.target.value)}>
        <option value="">{t("history.accountSelectEmpty")}</option>
        {accounts.map((account) => (
          <option key={account.id} value={account.id}>
            {account.name}
          </option>
        ))}
      </NativeSelect>
    </div>
  );
}

function OptionSelect({
  id,
  label,
  value,
  emptyLabel,
  options,
  onChange,
}: {
  id: string;
  label: string;
  value: string;
  emptyLabel: string;
  options: { id: string; name: string }[];
  onChange: (value: string) => void;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <NativeSelect id={id} value={value} onChange={(event) => onChange(event.target.value)}>
        <option value="">{emptyLabel}</option>
        {options.map((option) => (
          <option key={option.id} value={option.id}>
            {option.name}
          </option>
        ))}
      </NativeSelect>
    </div>
  );
}

function MoneyFields({
  prefix,
  label,
  amount,
  currency,
  onAmount,
  onCurrency,
}: {
  prefix: string;
  label: string;
  amount: string;
  currency: string;
  onAmount: (value: string) => void;
  onCurrency: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex gap-2">
      <div className="flex flex-1 flex-col gap-1.5">
        <Label htmlFor={`${prefix}-amount`}>{label}</Label>
        <Input id={`${prefix}-amount`} inputMode="decimal" value={amount} onChange={(event) => onAmount(event.target.value)} />
      </div>
      <div className="flex w-24 flex-col gap-1.5">
        <Label htmlFor={`${prefix}-currency`}>{t("history.currency")}</Label>
        <Input id={`${prefix}-currency`} value={currency} onChange={(event) => onCurrency(event.target.value.toUpperCase())} maxLength={3} />
      </div>
    </div>
  );
}

/**
 * RecordChangeForm implements the "record a change with ordinary
 * language" flow for every one of the ten domain.PreviewChange kinds via
 * history.ChangeCommandRequest's tagged union. It always previews before
 * committing: Preview calls PreviewChange (no side effect) and shows the
 * resulting balances/quantities; Confirm re-submits the identical request
 * to RecordChange, which re-validates and commits.
 *
 * When `fixActivityId` is set, the form instead drives HistoryService's
 * Fix flow: it starts pre-filled from `initial` and Confirm calls
 * FixChange instead of RecordChange. Reason and note are visible fields
 * so a Fix cannot resubmit hidden values the user did not see.
 */
export function RecordChangeForm({
  onRecorded,
  initial,
  fixActivityId,
}: {
  onRecorded: () => void;
  initial?: ChangeCommandRequest;
  fixActivityId?: string;
}) {
  const { t } = useTranslation();
  const accounts = useAccounts({});
  const instruments = useInstruments();
  const allAccountIds = (accounts.data ?? []).map((record) => record.account.id);
  const holdings = useAllHoldingsFlat(allAccountIds);
  const preview = usePreviewChange();
  const previewFix = usePreviewFixChange();
  const record = useRecordChange();
  const fix = useFixChange();

  const [request, setRequest] = useState<ChangeCommandRequest>(initial ?? emptyChangeRequest(ChangeCommandKind.ChangeMoneyAdded));
  const [previewResult, setPreviewResult] = useState<EndpointViewDTO[] | null>(null);

  const patch = (next: Partial<ChangeCommandRequest>) => {
    setRequest((current) => ({ ...current, ...next }));
    setPreviewResult(null);
  };

  const accountOptions = (accounts.data ?? []).map((record) => ({ id: record.account.id, name: record.account.name }));
  const holdingsAccountOptions = (accounts.data ?? [])
    .filter((record) => record.account.trackingMode === "holdings")
    .map((record) => ({ id: record.account.id, name: record.account.name }));
  const holdingOptions = holdings.data.map((holding) => ({
    id: holding.id,
    name: holding.instrumentName ?? t("portfolio.unknownInstrument"),
  }));
  const instrumentOptions = (instruments.data ?? []).map((instrument) => ({ id: instrument.id, name: instrument.name }));

  const kind = request.kind;
  const showReason = kind === ChangeCommandKind.ChangeMoneyAdded || kind === ChangeCommandKind.ChangeMoneyRemoved || kind === ChangeCommandKind.ChangeValueUpdate;
  const reasonOptions =
    kind === ChangeCommandKind.ChangeMoneyRemoved ? MONEY_OUT_REASONS : kind === ChangeCommandKind.ChangeValueUpdate ? VALUE_UPDATE_REASONS : MONEY_IN_REASONS;

  const runPreview = () => {
    if (fixActivityId) {
      previewFix.mutate({ activityId: fixActivityId, replacement: request }, { onSuccess: (result) => setPreviewResult(result.resulting) });
      return;
    }
    preview.mutate(request, { onSuccess: (result) => setPreviewResult(result.resulting) });
  };

  const runConfirm = () => {
    if (fixActivityId) {
      fix.mutate(
        { activityId: fixActivityId, replacement: request },
        {
          onSuccess: () => {
            setPreviewResult(null);
            onRecorded();
          },
        },
      );
      return;
    }
    record.mutate(request, {
      onSuccess: () => {
        setPreviewResult(null);
        setRequest(emptyChangeRequest(ChangeCommandKind.ChangeMoneyAdded));
        onRecorded();
      },
    });
  };

  const error = preview.error ?? previewFix.error ?? record.error ?? fix.error;
  const previewPending = fixActivityId ? previewFix.isPending : preview.isPending;
  const confirmPending = fixActivityId ? fix.isPending : record.isPending;

  return (
    <div className="flex flex-col gap-4" role="form" aria-label={t("history.formLabel")}>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="change-kind">{t("history.changeType")}</Label>
        <NativeSelect
          id="change-kind"
          value={kind}
          onChange={(event) => {
            setRequest(emptyChangeRequest(event.target.value as ChangeCommandKind));
            setPreviewResult(null);
          }}
        >
          {KINDS.map((value) => (
            <option key={value} value={value}>
              {t(`history.kind.${value}`)}
            </option>
          ))}
        </NativeSelect>
      </div>

      {(kind === ChangeCommandKind.ChangeMoneyAdded || kind === ChangeCommandKind.ChangeMoneyRemoved) && (
        <>
          <AccountSelect id="change-account" label={t("history.accountSelect")} value={request.accountId ?? ""} onChange={(accountId) => patch({ accountId })} accounts={accountOptions} />
          <MoneyFields prefix="change" label={t("history.amount")} amount={request.amount ?? ""} currency={request.currency ?? "USD"} onAmount={(amount) => patch({ amount })} onCurrency={(currency) => patch({ currency })} />
        </>
      )}

      {kind === ChangeCommandKind.ChangeValueUpdate && (
        <>
          <AccountSelect id="change-account" label={t("history.accountSelect")} value={request.accountId ?? ""} onChange={(accountId) => patch({ accountId })} accounts={accountOptions} />
          <MoneyFields prefix="change" label={t("history.newValue")} amount={request.newValue ?? ""} currency={request.newValueCurrency ?? "USD"} onAmount={(newValue) => patch({ newValue })} onCurrency={(newValueCurrency) => patch({ newValueCurrency })} />
        </>
      )}

      {kind === ChangeCommandKind.ChangeCashTransfer && (
        <>
          <AccountSelect id="change-from-account" label={t("history.fromAccount")} value={request.fromAccountId ?? ""} onChange={(fromAccountId) => patch({ fromAccountId })} accounts={accountOptions} />
          <AccountSelect id="change-to-account" label={t("history.toAccount")} value={request.toAccountId ?? ""} onChange={(toAccountId) => patch({ toAccountId })} accounts={accountOptions} />
          <MoneyFields prefix="change-sent" label={t("history.sent")} amount={request.sent ?? ""} currency={request.sentCurrency ?? "USD"} onAmount={(sent) => patch({ sent })} onCurrency={(sentCurrency) => patch({ sentCurrency })} />
          <MoneyFields prefix="change-received" label={t("history.received")} amount={request.received ?? ""} currency={request.receivedCurrency ?? "USD"} onAmount={(received) => patch({ received })} onCurrency={(receivedCurrency) => patch({ receivedCurrency })} />
        </>
      )}

      {kind === ChangeCommandKind.ChangeFXConversion && (
        <>
          <AccountSelect id="change-account" label={t("history.accountSelect")} value={request.accountId ?? ""} onChange={(accountId) => patch({ accountId })} accounts={holdingsAccountOptions} />
          <MoneyFields prefix="change-sold" label={t("history.sold")} amount={request.sold ?? ""} currency={request.soldCurrency ?? "USD"} onAmount={(sold) => patch({ sold })} onCurrency={(soldCurrency) => patch({ soldCurrency })} />
          <MoneyFields prefix="change-bought" label={t("history.bought")} amount={request.bought ?? ""} currency={request.boughtCurrency ?? "USD"} onAmount={(bought) => patch({ bought })} onCurrency={(boughtCurrency) => patch({ boughtCurrency })} />
        </>
      )}

      {kind === ChangeCommandKind.ChangePositionTransfer && (
        <>
          <OptionSelect id="change-from-holding" label={t("history.fromHolding")} value={request.fromHoldingId ?? ""} emptyLabel={t("history.selectEmpty")} options={holdingOptions} onChange={(fromHoldingId) => patch({ fromHoldingId })} />
          <OptionSelect id="change-to-holding" label={t("history.toHolding")} value={request.toHoldingId ?? ""} emptyLabel={t("history.selectEmpty")} options={holdingOptions} onChange={(toHoldingId) => patch({ toHoldingId })} />
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-quantity">{t("history.quantity")}</Label>
            <Input id="change-quantity" inputMode="decimal" value={request.quantity ?? ""} onChange={(event) => patch({ quantity: event.target.value })} />
          </div>
        </>
      )}

      {kind === ChangeCommandKind.ChangePositionAdjustment && (
        <>
          <OptionSelect id="change-holding" label={t("history.holding")} value={request.holdingId ?? ""} emptyLabel={t("history.selectEmpty")} options={holdingOptions} onChange={(holdingId) => patch({ holdingId })} />
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={request.added ?? true} onChange={(event) => patch({ added: event.target.checked })} /> {request.added ? t("history.added") : t("history.removed")}
          </label>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-quantity">{t("history.quantity")}</Label>
            <Input id="change-quantity" inputMode="decimal" value={request.quantity ?? ""} onChange={(event) => patch({ quantity: event.target.value })} />
          </div>
          {request.added && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="change-unit-cost">{t("history.unitCostOptional")}</Label>
              <Input id="change-unit-cost" inputMode="decimal" value={request.unitCost ?? ""} onChange={(event) => patch({ unitCost: event.target.value })} />
            </div>
          )}
        </>
      )}

      {kind === ChangeCommandKind.ChangeTrade && (
        <>
          <AccountSelect id="change-settlement-account" label={t("history.settlementAccount")} value={request.settlementAccountId ?? ""} onChange={(settlementAccountId) => patch({ settlementAccountId })} accounts={holdingsAccountOptions} />
          <OptionSelect id="change-instrument" label={t("history.instrument")} value={request.instrumentId ?? ""} emptyLabel={t("history.selectEmpty")} options={instrumentOptions} onChange={(instrumentId) => patch({ instrumentId })} />
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-side">{t("history.side")}</Label>
            <NativeSelect id="change-side" value={request.side ?? "buy"} onChange={(event) => patch({ side: event.target.value })}>
              <option value="buy">{t("history.buy")}</option>
              <option value="sell">{t("history.sell")}</option>
            </NativeSelect>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-quantity">{t("history.quantity")}</Label>
            <Input id="change-quantity" inputMode="decimal" value={request.quantity ?? ""} onChange={(event) => patch({ quantity: event.target.value })} />
          </div>
          <MoneyFields prefix="change-gross" label={t("history.grossTotal")} amount={request.gross ?? ""} currency={request.grossCurrency ?? "USD"} onAmount={(gross) => patch({ gross })} onCurrency={(grossCurrency) => patch({ grossCurrency })} />
          <MoneyFields prefix="change-fee" label={t("history.feeOptional")} amount={request.fee ?? ""} currency={request.feeCurrency ?? request.grossCurrency ?? "USD"} onAmount={(fee) => patch({ fee })} onCurrency={(feeCurrency) => patch({ feeCurrency })} />
        </>
      )}

      {(kind === ChangeCommandKind.ChangeDebtDraw || kind === ChangeCommandKind.ChangeDebtPayment) && (
        <>
          <AccountSelect id="change-debt-account" label={t("history.debtAccount")} value={request.debtAccountId ?? ""} onChange={(debtAccountId) => patch({ debtAccountId })} accounts={accountOptions} />
          <AccountSelect id="change-cash-account" label={t("history.cashAccount")} value={request.cashAccountId ?? ""} onChange={(cashAccountId) => patch({ cashAccountId })} accounts={accountOptions} />
          <MoneyFields prefix="change-principal" label={t("history.principal")} amount={request.principal ?? ""} currency={request.principalCurrency ?? "USD"} onAmount={(principal) => patch({ principal })} onCurrency={(principalCurrency) => patch({ principalCurrency })} />
          {kind === ChangeCommandKind.ChangeDebtPayment && (
            <MoneyFields prefix="change-interest" label={t("history.interestOptional")} amount={request.interestOrFee ?? ""} currency={request.interestOrFeeCurrency ?? request.principalCurrency ?? "USD"} onAmount={(interestOrFee) => patch({ interestOrFee })} onCurrency={(interestOrFeeCurrency) => patch({ interestOrFeeCurrency })} />
          )}
        </>
      )}

      {showReason && (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="change-reason">{t("history.reasonLabel")}</Label>
          <NativeSelect id="change-reason" value={request.reason ?? "other"} onChange={(event) => patch({ reason: event.target.value })}>
            {reasonOptions.map((reason) => (
              <option key={reason} value={reason}>
                {t(`history.reason.${reason}`)}
              </option>
            ))}
          </NativeSelect>
        </div>
      )}

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="change-note">{t("history.note")}</Label>
        <Input id="change-note" value={request.note ?? ""} onChange={(event) => patch({ note: event.target.value || null })} placeholder={t("history.notePlaceholder")} />
      </div>

      {(accounts.isError || instruments.isError || holdings.isError) && (
        <div role="alert" className="flex flex-wrap items-center gap-2 text-sm text-destructive">
          <span>{t("history.dependenciesError")}</span>
          <Button type="button" variant="outline" size="sm" onClick={() => void Promise.all([accounts.refetch(), instruments.refetch(), holdings.refetch()])}>
            {t("common.retryAction")}
          </Button>
        </div>
      )}

      {error && (
        <p role="alert" className="text-sm text-destructive">
          {displayError(error, t("history.actionError"))}
        </p>
      )}

      {previewResult && (
        <div role="status" className="flex flex-col gap-1 rounded-md border border-border bg-muted p-3 text-sm">
          <p className="font-medium">{t("history.previewTitle")}</p>
          <p className="text-muted-foreground">{t("history.previewDescription")}</p>
          {previewResult.map((endpoint, index) => (
            <p key={index}>
              {endpoint.name}: {endpoint.amount ? formatAmount(endpoint.amount, endpoint.currency) : formatAmount(endpoint.quantity ?? "0")}
            </p>
          ))}
        </div>
      )}

      <div className="flex gap-2">
        {!previewResult ? (
          <Button type="button" onClick={runPreview} disabled={previewPending}>
            {previewPending ? t("common.pending") : t("common.preview")}
          </Button>
        ) : (
          <Button type="button" onClick={runConfirm} disabled={confirmPending}>
            {confirmPending ? t("common.pending") : t("common.confirm")}
          </Button>
        )}
      </div>
    </div>
  );
}
