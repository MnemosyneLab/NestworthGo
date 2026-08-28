import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { useAppendAccountCashValue, useAppendAccountValue } from "@/queries/accounts";
import { useCreateHolding, useCreateInstrument, useCurrentInstrumentQuote, useInstruments } from "@/queries/investments";
import { useRefreshInstrument } from "@/queries/marketdata";
import { useSupportedCurrencies } from "@/queries/settings";
import { useHistoryOrigin } from "@/queries/history";
import { InstrumentForm } from "@/features/investments/InstrumentForm";
import { RecordChangeForm, type RecordChangeLock } from "@/features/history/RecordChangeForm";
import { StartHistoryForm } from "@/features/history/StartHistoryForm";
import { emptyChangeRequest } from "@/features/history/activityToCommand";
import { ChangeCommandKind, type ChangeCommandRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";
import type { AccountRecordDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import type { HistoryOriginDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";
import { displayError } from "@/lib/display";
import { toast } from "sonner";
import { QuoteHint } from "@/features/history/QuoteHint";
import { formatAmount, sameCanonicalDecimal } from "@/lib/money";

export type AccountAction =
  | "cash"
  | "deposit"
  | "withdraw"
  | "fx"
  | "transfer"
  | "buy"
  | "sell"
  | "position"
  | "simple"
  | "settings"
  | null;

const HISTORY_ACTIONS: AccountAction[] = ["deposit", "withdraw", "fx", "transfer", "buy", "sell"];

export function actionNeedsHistory(action: AccountAction): boolean {
  return action !== null && HISTORY_ACTIONS.includes(action);
}

export function AccountActionSheet({
  action,
  record,
  valuation,
  historyStarted,
  onClose,
  onRecorded,
}: {
  action: Exclude<AccountAction, "settings" | null>;
  record: AccountRecordDTO;
  valuation?: import("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models").AccountValuationDTO;
  historyStarted: boolean;
  onClose: () => void;
  onRecorded: () => void;
}) {
  const { t } = useTranslation();
  const origin = useHistoryOrigin();
  const [startedInSheet, setStartedInSheet] = useState(false);
  const [startedOrigin, setStartedOrigin] = useState<HistoryOriginDTO>();
  const needsHistory = actionNeedsHistory(action);
  const historyReady = historyStarted || Boolean(origin.data) || startedInSheet;
  const waitingForHistory = needsHistory && !historyReady;
  const title =
    action === "cash"
      ? historyStarted
        ? t("accounts.reconcileBalance")
        : t("accounts.addCashBalance")
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
                : action === "position"
                  ? t("accounts.recordPosition")
                  : record.account.trackingMode === "manual_value"
                    ? t("accounts.updateValue")
                    : t("accounts.updateBalance");

  return (
    <Sheet open onOpenChange={(open) => !open && onClose()}>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>{waitingForHistory ? t("accounts.historyRequiredTitle") : title}</SheetTitle>
          {waitingForHistory && <SheetDescription>{t("accounts.historyRequiredDescription")}</SheetDescription>}
        </SheetHeader>
        <div className="overflow-y-auto">
          {waitingForHistory ? (
            <StartHistoryForm
              compact
              onCancel={onClose}
              onStarted={(nextOrigin) => {
                setStartedOrigin(nextOrigin);
                setStartedInSheet(true);
              }}
            />
          ) : action === "cash" ? (
            <CashBalanceForm record={record} valuation={valuation} historyStarted={historyReady} onDone={onRecorded} />
          ) : action === "position" ? (
            <ExistingPositionForm record={record} onDone={onRecorded} />
          ) : action === "simple" ? (
            <SimpleValueForm record={record} valuation={valuation} onDone={onRecorded} />
          ) : (
            <LockedChangeForm action={action} record={record} onDone={onRecorded} originOverride={startedOrigin} />
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}

function CashBalanceForm({
  record,
  valuation,
  historyStarted,
  onDone,
}: {
  record: AccountRecordDTO;
  valuation?: import("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models").AccountValuationDTO;
  historyStarted: boolean;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const currencies = useSupportedCurrencies();
  const appendCash = useAppendAccountCashValue();
  const [currency, setCurrency] = useState(record.account.defaultCurrency);
  const [amount, setAmount] = useState("");
  const [error, setError] = useState<string | undefined>();
  const options = currencies.data ?? [record.account.defaultCurrency];
  const currentCash = (valuation?.components ?? []).find((component) => !component.instrumentId && component.nativeCurrency === currency);

  const submit = () => {
    if (!amount.trim()) {
      setError(t("error.validation.notEmpty"));
      return;
    }
    setError(undefined);
    appendCash.mutate(
      { accountId: record.account.id, amount: amount.trim(), currency },
      {
        onSuccess: () => {
          toast.success(t("common.saved"));
          onDone();
        },
        onError: (cause) => setError(displayError(cause, t("accounts.saveError"))),
      },
    );
  };

  return (
    <div className="flex flex-col gap-4" role="form" aria-label={t("accounts.addCashBalance")}>
      <p className="text-sm text-muted-foreground">{historyStarted ? t("accounts.cashReconcileHint") : t("accounts.openingCashHint")}</p>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="cash-currency">{t("accounts.cashCurrency")}</Label>
        <NativeSelect id="cash-currency" value={currency} onChange={(event) => setCurrency(event.target.value)}>
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
        <Input id="cash-amount" inputMode="decimal" value={amount} onChange={(event) => setAmount(event.target.value)} autoFocus />
      </div>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <Button type="button" onClick={submit} disabled={appendCash.isPending}>
        {appendCash.isPending ? t("common.pending") : t("common.save")}
      </Button>
    </div>
  );
}

function ExistingPositionForm({ record, onDone }: { record: AccountRecordDTO; onDone: () => void }) {
  const { t } = useTranslation();
  const instruments = useInstruments();
  const createHolding = useCreateHolding();
  const createInstrument = useCreateInstrument();
  const refreshInstrument = useRefreshInstrument();
  const [instrumentId, setInstrumentId] = useState("");
  const [quantity, setQuantity] = useState("");
  const [unitCost, setUnitCost] = useState("");
  const unitCostRef = useRef("");
  const automaticUnitCost = useRef("");
  const [creatingInstrument, setCreatingInstrument] = useState(false);
  const [error, setError] = useState<string | undefined>();
  const selectedInstrument = (instruments.data ?? []).find((instrument) => instrument.id === instrumentId);
  const quote = useCurrentInstrumentQuote(instrumentId);

  useEffect(() => {
    if (quote.data && quote.data.instrumentId === instrumentId && (!unitCostRef.current || unitCostRef.current === automaticUnitCost.current)) {
      // An async market quote is a form default; preserve a user's override.
      setUnitCost(quote.data.unitPrice);
      unitCostRef.current = quote.data.unitPrice;
      automaticUnitCost.current = quote.data.unitPrice;
    }
  }, [instrumentId, quote.data]);

  const quantityPositive = (() => {
    const trimmed = quantity.trim();
    if (!trimmed) {
      return false;
    }
    return trimmed !== "0" && !/^0(?:\.0+)?$/.test(trimmed) && !trimmed.startsWith("-");
  })();

  const submit = () => {
    if (!instrumentId || !quantityPositive) {
      setError(t("accounts.quantityRequired"));
      return;
    }
    setError(undefined);
    createHolding.mutate(
      { accountId: record.account.id, instrumentId, quantity: quantity.trim(), unitCost: unitCost.trim() || undefined },
      {
        onSuccess: () => {
          toast.success(t("common.saved"));
          onDone();
        },
        onError: (cause) => setError(displayError(cause, t("accounts.saveError"))),
      },
    );
  };

  return (
    <div className="flex flex-col gap-4" role="form" aria-label={t("accounts.recordPosition")}>
      <p className="text-sm text-muted-foreground">{t("accounts.recordPositionHelp")}</p>
      {creatingInstrument ? (
        <InstrumentForm
          isSubmitting={createInstrument.isPending}
          submissionError={createInstrument.isError ? displayError(createInstrument.error, t("portfolio.createError")) : undefined}
          onSubmit={(request) => {
            createInstrument.mutate(request, {
              onSuccess: (created) => {
                setInstrumentId(created.id);
                setCreatingInstrument(false);
              },
            });
          }}
        />
      ) : (
        <>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="position-instrument">{t("history.instrument")}</Label>
            <NativeSelect id="position-instrument" value={instrumentId} onChange={(event) => { setInstrumentId(event.target.value); unitCostRef.current = ""; automaticUnitCost.current = ""; setUnitCost(""); }}>
              <option value="">{t("history.selectEmpty")}</option>
              {(instruments.data ?? []).filter((instrument) => !instrument.archivedAt).map((instrument) => (
                <option key={instrument.id} value={instrument.id}>
                  {instrument.name}
                </option>
              ))}
            </NativeSelect>
          </div>
          {selectedInstrument && <QuoteHint quote={quote.data} quoteLabel={quote.data ? t("portfolio.latestPrice", { value: formatAmount(quote.data.unitPrice, quote.data.currency) }) : undefined} manual={selectedInstrument.quoteSource === "manual"} onUpdate={() => refreshInstrument.mutate(selectedInstrument.id)} isUpdating={refreshInstrument.isPending} loading={quote.isLoading} error={refreshInstrument.error} />}
          <Button type="button" variant="ghost" size="sm" className="self-start" onClick={() => setCreatingInstrument(true)}>
            {t("accounts.createInstrument")}
          </Button>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="position-quantity">{t("accounts.quantity")}</Label>
            <Input id="position-quantity" inputMode="decimal" value={quantity} onChange={(event) => setQuantity(event.target.value)} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="position-unit-cost">{t("history.unitCostOptional")}</Label>
            <Input id="position-unit-cost" inputMode="decimal" value={unitCost} onChange={(event) => { unitCostRef.current = event.target.value; setUnitCost(event.target.value); }} />
          </div>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <Button type="button" onClick={submit} disabled={createHolding.isPending}>
            {createHolding.isPending ? t("common.pending") : t("accounts.recordPosition")}
          </Button>
        </>
      )}
    </div>
  );
}

function SimpleValueForm({
  record,
  valuation,
  onDone,
}: {
  record: AccountRecordDTO;
  valuation?: import("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models").AccountValuationDTO;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const appendValue = useAppendAccountValue();
  const valuationComponent = (valuation?.components ?? []).find((component) => !component.instrumentId && component.nativeAmount.trim() !== "");
  const currentAmount = record.latestValue?.amount.amount ?? valuationComponent?.nativeAmount;
  const currentCurrency = record.latestValue?.amount.currency ?? valuationComponent?.nativeCurrency ?? record.account.defaultCurrency;
  const amountRef = useRef(currentAmount ?? "");
  const automaticAmount = useRef(currentAmount ?? "");
  const [amount, setAmount] = useState(currentAmount ?? "");
  const [error, setError] = useState<string | undefined>();

  useEffect(() => {
    if (currentAmount !== undefined && (!amountRef.current || amountRef.current === automaticAmount.current)) {
      // Valuation data arrives asynchronously and supplies the initial form value.
      setAmount(currentAmount);
      amountRef.current = currentAmount;
      automaticAmount.current = currentAmount;
    }
  }, [currentAmount]);

  const submit = () => {
    if (!amount.trim()) {
      setError(t("error.validation.notEmpty"));
      return;
    }
    setError(undefined);
    appendValue.mutate(
      { id: record.account.id, amount: amount.trim() },
      {
        onSuccess: () => {
          toast.success(t("common.saved"));
          onDone();
        },
        onError: (cause) => setError(displayError(cause, t("accounts.saveError"))),
      },
    );
  };
  const unchanged = Boolean(
    currentAmount !== undefined &&
      amount.trim() &&
      sameCanonicalDecimal(amount.trim(), currentAmount) &&
      currentCurrency === record.account.defaultCurrency,
  );

  return (
    <div className="flex flex-col gap-4" role="form" aria-label={t("accounts.updateValue")}>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="simple-amount">{t("history.newValue")}</Label>
        <Input id="simple-amount" inputMode="decimal" value={amount} onChange={(event) => { amountRef.current = event.target.value; setAmount(event.target.value); }} autoFocus />
        <p className="text-xs text-muted-foreground">{record.account.defaultCurrency}</p>
      </div>
      {currentAmount !== undefined && <p className="text-xs text-muted-foreground">{t("accounts.currentBalance", { amount: formatAmount(currentAmount, currentCurrency) })}</p>}
      {unchanged && <p className="text-xs text-muted-foreground">{t("history.differentValueRequired")}</p>}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <Button type="button" onClick={submit} disabled={appendValue.isPending || unchanged}>
        {appendValue.isPending ? t("common.pending") : t("common.save")}
      </Button>
    </div>
  );
}

function LockedChangeForm({
  action,
  record,
  onDone,
  originOverride,
}: {
  action: Exclude<AccountAction, "cash" | "position" | "simple" | "settings" | null>;
  record: AccountRecordDTO;
  onDone: () => void;
  originOverride?: HistoryOriginDTO;
}) {
  const { t } = useTranslation();
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
          : ChangeCommandKind.ChangeTrade;
  const initial: ChangeCommandRequest = {
    ...emptyChangeRequest(kind, currency),
    accountId: record.account.id,
    settlementAccountId: record.account.id,
    fromAccountId: action === "transfer" ? record.account.id : undefined,
    side: action === "sell" ? "sell" : "buy",
    currency,
  };
  const lock: RecordChangeLock = {
    kind,
    hideKind: true,
    accountId: action === "buy" || action === "sell" ? undefined : record.account.id,
    settlementAccountId: action === "buy" || action === "sell" ? record.account.id : undefined,
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
              onChange={() => setMoneyKind(ChangeCommandKind.ChangeMoneyAdded)}
            />
            {t("history.kind.money_added")}
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="radio"
              name="money-direction"
              checked={moneyKind === ChangeCommandKind.ChangeMoneyRemoved}
              onChange={() => setMoneyKind(ChangeCommandKind.ChangeMoneyRemoved)}
            />
            {t("history.kind.money_removed")}
          </label>
        </div>
      )}
      <RecordChangeForm key={kind} initial={initial} lock={lock} originOverride={originOverride} onRecorded={onDone} />
    </div>
  );
}
