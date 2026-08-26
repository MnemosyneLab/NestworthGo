import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAccounts } from "@/queries/accounts";
import { useInstruments, useAllHoldingsFlat } from "@/queries/investments";
import { usePreviewChange, usePreviewFixChange, useRecordChange, useFixChange } from "@/queries/history";
import type { ChangeCommandRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";
import type { EndpointViewDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { formatAmount } from "@/lib/money";
import { displayError } from "@/lib/display";

const KINDS = [
  "money_added",
  "money_removed",
  "cash_transfer",
  "fx_conversion",
  "position_transfer",
  "position_adjustment",
  "trade",
  "value_update",
  "debt_draw",
  "debt_payment",
];

function AccountSelect({ id, label, value, onChange, accounts }: { id: string; label: string; value: string; onChange: (value: string) => void; accounts: { id: string; name: string }[] }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <select id={id} value={value} onChange={(event) => onChange(event.target.value)} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
        <option value="">{t("history.accountSelectEmpty")}</option>
        {accounts.map((account) => (
          <option key={account.id} value={account.id}>
            {account.name}
          </option>
        ))}
      </select>
    </div>
  );
}

function MoneyFields({ prefix, label, amount, currency, onAmount, onCurrency }: { prefix: string; label: string; amount: string; currency: string; onAmount: (v: string) => void; onCurrency: (v: string) => void }) {
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

export interface RecordChangeFormInitial {
  kind: string;
  fields: Record<string, string>;
  added: boolean;
}

/**
 * RecordChangeForm implements the "record a change with ordinary
 * language" flow for every one of the ten domain.PreviewChange kinds via
 * history.ChangeCommandRequest's tagged union. It always previews before
 * committing: Preview
 * calls PreviewChange (no side effect) and shows the resulting
 * balances/quantities; Confirm re-submits the identical request to
 * RecordChange, which re-validates and commits.
 *
 * When `fixActivityId` is set, the form instead drives HistoryService's
 * Fix flow: it starts pre-filled from `initial` (see
 * activityToInitialCommand.ts) and Confirm calls FixChange instead of
 * RecordChange, replacing the original activity with the edited one.
 */
export function RecordChangeForm({
  onRecorded,
  initial,
  fixActivityId,
}: {
  onRecorded: () => void;
  initial?: RecordChangeFormInitial;
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

  const [kind, setKind] = useState<string>(initial?.kind ?? "money_added");
  const [fields, setFields] = useState<Record<string, string>>(initial?.fields ?? { currency: "USD" });
  const [added, setAdded] = useState(initial?.added ?? true);
  const [previewResult, setPreviewResult] = useState<EndpointViewDTO[] | null>(null);

  const set = (key: string, value: string) => {
    setFields((prev) => ({ ...prev, [key]: value }));
    setPreviewResult(null);
  };

  const accountOptions = (accounts.data ?? []).map((record) => ({ id: record.account.id, name: record.account.name }));
  const holdingsAccountOptions = (accounts.data ?? [])
    .filter((record) => record.account.trackingMode === "holdings")
    .map((record) => ({ id: record.account.id, name: record.account.name }));

  const buildRequest = (): ChangeCommandRequest => ({
    kind: kind as ChangeCommandRequest["kind"],
    added,
    ...fields,
  });

  const runPreview = () => {
    if (fixActivityId) {
      previewFix.mutate(
        { activityId: fixActivityId, replacement: buildRequest() },
        { onSuccess: (result) => setPreviewResult(result.resulting) },
      );
      return;
    }
    preview.mutate(buildRequest(), { onSuccess: (result) => setPreviewResult(result.resulting) });
  };

  const runConfirm = () => {
    if (fixActivityId) {
      fix.mutate(
        { activityId: fixActivityId, replacement: buildRequest() },
        {
          onSuccess: () => {
            setPreviewResult(null);
            onRecorded();
          },
        },
      );
      return;
    }
    record.mutate(buildRequest(), {
      onSuccess: () => {
        setPreviewResult(null);
        setFields({ currency: "USD" });
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
        <select
          id="change-kind"
          value={kind}
          onChange={(event) => {
            setKind(event.target.value);
            setFields({ currency: "USD" });
            setPreviewResult(null);
          }}
          className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground"
        >
          {KINDS.map((value) => (
            <option key={value} value={value}>
              {t(`history.kind.${value}`)}
            </option>
          ))}
        </select>
      </div>

      {kind === "money_added" && (
        <>
          <AccountSelect id="change-account" label={t("history.accountSelect")} value={fields.accountId ?? ""} onChange={(v) => set("accountId", v)} accounts={accountOptions} />
          <MoneyFields prefix="change" label={t("history.amount")} amount={fields.amount ?? ""} currency={fields.currency ?? "USD"} onAmount={(v) => set("amount", v)} onCurrency={(v) => set("currency", v)} />
        </>
      )}

      {kind === "money_removed" && (
        <>
          <AccountSelect id="change-account" label={t("history.accountSelect")} value={fields.accountId ?? ""} onChange={(v) => set("accountId", v)} accounts={accountOptions} />
          <MoneyFields prefix="change" label={t("history.amount")} amount={fields.amount ?? ""} currency={fields.currency ?? "USD"} onAmount={(v) => set("amount", v)} onCurrency={(v) => set("currency", v)} />
        </>
      )}

      {kind === "value_update" && (
        <>
          <AccountSelect id="change-account" label={t("history.accountSelect")} value={fields.accountId ?? ""} onChange={(v) => set("accountId", v)} accounts={accountOptions} />
          <MoneyFields prefix="change" label={t("history.newValue")} amount={fields.newValue ?? ""} currency={fields.newValueCurrency ?? "USD"} onAmount={(v) => set("newValue", v)} onCurrency={(v) => set("newValueCurrency", v)} />
        </>
      )}

      {kind === "cash_transfer" && (
        <>
          <AccountSelect id="change-from-account" label={t("history.fromAccount")} value={fields.fromAccountId ?? ""} onChange={(v) => set("fromAccountId", v)} accounts={accountOptions} />
          <AccountSelect id="change-to-account" label={t("history.toAccount")} value={fields.toAccountId ?? ""} onChange={(v) => set("toAccountId", v)} accounts={accountOptions} />
          <MoneyFields prefix="change-sent" label={t("history.sent")} amount={fields.sent ?? ""} currency={fields.sentCurrency ?? "USD"} onAmount={(v) => set("sent", v)} onCurrency={(v) => set("sentCurrency", v)} />
          <MoneyFields prefix="change-received" label={t("history.received")} amount={fields.received ?? ""} currency={fields.receivedCurrency ?? "USD"} onAmount={(v) => set("received", v)} onCurrency={(v) => set("receivedCurrency", v)} />
        </>
      )}

      {kind === "fx_conversion" && (
        <>
          <AccountSelect id="change-account" label={t("history.accountSelect")} value={fields.accountId ?? ""} onChange={(v) => set("accountId", v)} accounts={holdingsAccountOptions} />
          <MoneyFields prefix="change-sold" label={t("history.sold")} amount={fields.sold ?? ""} currency={fields.soldCurrency ?? "USD"} onAmount={(v) => set("sold", v)} onCurrency={(v) => set("soldCurrency", v)} />
          <MoneyFields prefix="change-bought" label={t("history.bought")} amount={fields.bought ?? ""} currency={fields.boughtCurrency ?? "USD"} onAmount={(v) => set("bought", v)} onCurrency={(v) => set("boughtCurrency", v)} />
        </>
      )}

      {kind === "position_transfer" && (
        <>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-from-holding">{t("history.fromHolding")}</Label>
            <select id="change-from-holding" value={fields.fromHoldingId ?? ""} onChange={(event) => set("fromHoldingId", event.target.value)} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
              <option value="">{t("history.selectEmpty")}</option>
              {holdings.data.map((holding) => (
                <option key={holding.id} value={holding.id}>
                  {holding.instrumentName ?? t("portfolio.unknownInstrument")}
                </option>
              ))}
            </select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-to-holding">{t("history.toHolding")}</Label>
            <select id="change-to-holding" value={fields.toHoldingId ?? ""} onChange={(event) => set("toHoldingId", event.target.value)} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
              <option value="">{t("history.selectEmpty")}</option>
              {holdings.data.map((holding) => (
                <option key={holding.id} value={holding.id}>
                  {holding.instrumentName ?? t("portfolio.unknownInstrument")}
                </option>
              ))}
            </select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-quantity">{t("history.quantity")}</Label>
            <Input id="change-quantity" inputMode="decimal" value={fields.quantity ?? ""} onChange={(event) => set("quantity", event.target.value)} />
          </div>
        </>
      )}

      {kind === "position_adjustment" && (
        <>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-holding">{t("history.holding")}</Label>
            <select id="change-holding" value={fields.holdingId ?? ""} onChange={(event) => set("holdingId", event.target.value)} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
              <option value="">{t("history.selectEmpty")}</option>
              {holdings.data.map((holding) => (
                <option key={holding.id} value={holding.id}>
                  {holding.instrumentName ?? t("portfolio.unknownInstrument")}
                </option>
              ))}
            </select>
          </div>
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={added} onChange={(event) => setAdded(event.target.checked)} /> {added ? t("history.added") : t("history.removed")}
          </label>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-quantity">{t("history.quantity")}</Label>
            <Input id="change-quantity" inputMode="decimal" value={fields.quantity ?? ""} onChange={(event) => set("quantity", event.target.value)} />
          </div>
          {added && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="change-unit-cost">{t("history.unitCostOptional")}</Label>
              <Input id="change-unit-cost" inputMode="decimal" value={fields.unitCost ?? ""} onChange={(event) => set("unitCost", event.target.value)} />
            </div>
          )}
        </>
      )}

      {kind === "trade" && (
        <>
          <AccountSelect id="change-settlement-account" label={t("history.settlementAccount")} value={fields.settlementAccountId ?? ""} onChange={(v) => set("settlementAccountId", v)} accounts={holdingsAccountOptions} />
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-instrument">{t("history.instrument")}</Label>
            <select id="change-instrument" value={fields.instrumentId ?? ""} onChange={(event) => set("instrumentId", event.target.value)} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
              <option value="">{t("history.selectEmpty")}</option>
              {(instruments.data ?? []).map((instrument) => (
                <option key={instrument.id} value={instrument.id}>
                  {instrument.name}
                </option>
              ))}
            </select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-side">{t("history.side")}</Label>
            <select id="change-side" value={fields.side ?? "buy"} onChange={(event) => set("side", event.target.value)} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
              <option value="buy">{t("history.buy")}</option>
              <option value="sell">{t("history.sell")}</option>
            </select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-quantity">{t("history.quantity")}</Label>
            <Input id="change-quantity" inputMode="decimal" value={fields.quantity ?? ""} onChange={(event) => set("quantity", event.target.value)} />
          </div>
          <MoneyFields prefix="change-gross" label={t("history.grossTotal")} amount={fields.gross ?? ""} currency={fields.grossCurrency ?? "USD"} onAmount={(v) => set("gross", v)} onCurrency={(v) => set("grossCurrency", v)} />
          <MoneyFields prefix="change-fee" label={t("history.feeOptional")} amount={fields.fee ?? ""} currency={fields.feeCurrency ?? fields.grossCurrency ?? "USD"} onAmount={(v) => set("fee", v)} onCurrency={(v) => set("feeCurrency", v)} />
        </>
      )}

      {(kind === "debt_draw" || kind === "debt_payment") && (
        <>
          <AccountSelect id="change-debt-account" label={t("history.debtAccount")} value={fields.debtAccountId ?? ""} onChange={(v) => set("debtAccountId", v)} accounts={accountOptions} />
          <AccountSelect id="change-cash-account" label={t("history.cashAccount")} value={fields.cashAccountId ?? ""} onChange={(v) => set("cashAccountId", v)} accounts={accountOptions} />
          <MoneyFields prefix="change-principal" label={t("history.principal")} amount={fields.principal ?? ""} currency={fields.principalCurrency ?? "USD"} onAmount={(v) => set("principal", v)} onCurrency={(v) => set("principalCurrency", v)} />
          {kind === "debt_payment" && (
            <MoneyFields prefix="change-interest" label={t("history.interestOptional")} amount={fields.interestOrFee ?? ""} currency={fields.interestOrFeeCurrency ?? fields.principalCurrency ?? "USD"} onAmount={(v) => set("interestOrFee", v)} onCurrency={(v) => set("interestOrFeeCurrency", v)} />
          )}
        </>
      )}

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
