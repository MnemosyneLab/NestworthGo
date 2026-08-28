import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { LoadingState } from "@/components/layout/PageState";
import { useAccounts } from "@/queries/accounts";
import {
  useAllHoldingsFlat,
  useCreateInstrument,
  useCurrentFXQuote,
  useCurrentInstrumentQuote,
  useInstruments,
  useFXPreferences,
  useSetFXPreference,
} from "@/queries/investments";
import { useHistoryOrigin, usePreviewChange, usePreviewFixChange, useRecordChange, useFixChange } from "@/queries/history";
import { useSettings, useSupportedCurrencies } from "@/queries/settings";
import { useCatalog } from "@/queries/catalog";
import { useBootstrap } from "@/queries/household";
import { useRefreshFX, useRefreshInstrument } from "@/queries/marketdata";
import { InstrumentForm } from "@/features/investments/InstrumentForm";
import { ChangeCommandKind, type ChangeCommandRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";
import type { HistoryOriginDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";
import type { AccountRecordDTO, EndpointViewDTO, FXQuoteDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { currencyFractionDigits, divideCanonical, formatAmount, isPositiveCanonical, multiplyCanonical, sameCanonicalDecimal } from "@/lib/money";
import { displayError } from "@/lib/display";
import { formatTimestamp, localDateTimeInTimeZone, resolvedTimeZone } from "@/lib/time";
import { emptyChangeRequest } from "@/features/history/activityToCommand";
import { QuoteHint } from "@/features/history/QuoteHint";

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

type NamedOption = { id: string; name: string };

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
  accounts: NamedOption[];
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <NativeSelect id={id} value={value} onChange={(event) => onChange(event.target.value)}>
        {accounts.length === 0 ? <option value="" disabled>{t("common.noMatches")}</option> : (
          <>
            <option value="">{t("history.accountSelectEmpty")}</option>
            {accounts.map((account) => (
              <option key={account.id} value={account.id}>
                {account.name}
              </option>
            ))}
          </>
        )}
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
  options: NamedOption[];
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <NativeSelect id={id} value={value} onChange={(event) => onChange(event.target.value)}>
        {options.length === 0 ? <option value="" disabled>{t("common.noMatches")}</option> : (
          <>
            <option value="">{emptyLabel}</option>
            {options.map((option) => (
              <option key={option.id} value={option.id}>
                {option.name}
              </option>
            ))}
          </>
        )}
      </NativeSelect>
    </div>
  );
}

function MoneyFields({
  prefix,
  label,
  amount,
  currency,
  currencies,
  onAmount,
  onCurrency,
  disabledCurrency = false,
  allowEmpty = false,
}: {
  prefix: string;
  label: string;
  amount: string;
  currency: string;
  currencies: string[];
  onAmount: (value: string) => void;
  onCurrency?: (value: string) => void;
  disabledCurrency?: boolean;
  allowEmpty?: boolean;
}) {
  const { t } = useTranslation();
  const options = currencies.includes(currency) || !currency ? currencies : [currency, ...currencies];
  return (
    <div className="flex gap-2">
      <div className="flex flex-1 flex-col gap-1.5">
        <Label htmlFor={`${prefix}-amount`}>{label}</Label>
        <Input id={`${prefix}-amount`} inputMode="decimal" value={amount} onChange={(event) => onAmount(event.target.value)} />
      </div>
      <div className="flex w-28 flex-col gap-1.5">
        <Label htmlFor={`${prefix}-currency`}>{t("history.currency")}</Label>
        <NativeSelect
          id={`${prefix}-currency`}
          value={currency}
          disabled={disabledCurrency}
          onChange={(event) => onCurrency?.(event.target.value)}
        >
          {allowEmpty && <option value="">{t("history.selectEmpty")}</option>}
          {options.map((code) => (
            <option key={code} value={code}>
              {code}
            </option>
          ))}
        </NativeSelect>
      </div>
    </div>
  );
}

export type RecordChangeLock = {
  kind?: ChangeCommandKind;
  hideKind?: boolean;
  accountId?: string;
  settlementAccountId?: string;
};

function currencyFromRequest(request: ChangeCommandRequest | undefined): string | undefined {
  if (!request) {
    return undefined;
  }
  return request.currency || request.grossCurrency || request.newValueCurrency || request.sentCurrency || request.soldCurrency || request.principalCurrency || undefined;
}

function accountOptionName(record: AccountRecordDTO): string {
  return `${record.account.name} · ${record.account.defaultCurrency || ""}`.replace(/ · $/, "");
}

function quoteAmount(quote: FXQuoteDTO, amount: string, from: string, to: string): string {
  const places = currencyFractionDigits(to);
  if (quote.baseCurrency === from && quote.quoteCurrency === to) {
    return multiplyCanonical(amount, quote.rate, places);
  }
  if (quote.baseCurrency === to && quote.quoteCurrency === from) {
    return divideCanonical(amount, quote.rate, places);
  }
  return "";
}

function timeKey(date: string, clock: string): string {
  return `${date}T${clock}`;
}

/**
 * RecordChangeForm implements all ten change kinds. Defaults are deliberately
 * tracked as replaceable values: changing a source input updates a dependent
 * field only while that field is still blank or still equal to its last
 * automatic value. A field edited by the user is never silently overwritten.
 */
export function RecordChangeForm({
  onRecorded,
  initial,
  fixActivityId,
  lock,
  originalEffectiveAt,
  originOverride,
}: {
  onRecorded: () => void;
  initial?: ChangeCommandRequest;
  fixActivityId?: string;
  lock?: RecordChangeLock;
  originalEffectiveAt?: string;
  originOverride?: HistoryOriginDTO;
}) {
  const { t } = useTranslation();
  const bootstrap = useBootstrap();
  const currencies = useSupportedCurrencies();
  const fromInitial = currencyFromRequest(initial);
  if (!fromInitial && !bootstrap.isFetched) {
    return <LoadingState label={t("history.loading")} />;
  }
  const resolvedCurrency = fromInitial || bootstrap.data?.household?.baseCurrency || currencies.data?.[0];
  if (!resolvedCurrency) {
    return <LoadingState label={t("history.loading")} />;
  }
  return (
    <RecordChangeFormReady
      key={resolvedCurrency}
      onRecorded={onRecorded}
      initial={initial}
      fixActivityId={fixActivityId}
      lock={lock}
      originalEffectiveAt={originalEffectiveAt}
      originOverride={originOverride}
      defaultCurrency={resolvedCurrency}
    />
  );
}

function RecordChangeFormReady({
  onRecorded,
  initial,
  fixActivityId,
  lock,
  originalEffectiveAt,
  originOverride,
  defaultCurrency,
}: {
  onRecorded: () => void;
  initial?: ChangeCommandRequest;
  fixActivityId?: string;
  lock?: RecordChangeLock;
  originalEffectiveAt?: string;
  originOverride?: HistoryOriginDTO;
  defaultCurrency: string;
}) {
  const { t } = useTranslation();
  const accounts = useAccounts({});
  const instruments = useInstruments();
  const currencies = useSupportedCurrencies();
  const catalog = useCatalog();
  const settings = useSettings();
  const origin = useHistoryOrigin();
  const effectiveOrigin = originOverride ?? origin.data;
  const allAccountIds = (accounts.data ?? []).map((record) => record.account.id);
  const holdings = useAllHoldingsFlat(allAccountIds);
  const preview = usePreviewChange();
  const previewFix = usePreviewFixChange();
  const record = useRecordChange();
  const fix = useFixChange();
  const createInstrument = useCreateInstrument();
  const refreshInstrument = useRefreshInstrument();
  const refreshFX = useRefreshFX();
  const fxPreferences = useFXPreferences();
  const setFXPreference = useSetFXPreference();
  const [creatingInstrument, setCreatingInstrument] = useState(false);
  const [fxDriver, setFXDriver] = useState<"sold" | "bought">("sold");
  const currencyOptions = currencies.data ?? [defaultCurrency];

  const initialRequest: ChangeCommandRequest = (() => {
    const base = initial ?? emptyChangeRequest(lock?.kind ?? ChangeCommandKind.ChangeMoneyAdded, defaultCurrency);
    return {
      ...base,
      kind: lock?.kind ?? base.kind,
      accountId: lock?.accountId ?? base.accountId,
      settlementAccountId: lock?.settlementAccountId ?? base.settlementAccountId,
      fromAccountId:
        lock?.accountId && (lock.kind === ChangeCommandKind.ChangeCashTransfer || base.kind === ChangeCommandKind.ChangeCashTransfer)
          ? lock.accountId
          : base.fromAccountId,
    };
  })();
  const [request, setRequest] = useState<ChangeCommandRequest>(initialRequest);
  const autoValues = useRef<Record<string, string>>(
    initial
      ? Object.fromEntries(
          Object.entries(initialRequest)
            .filter(([, value]) => String(value ?? "") !== "")
            .map(([field]) => [field, "__user__"]),
        )
      : {},
  );
  const [previewResult, setPreviewResult] = useState<EndpointViewDTO[] | null>(null);

  const markAutomaticFields = (current: ChangeCommandRequest, fields: string[]) => {
    fields.forEach((field) => {
      if (autoValues.current[field] !== "__user__") {
        autoValues.current[field] = String(current[field as keyof ChangeCommandRequest] ?? "");
      }
    });
  };

  const patch = (next: Partial<ChangeCommandRequest>, automaticFields: string[] = []) => {
    setRequest((current) => {
      markAutomaticFields(current, automaticFields);
      Object.keys(next).forEach((field) => {
        if (!automaticFields.includes(field)) {
          const nextValue = String(next[field as keyof ChangeCommandRequest] ?? "").trim();
          if (nextValue === "") {
            delete autoValues.current[field];
          } else {
            autoValues.current[field] = "__user__";
          }
        }
      });
      return { ...current, ...next };
    });
    setPreviewResult(null);
  };

  const setAutomatic = (field: keyof ChangeCommandRequest, value: string) => {
    if (!value) {
      return;
    }
    setRequest((current) => {
      const currentValue = String(current[field] ?? "");
      const previousAutomatic = autoValues.current[field];
      if (currentValue !== "" && previousAutomatic !== currentValue) {
        return current;
      }
      autoValues.current[field] = value;
      return { ...current, [field]: value };
    });
  };

  const resetAutomaticTracking = (next: ChangeCommandRequest) => {
    autoValues.current = {};
    Object.keys(next).forEach((field) => {
      if (String(next[field as keyof ChangeCommandRequest] ?? "") !== "") {
        autoValues.current[field] = "__user__";
      }
    });
  };

  const accountRecords = useMemo(() => accounts.data ?? [], [accounts.data]);
  const accountById = useMemo(() => new Map(accountRecords.map((record) => [record.account.id, record])), [accountRecords]);
  const activeAccountRecords = accountRecords.filter((record) => !record.account.archivedAt);
  const holdingsAccountRecords = activeAccountRecords.filter((record) => record.account.trackingMode === "holdings");
  const valueAccountRecords = activeAccountRecords.filter((record) => record.account.trackingMode === "balance" || record.account.trackingMode === "manual_value");
  const debtAccountRecords = activeAccountRecords.filter((record) => record.account.balanceSheetRole === "liability" && record.account.trackingMode === "balance");
  const cashAccountRecords = activeAccountRecords.filter(
    (record) =>
      record.account.balanceSheetRole !== "liability" &&
      (record.account.trackingMode === "balance" || record.account.trackingMode === "holdings"),
  );
  const accountOptions = activeAccountRecords.map((record) => ({ id: record.account.id, name: accountOptionName(record) }));
  const holdingsAccountOptions = holdingsAccountRecords.map((record) => ({ id: record.account.id, name: accountOptionName(record) }));
  const valueAccountOptions = valueAccountRecords.map((record) => ({ id: record.account.id, name: accountOptionName(record) }));
  const debtAccountOptions = debtAccountRecords.map((record) => ({ id: record.account.id, name: accountOptionName(record) }));
  const cashAccountOptions = cashAccountRecords.map((record) => ({ id: record.account.id, name: accountOptionName(record) }));

  const holdingName = (holding: { instrumentName?: string; accountName?: string; quantity: string }) =>
    `${holding.instrumentName ?? t("portfolio.unknownInstrument")} · ${holding.accountName ?? t("history.unknownAccount")} · ${formatAmount(holding.quantity)}`;
  const holdingOptions = holdings.data.map((holding) => ({ id: holding.id, name: holdingName(holding) }));
  const selectedSettlement = request.settlementAccountId ?? "";
  const positiveSettlementHoldings = holdings.data.filter((holding) => holding.accountId === selectedSettlement && isPositiveCanonical(holding.quantity));
  const sellHoldingOptions = positiveSettlementHoldings.map((holding) => ({ id: holding.id, name: holdingName(holding) }));
  const activeInstrumentOptions = (instruments.data ?? [])
    .filter((instrument) => !instrument.archivedAt)
    .map((instrument) => ({ id: instrument.id, name: `${instrument.name} · ${instrument.quoteCurrency}` }));
  const selectedInstrument = (instruments.data ?? []).find((instrument) => instrument.id === request.instrumentId);
  const tradeOptions = request.side === "sell" ? sellHoldingOptions : activeInstrumentOptions;
  const tradeSelection = request.side === "sell" ? request.holdingId ?? "" : request.instrumentId ?? "";

  const matchingHoldingId = (accountId: string, instrumentId: string) =>
    holdings.data.find((holding) => holding.accountId === accountId && holding.instrumentId === instrumentId)?.id ?? "";

  const kind = request.kind;
  const showReason = kind === ChangeCommandKind.ChangeMoneyAdded || kind === ChangeCommandKind.ChangeMoneyRemoved || kind === ChangeCommandKind.ChangeValueUpdate;
  const reasonOptions =
    kind === ChangeCommandKind.ChangeMoneyRemoved
      ? (catalog.data?.moneyOutReasons ?? [])
      : kind === ChangeCommandKind.ChangeValueUpdate
        ? (catalog.data?.valueUpdateReasons ?? [])
        : (catalog.data?.moneyInReasons ?? []);
  const tradeSides = catalog.data?.tradeSides ?? [];
  const currentValue = request.accountId ? accountById.get(request.accountId)?.latestValue : undefined;
  const fxPreference = (() => {
    const sold = request.soldCurrency ?? "";
    const bought = request.boughtCurrency ?? "";
    return (fxPreferences.data ?? []).find(
      (preference) => Boolean(sold && bought) && [preference.currencyA, preference.currencyB].sort().join("/") === [sold, bought].sort().join("/"),
    );
  })();
  const instrumentQuote = useCurrentInstrumentQuote(request.instrumentId ?? "");
  const fxQuote = useCurrentFXQuote(request.soldCurrency ?? "", request.boughtCurrency ?? "");
  const fxSourceAmount = fxDriver === "sold" ? request.sold : request.bought;

  useEffect(() => {
    if (kind !== ChangeCommandKind.ChangeTrade || !request.quantity?.trim() || !instrumentQuote.data || !request.grossCurrency) {
      return;
    }
    setAutomatic("gross", multiplyCanonical(request.quantity.trim(), instrumentQuote.data.unitPrice, currencyFractionDigits(request.grossCurrency)));
  }, [instrumentQuote.data, kind, request.grossCurrency, request.quantity]);

  useEffect(() => {
    if (kind !== ChangeCommandKind.ChangeFXConversion || !fxQuote.data) {
      return;
    }
    const soldCurrency = request.soldCurrency ?? "";
    const boughtCurrency = request.boughtCurrency ?? "";
    if (!soldCurrency || !boughtCurrency || soldCurrency === boughtCurrency) {
      return;
    }
    if (fxDriver === "sold" && fxSourceAmount?.trim()) {
      setAutomatic("bought", quoteAmount(fxQuote.data, fxSourceAmount.trim(), soldCurrency, boughtCurrency));
    } else if (fxDriver === "bought" && fxSourceAmount?.trim()) {
      setAutomatic("sold", quoteAmount(fxQuote.data, fxSourceAmount.trim(), boughtCurrency, soldCurrency));
    }
  }, [fxDriver, fxQuote.data, fxSourceAmount, kind, request.boughtCurrency, request.soldCurrency]);

  useEffect(() => {
    if (kind !== ChangeCommandKind.ChangeCashTransfer || !request.sentCurrency || !request.receivedCurrency) {
      return;
    }
    if (request.sentCurrency !== request.receivedCurrency) {
      setRequest((current) => {
        const received = current.received ?? "";
        if (!received || autoValues.current.received !== received) {
          return current;
        }
        delete autoValues.current.received;
        return { ...current, received: "" };
      });
      return;
    }
    if (!request.sent?.trim()) {
      return;
    }
    setAutomatic("received", request.sent.trim());
  }, [kind, request.receivedCurrency, request.sent, request.sentCurrency]);

  useEffect(() => {
    if (kind === ChangeCommandKind.ChangeFXConversion && request.feeCurrency !== request.soldCurrency) {
      // Keep the locked fee currency aligned with the editable source currency.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setRequest((current) => ({ ...current, feeCurrency: current.soldCurrency ?? "" }));
    }
    if (kind === ChangeCommandKind.ChangeTrade && request.feeCurrency !== request.grossCurrency) {
      setRequest((current) => ({ ...current, feeCurrency: current.grossCurrency ?? "" }));
    }
    if (kind === ChangeCommandKind.ChangeDebtPayment && request.interestOrFeeCurrency !== request.principalCurrency) {
      setRequest((current) => ({ ...current, interestOrFeeCurrency: current.principalCurrency ?? "" }));
    }
  }, [kind, request.feeCurrency, request.grossCurrency, request.interestOrFeeCurrency, request.principalCurrency, request.soldCurrency]);

  useEffect(() => {
    if (kind === ChangeCommandKind.ChangeTrade && request.side === "sell" && request.holdingId && !positiveSettlementHoldings.some((holding) => holding.id === request.holdingId)) {
      // A refreshed holdings list can invalidate a previously selected settlement holding.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setRequest((current) => ({ ...current, holdingId: "", instrumentId: "" }));
    }
  }, [kind, positiveSettlementHoldings, request.holdingId, request.side]);

  useEffect(() => {
    if (fixActivityId || request.effectiveLocalDate || request.effectiveLocalTime || !effectiveOrigin) {
      return;
    }
    const local = localDateTimeInTimeZone(effectiveOrigin.timezone);
    if (local) {
      // Fix forms show the original event time read-only; new forms default once from the origin.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setRequest((current) => ({ ...current, effectiveLocalDate: local.date, effectiveLocalTime: local.time, effectiveAt: "" }));
    }
  }, [effectiveOrigin, fixActivityId, request.effectiveLocalDate, request.effectiveLocalTime]);

  const originZone = resolvedTimeZone(effectiveOrigin?.timezone || settings.data?.timezone);
  const originLocal = effectiveOrigin ? localDateTimeInTimeZone(originZone, new Date(effectiveOrigin.startedAt)) : undefined;
  const nowLocal = localDateTimeInTimeZone(originZone);
  const localDate = request.effectiveLocalDate ?? "";
  const localTime = request.effectiveLocalTime ?? "";
  const localPairPresent = Boolean(localDate && localTime);
  const localPairPartial = Boolean(localDate) !== Boolean(localTime);
  const localPairKey = localPairPresent ? timeKey(localDate, localTime) : "";
  const timeError = fixActivityId
    ? undefined
    : localPairPartial
      ? t("history.effectiveDateTimeTogether")
      : !localPairPresent
        ? t("history.effectiveDateTimeRequired")
        : !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(localPairKey)
          ? t("history.effectiveDateTimeInvalid")
          : originLocal && localPairKey < timeKey(originLocal.date, originLocal.time)
            ? t("history.effectiveDateTimeBeforeOrigin")
            : nowLocal && localPairKey > timeKey(nowLocal.date, nowLocal.time)
              ? t("history.effectiveDateTimeFuture")
              : undefined;

  const valueUnchanged = Boolean(
    !fixActivityId &&
      kind === ChangeCommandKind.ChangeValueUpdate &&
      currentValue &&
      request.newValue?.trim() &&
      sameCanonicalDecimal(request.newValue.trim(), currentValue.amount.amount) &&
      (request.newValueCurrency ?? "") === currentValue.amount.currency,
  );

  const hasRequiredFields = (() => {
    switch (kind) {
      case ChangeCommandKind.ChangeMoneyAdded:
      case ChangeCommandKind.ChangeMoneyRemoved:
        return Boolean(request.accountId && request.amount?.trim() && request.currency);
      case ChangeCommandKind.ChangeCashTransfer:
        return Boolean(request.fromAccountId && request.toAccountId && request.fromAccountId !== request.toAccountId && request.sent?.trim() && request.received?.trim() && request.sentCurrency && request.receivedCurrency);
      case ChangeCommandKind.ChangeFXConversion:
        return Boolean(request.accountId && request.sold?.trim() && request.bought?.trim() && request.soldCurrency && request.boughtCurrency && request.soldCurrency !== request.boughtCurrency);
      case ChangeCommandKind.ChangePositionTransfer:
        return Boolean(request.fromHoldingId && request.toHoldingId && request.fromHoldingId !== request.toHoldingId && request.quantity?.trim());
      case ChangeCommandKind.ChangePositionAdjustment:
        return Boolean(request.holdingId && request.quantity?.trim());
      case ChangeCommandKind.ChangeTrade:
        return Boolean(request.settlementAccountId && request.instrumentId && request.side && request.quantity?.trim() && request.gross?.trim() && (request.side !== "sell" || request.holdingId));
      case ChangeCommandKind.ChangeValueUpdate:
        return Boolean(request.accountId && request.newValue?.trim() && request.newValueCurrency && !valueUnchanged);
      case ChangeCommandKind.ChangeDebtDraw:
      case ChangeCommandKind.ChangeDebtPayment:
        return Boolean(request.debtAccountId && request.cashAccountId && request.debtAccountId !== request.cashAccountId && request.principal?.trim() && request.principalCurrency);
      default:
        return false;
    }
  })();
  const canPreview = hasRequiredFields && !timeError && !creatingInstrument && !accounts.isLoading && !instruments.isLoading && !holdings.isLoading && (!origin.isLoading || Boolean(originOverride));

  const buildRequest = (): ChangeCommandRequest => ({
    ...request,
    feeCurrency: request.feeCurrency || request.soldCurrency || request.grossCurrency,
    interestOrFeeCurrency: request.interestOrFeeCurrency || request.principalCurrency,
    effectiveLocalDate: fixActivityId ? "" : request.effectiveLocalDate,
    effectiveLocalTime: fixActivityId ? "" : request.effectiveLocalTime,
    effectiveAt: "",
  });

  const runPreview = () => {
    if (!canPreview) {
      return;
    }
    const command = buildRequest();
    if (fixActivityId) {
      previewFix.mutate({ activityId: fixActivityId, replacement: command }, { onSuccess: (result) => setPreviewResult(result.resulting) });
      return;
    }
    preview.mutate(command, { onSuccess: (result) => setPreviewResult(result.resulting) });
  };

  const runConfirm = () => {
    const command = buildRequest();
    if (fixActivityId) {
      fix.mutate(
        { activityId: fixActivityId, replacement: command },
        {
          onSuccess: () => {
            setPreviewResult(null);
            onRecorded();
          },
        },
      );
      return;
    }
    record.mutate(command, {
      onSuccess: () => {
        setPreviewResult(null);
        setRequest(emptyChangeRequest(ChangeCommandKind.ChangeMoneyAdded, defaultCurrency));
        onRecorded();
      },
    });
  };

  const error = preview.error ?? previewFix.error ?? record.error ?? fix.error;
  const previewPending = fixActivityId ? previewFix.isPending : preview.isPending;
  const confirmPending = fixActivityId ? fix.isPending : record.isPending;
  const selectedValueCurrency = accountById.get(request.accountId ?? "")?.account.defaultCurrency ?? currentValue?.amount.currency ?? defaultCurrency;
  const selectedCashAccountCurrency = accountById.get(request.cashAccountId ?? "")?.account.defaultCurrency ?? defaultCurrency;
  const fxPairReady = Boolean(request.soldCurrency && request.boughtCurrency && request.soldCurrency !== request.boughtCurrency);

  const handleAccountWithCurrency = (accountId: string, fields: Partial<ChangeCommandRequest>, automaticFields: string[] = []) => {
    const accountCurrency = accountById.get(accountId)?.account.defaultCurrency || defaultCurrency;
    patch({ ...fields, accountId, currency: accountCurrency, newValueCurrency: accountCurrency }, automaticFields);
  };

  const handleFXAccount = (accountId: string) => {
    const accountCurrency = accountById.get(accountId)?.account.defaultCurrency || defaultCurrency;
    setFXDriver("sold");
    patch(
      {
        accountId,
        soldCurrency: accountCurrency,
        feeCurrency: accountCurrency,
        ...(accountCurrency === request.boughtCurrency ? { bought: "", boughtCurrency: "" } : {}),
      },
      accountCurrency === request.boughtCurrency ? ["bought"] : [],
    );
  };

  const handleTransferAccount = (field: "fromAccountId" | "toAccountId", currencyField: "sentCurrency" | "receivedCurrency", accountId: string) => {
    const accountCurrency = accountById.get(accountId)?.account.defaultCurrency || defaultCurrency;
    patch({ [field]: accountId, [currencyField]: accountCurrency } as Partial<ChangeCommandRequest>, [currencyField]);
  };

  const handleCashDebtAccount = (cashAccountId: string) => {
    const accountCurrency = accountById.get(cashAccountId)?.account.defaultCurrency || defaultCurrency;
    patch(
      { cashAccountId, principalCurrency: accountCurrency, interestOrFeeCurrency: accountCurrency },
      ["principalCurrency", "interestOrFeeCurrency"],
    );
  };

  const updateFX = () => {
    if (!fxPairReady) {
      return;
    }
    const variables = { currencyA: request.soldCurrency!, currencyB: request.boughtCurrency! };
    if (!fxPreference) {
      setFXPreference.mutate(
        { ...variables, source: "provider" },
        { onSuccess: () => refreshFX.mutate(variables) },
      );
      return;
    }
    refreshFX.mutate(variables);
  };

  return (
    <div className="flex flex-col gap-4" role="form" aria-label={t("history.formLabel")}>
      {!lock?.hideKind && (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="change-kind">{t("history.changeType")}</Label>
          <NativeSelect
            id="change-kind"
            value={kind}
            onChange={(event) => {
              const nextKind = event.target.value as ChangeCommandKind;
              const next = emptyChangeRequest(nextKind, defaultCurrency);
              const locked = {
                ...next,
                accountId: lock?.accountId ?? next.accountId,
                settlementAccountId: lock?.settlementAccountId ?? next.settlementAccountId,
                fromAccountId: lock?.accountId && nextKind === ChangeCommandKind.ChangeCashTransfer ? lock.accountId : next.fromAccountId,
              };
              resetAutomaticTracking(locked);
              setRequest(locked);
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
      )}

      {(kind === ChangeCommandKind.ChangeMoneyAdded || kind === ChangeCommandKind.ChangeMoneyRemoved) && (
        <>
          {!lock?.accountId && <AccountSelect id="change-account" label={t("history.accountSelect")} value={request.accountId ?? ""} onChange={(accountId) => handleAccountWithCurrency(accountId, {}, ["amount"])} accounts={accountOptions} />}
          <MoneyFields prefix="change" label={t("history.amount")} amount={request.amount ?? ""} currency={request.currency ?? defaultCurrency} currencies={currencyOptions} onAmount={(amount) => patch({ amount })} onCurrency={(currency) => patch({ currency })} />
        </>
      )}

      {kind === ChangeCommandKind.ChangeValueUpdate && (
        <>
          {!lock?.accountId && (
            <AccountSelect
              id="change-account"
              label={t("history.accountSelect")}
              value={request.accountId ?? ""}
              onChange={(accountId) => {
                const latest = accountById.get(accountId)?.latestValue;
                const accountCurrency = accountById.get(accountId)?.account.defaultCurrency || defaultCurrency;
                patch({ accountId, newValue: latest?.amount.amount ?? "", newValueCurrency: accountCurrency }, ["newValue"]);
              }}
              accounts={valueAccountOptions}
            />
          )}
          <MoneyFields prefix="change" label={t("history.newValue")} amount={request.newValue ?? ""} currency={request.newValueCurrency ?? selectedValueCurrency} currencies={currencyOptions} disabledCurrency onAmount={(newValue) => patch({ newValue })} />
          {currentValue && <p className="text-xs text-muted-foreground">{t("history.currentValue", { value: formatAmount(currentValue.amount.amount, currentValue.amount.currency) })}</p>}
          {valueUnchanged && <p className="text-xs text-muted-foreground">{t("history.differentValueRequired")}</p>}
        </>
      )}

      {kind === ChangeCommandKind.ChangeCashTransfer && (
        <>
          {!lock?.accountId && <AccountSelect id="change-from-account" label={t("history.fromAccount")} value={request.fromAccountId ?? ""} onChange={(fromAccountId) => handleTransferAccount("fromAccountId", "sentCurrency", fromAccountId)} accounts={accountOptions} />}
          <AccountSelect id="change-to-account" label={t("history.toAccount")} value={request.toAccountId ?? ""} onChange={(toAccountId) => handleTransferAccount("toAccountId", "receivedCurrency", toAccountId)} accounts={accountOptions} />
          <MoneyFields prefix="change-sent" label={t("history.sent")} amount={request.sent ?? ""} currency={request.sentCurrency ?? defaultCurrency} currencies={currencyOptions} onAmount={(sent) => patch({ sent })} onCurrency={(sentCurrency) => patch({ sentCurrency })} />
          <MoneyFields prefix="change-received" label={t("history.received")} amount={request.received ?? ""} currency={request.receivedCurrency ?? request.sentCurrency ?? defaultCurrency} currencies={currencyOptions} onAmount={(received) => patch({ received })} onCurrency={(receivedCurrency) => patch({ receivedCurrency })} />
        </>
      )}

      {kind === ChangeCommandKind.ChangeFXConversion && (
        <>
          {!lock?.accountId && <AccountSelect id="change-account" label={t("history.accountSelect")} value={request.accountId ?? ""} onChange={handleFXAccount} accounts={holdingsAccountOptions} />}
          <MoneyFields
            prefix="change-sold"
            label={t("history.sold")}
            amount={request.sold ?? ""}
            currency={request.soldCurrency ?? defaultCurrency}
            currencies={currencyOptions}
            onAmount={(sold) => {
              if (sold.trim()) {
                setFXDriver("sold");
                patch({ sold }, ["bought"]);
              } else if (request.bought?.trim()) {
                setFXDriver("bought");
                patch({ sold });
              } else {
                setFXDriver("sold");
                patch({ sold });
              }
            }}
            onCurrency={(soldCurrency) => {
              setFXDriver("sold");
              if (soldCurrency === request.boughtCurrency) {
                patch({ soldCurrency, boughtCurrency: "", bought: "" }, ["bought"]);
                return;
              }
              patch({ soldCurrency }, ["bought"]);
            }}
          />
          <MoneyFields
            prefix="change-bought"
            label={t("history.bought")}
            amount={request.bought ?? ""}
            currency={request.boughtCurrency ?? ""}
            currencies={currencyOptions}
            allowEmpty
            onAmount={(bought) => {
              if (bought.trim()) {
                setFXDriver("bought");
                patch({ bought }, ["sold"]);
              } else if (request.sold?.trim()) {
                setFXDriver("sold");
                patch({ bought });
              } else {
                setFXDriver("bought");
                patch({ bought });
              }
            }}
            onCurrency={(boughtCurrency) => {
              setFXDriver("bought");
              if (boughtCurrency === request.soldCurrency) {
                patch({ boughtCurrency: "", bought: "" }, ["sold"]);
                return;
              }
              patch({ boughtCurrency }, ["sold"]);
            }}
          />
          {fxPairReady && <QuoteHint
            quote={fxQuote.data}
            quoteLabel={fxQuote.data ? t("marketData.latestRate", { base: fxQuote.data.baseCurrency, rate: formatAmount(fxQuote.data.rate), quote: fxQuote.data.quoteCurrency }) : undefined}
            manual={fxPreference?.sourceKind === "manual"}
            onUpdate={updateFX}
            isUpdating={setFXPreference.isPending || refreshFX.isPending}
            loading={fxQuote.isLoading}
            error={setFXPreference.error ?? refreshFX.error}
          />}
          <MoneyFields prefix="change-fx-fee" label={t("history.fxFee")} amount={request.fee ?? ""} currency={request.feeCurrency ?? request.soldCurrency ?? defaultCurrency} currencies={currencyOptions} disabledCurrency onAmount={(fee) => patch({ fee })} />
        </>
      )}

      {kind === ChangeCommandKind.ChangePositionTransfer && (
        <>
          <OptionSelect id="change-from-holding" label={t("history.fromHolding")} value={request.fromHoldingId ?? ""} emptyLabel={t("history.selectEmpty")} options={holdingOptions} onChange={(fromHoldingId) => patch({ fromHoldingId })} />
          <OptionSelect id="change-to-holding" label={t("history.toHolding")} value={request.toHoldingId ?? ""} emptyLabel={t("history.selectEmpty")} options={holdingOptions} onChange={(toHoldingId) => patch({ toHoldingId })} />
          <div className="flex flex-col gap-1.5"><Label htmlFor="change-quantity">{t("history.quantity")}</Label><Input id="change-quantity" inputMode="decimal" value={request.quantity ?? ""} onChange={(event) => patch({ quantity: event.target.value })} /></div>
        </>
      )}

      {kind === ChangeCommandKind.ChangePositionAdjustment && (
        <>
          <OptionSelect id="change-holding" label={t("history.holding")} value={request.holdingId ?? ""} emptyLabel={t("history.selectEmpty")} options={holdingOptions} onChange={(holdingId) => patch({ holdingId })} />
          <div className="flex flex-col gap-2" role="radiogroup" aria-label={t("history.addedRemovedHint")}>
            <label className="flex items-center gap-2 text-sm"><input type="radio" name="position-direction" checked={request.added === true} onChange={() => patch({ added: true })} /> {t("history.added")}</label>
            <label className="flex items-center gap-2 text-sm"><input type="radio" name="position-direction" checked={request.added === false} onChange={() => patch({ added: false, unitCost: "" })} /> {t("history.removed")}</label>
          </div>
          <div className="flex flex-col gap-1.5"><Label htmlFor="change-quantity">{t("history.quantity")}</Label><Input id="change-quantity" inputMode="decimal" value={request.quantity ?? ""} onChange={(event) => patch({ quantity: event.target.value })} /></div>
          {request.added && <div className="flex flex-col gap-1.5"><Label htmlFor="change-unit-cost">{t("history.unitCostOptional")}</Label><Input id="change-unit-cost" inputMode="decimal" value={request.unitCost ?? ""} onChange={(event) => patch({ unitCost: event.target.value })} /></div>}
        </>
      )}

      {kind === ChangeCommandKind.ChangeTrade && (
        <>
          {!lock?.settlementAccountId && <AccountSelect id="change-settlement-account" label={t("history.settlementAccount")} value={request.settlementAccountId ?? ""} onChange={(settlementAccountId) => patch({ settlementAccountId, holdingId: request.side === "sell" ? "" : matchingHoldingId(settlementAccountId, request.instrumentId ?? "") }, ["gross"])} accounts={holdingsAccountOptions} />}
          <OptionSelect
            id="change-instrument"
            label={t("history.instrument")}
            value={tradeSelection}
            emptyLabel={t("history.selectEmpty")}
            options={tradeOptions}
            onChange={(selection) => {
              if (request.side === "sell") {
                const holding = holdings.data.find((candidate) => candidate.id === selection);
                const instrument = (instruments.data ?? []).find((candidate) => candidate.id === holding?.instrumentId);
                patch({ holdingId: selection, instrumentId: holding?.instrumentId ?? "", grossCurrency: instrument?.quoteCurrency ?? defaultCurrency, feeCurrency: instrument?.quoteCurrency ?? defaultCurrency }, ["gross"]);
                return;
              }
              const instrument = (instruments.data ?? []).find((candidate) => candidate.id === selection);
              const currency = instrument?.quoteCurrency ?? defaultCurrency;
              patch({ instrumentId: selection, holdingId: matchingHoldingId(request.settlementAccountId ?? "", selection), grossCurrency: currency, feeCurrency: currency }, ["gross"]);
            }}
          />
          {selectedInstrument && <QuoteHint quote={instrumentQuote.data} quoteLabel={instrumentQuote.data ? t("portfolio.latestPrice", { value: formatAmount(instrumentQuote.data.unitPrice, instrumentQuote.data.currency) }) : undefined} manual={selectedInstrument.quoteSource === "manual"} onUpdate={() => refreshInstrument.mutate(selectedInstrument.id)} isUpdating={refreshInstrument.isPending} loading={instrumentQuote.isLoading} error={refreshInstrument.error} />}
          {creatingInstrument ? (
            <InstrumentForm
              isSubmitting={createInstrument.isPending}
              submissionError={createInstrument.isError ? displayError(createInstrument.error, t("portfolio.createError")) : undefined}
              onSubmit={(instrumentRequest) => {
                createInstrument.mutate(instrumentRequest, {
                  onSuccess: (created) => {
                    const currency = created.quoteCurrency ?? defaultCurrency;
                    patch({ instrumentId: created.id, holdingId: matchingHoldingId(request.settlementAccountId ?? "", created.id), grossCurrency: currency, feeCurrency: currency }, ["gross"]);
                    setCreatingInstrument(false);
                  },
                });
              }}
            />
          ) : <Button type="button" variant="ghost" size="sm" className="self-start" onClick={() => setCreatingInstrument(true)}>{t("accounts.createInstrument")}</Button>}
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="change-side">{t("history.side")}</Label>
            <NativeSelect id="change-side" value={request.side ?? "buy"} onChange={(event) => patch({ side: event.target.value, holdingId: "", instrumentId: "", gross: "", grossCurrency: defaultCurrency, fee: "", feeCurrency: defaultCurrency }, ["gross"])}>
              {tradeSides.map((side) => <option key={side} value={side}>{t(`history.${side}`)}</option>)}
            </NativeSelect>
          </div>
          <div className="flex flex-col gap-1.5"><Label htmlFor="change-quantity">{t("history.quantity")}</Label><Input id="change-quantity" inputMode="decimal" value={request.quantity ?? ""} onChange={(event) => patch({ quantity: event.target.value }, ["gross"])} /></div>
          <MoneyFields prefix="change-gross" label={t("history.grossTotal")} amount={request.gross ?? ""} currency={request.grossCurrency ?? defaultCurrency} currencies={currencyOptions} disabledCurrency={Boolean(selectedInstrument)} onAmount={(gross) => patch({ gross })} onCurrency={selectedInstrument ? undefined : (grossCurrency) => patch({ grossCurrency }, ["gross"])} />
          <MoneyFields prefix="change-fee" label={t("history.feeOptional")} amount={request.fee ?? ""} currency={request.feeCurrency ?? request.grossCurrency ?? defaultCurrency} currencies={currencyOptions} disabledCurrency onAmount={(fee) => patch({ fee })} />
        </>
      )}

      {(kind === ChangeCommandKind.ChangeDebtDraw || kind === ChangeCommandKind.ChangeDebtPayment) && (
        <>
          <AccountSelect id="change-debt-account" label={t("history.debtAccount")} value={request.debtAccountId ?? ""} onChange={(debtAccountId) => patch({ debtAccountId }, ["principal"])} accounts={debtAccountOptions} />
          <AccountSelect id="change-cash-account" label={t("history.cashAccount")} value={request.cashAccountId ?? ""} onChange={handleCashDebtAccount} accounts={cashAccountOptions} />
          <MoneyFields prefix="change-principal" label={t("history.principal")} amount={request.principal ?? ""} currency={request.principalCurrency ?? selectedCashAccountCurrency} currencies={currencyOptions} disabledCurrency onAmount={(principal) => patch({ principal })} />
          {kind === ChangeCommandKind.ChangeDebtPayment && <MoneyFields prefix="change-interest" label={t("history.interestOptional")} amount={request.interestOrFee ?? ""} currency={request.interestOrFeeCurrency ?? request.principalCurrency ?? defaultCurrency} currencies={currencyOptions} disabledCurrency onAmount={(interestOrFee) => patch({ interestOrFee })} />}
        </>
      )}

      {showReason && <div className="flex flex-col gap-1.5"><Label htmlFor="change-reason">{t("history.reasonLabel")}</Label><NativeSelect id="change-reason" value={request.reason ?? "other"} onChange={(event) => patch({ reason: event.target.value })}>{reasonOptions.map((reason) => <option key={reason} value={reason}>{t(`history.reason.${reason}`)}</option>)}</NativeSelect></div>}

      {!fixActivityId ? (
        <>
          <div className="grid gap-2 sm:grid-cols-2">
            <div className="flex flex-col gap-1.5"><Label htmlFor="change-effective-date">{t("history.effectiveDate")}</Label><Input id="change-effective-date" type="date" value={localDate} onChange={(event) => patch({ effectiveLocalDate: event.target.value, effectiveAt: "" })} /></div>
            <div className="flex flex-col gap-1.5"><Label htmlFor="change-effective-time">{t("history.effectiveTime")}</Label><Input id="change-effective-time" type="time" value={localTime} onChange={(event) => patch({ effectiveLocalTime: event.target.value, effectiveAt: "" })} /></div>
          </div>
          <p className="text-xs text-muted-foreground">{t("history.effectiveDateTimeHelp", { timezone: originZone })}</p>
        </>
      ) : (
        <div className="flex flex-col gap-1.5"><Label htmlFor="change-original-effective-at">{t("history.originalEffectiveAt")}</Label><Input id="change-original-effective-at" readOnly value={originalEffectiveAt ? formatTimestamp(originalEffectiveAt, effectiveOrigin?.timezone) : ""} /><p className="text-xs text-muted-foreground">{t("history.fixTimestampHelp")}</p></div>
      )}
      {timeError && <p role="alert" className="text-sm text-destructive">{timeError}</p>}

      <div className="flex flex-col gap-1.5"><Label htmlFor="change-note">{t("history.note")}</Label><Input id="change-note" value={request.note ?? ""} onChange={(event) => patch({ note: event.target.value || null })} placeholder={t("history.notePlaceholder")} /></div>

      {(accounts.isError || instruments.isError || holdings.isError) && <div role="alert" className="flex flex-wrap items-center gap-2 text-sm text-destructive"><span>{t("history.dependenciesError")}</span><Button type="button" variant="outline" size="sm" onClick={() => void Promise.all([accounts.refetch(), instruments.refetch(), holdings.refetch()])}>{t("common.retryAction")}</Button></div>}
      {error && <p role="alert" className="text-sm text-destructive">{displayError(error, t("history.actionError"))}</p>}

      {previewResult && <div role="status" className="flex flex-col gap-1 rounded-md border border-border bg-muted p-3 text-sm"><p className="font-medium">{t("history.previewTitle")}</p><p className="text-muted-foreground">{t("history.previewDescription")}</p>{previewResult.map((endpoint, index) => <p key={index}>{endpoint.name}: {endpoint.amount ? formatAmount(endpoint.amount, endpoint.currency) : formatAmount(endpoint.quantity ?? "0")}</p>)}</div>}

      <div className="flex gap-2">
        {!previewResult ? <Button type="button" onClick={runPreview} disabled={!canPreview || previewPending}>{previewPending ? t("common.pending") : t("common.preview")}</Button> : <Button type="button" onClick={runConfirm} disabled={confirmPending}>{confirmPending ? t("common.pending") : t("common.confirm")}</Button>}
      </div>
    </div>
  );
}
