import { useInstrumentHoldingGroups } from "@/queries/analytics";
import { HoldingGroupsList } from "@/features/investments/HoldingGroupsList";
import { AccountReconciliation } from "@/features/reconciliation/AccountReconciliation";
import { hasUnresolvedSheetConfirmation } from "@/components/ui/sheet-confirmation";
import { AnalyzeMenu } from "@/features/insights/AnalyzeMenu";
import { useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowLeft } from "lucide-react";
import { Button, buttonVariants } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { ActionMenu } from "@/components/ui/action-menu";
import { MoneyText } from "@/components/ui/money-text";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Sheet, SheetContent, SheetHeader, SheetFooter, SheetTitle } from "@/components/ui/sheet";
import { PageIntro } from "@/components/layout/PageHeader";
import { PageChrome } from "@/components/layout/PageChrome";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { AccountForm, type AccountFormExtras } from "@/features/accounts/AccountForm";
import { AccountActionSheet, type AccountAction } from "@/features/accounts/AccountActionSheets";
import { useAccountCashValues, useArchiveAccount, useUpdateAccount, toUpdateAccountRequest } from "@/queries/accounts";
import { AccountProductsSection } from "@/features/liquidity/AccountProductsSection";
import { useHistoryOrigin } from "@/queries/history";
import { compareCanonical, formatAmount, formatSignedMoney } from "@/lib/money";
import { accountDisplayMoney } from "@/features/accounts/accountDisplayMoney";
import { componentHouseholdAmount, sortValuationComponents } from "@/features/accounts/accountComposition";
import { AccountCompositionPanel } from "./AccountCompositionPanel";
import { formatTimestamp } from "@/lib/time";
import { useSettings } from "@/queries/settings";
import { displayEnum, displayError } from "@/lib/display";
import { cn } from "@/lib/utils";
import { toast } from "sonner";
import type { AccountCashValueDTO, AccountRecordDTO, AccountValuationDTO, ValuationComponentDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import type { CreateAccountRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account/models";
import { AccountIcon } from "@/components/icons/AccountIcon";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";

function cashComponents(valuation?: AccountValuationDTO): ValuationComponentDTO[] {
  const householdCurrency = valuation?.baseValue?.currency ?? "";
  return sortValuationComponents(
    (valuation?.components ?? []).filter((component) => !component.instrumentId),
    householdCurrency,
  );
}

type AccountTranslator = (key: string, options?: Record<string, unknown>) => string;

function cashActivityLabel(t: AccountTranslator, value: AccountCashValueDTO): string {
  const kind = displayEnum(t, "history.kind", value.activityKind);
  if (kind) {
    return kind;
  }
  return value.observationKind === "event"
    ? t("accounts.cashHistoryActivity")
    : t("accounts.cashHistoryOpeningBalance");
}

function cashChangeLabel(value: AccountCashValueDTO): string {
  if (!value.change) {
    return "—";
  }
  return formatSignedMoney(value.change.amount, value.change.currency).replace(/^-/, "−");
}

function ComponentAmounts({
  component,
  householdCurrency,
  pendingLabel,
}: {
  component: ValuationComponentDTO;
  householdCurrency: string;
  pendingLabel: string;
}) {
  const native = component.nativeAmount
    ? formatAmount(component.nativeAmount, component.nativeCurrency)
    : pendingLabel;
  const householdAmount = component.baseAmount
    ? formatAmount(component.baseAmount.amount, component.baseAmount.currency)
    : componentHouseholdAmount(component, householdCurrency)
      ? formatAmount(componentHouseholdAmount(component, householdCurrency) as string, householdCurrency)
      : undefined;
  const showHousehold = Boolean(householdAmount && component.nativeCurrency !== householdCurrency);
  return (
    <span className="flex flex-col items-end">
      <span className="num font-semibold">{native}</span>
      {showHousehold && <span className="num text-xs font-normal text-muted-foreground">{householdAmount}</span>}
    </span>
  );
}

/**
 * AccountDetail is the in-workspace full-width Account view. Composite
 * investment accounts share one Cash + Investments layout; multi-currency
 * cash-on-hand accounts show only Cash. Simple accounts show one current
 * value. Amounts come from backend valuation components.
 */
export function AccountDetail({
  record,
  valuation,
  valuationUpdatedAt,
  onBack, updateValue = false,
}: {
  record: AccountRecordDTO;
  valuation?: AccountValuationDTO;
  valuationUpdatedAt?: number;
  onBack: () => void;
  updateValue?: boolean;
}) {
  const { t, i18n } = useTranslation();
  const origin = useHistoryOrigin();
  const settings = useSettings();
  const updateAccount = useUpdateAccount();
  const archiveAccount = useArchiveAccount();
  const [action, setActionState] = useState<AccountAction>(updateValue && !record.account.archivedAt ? record.account.trackingMode === "holdings" ? "cash" : "simple" : null);
  const cashActionTrigger = useRef<HTMLButtonElement>(null);
  const backTrigger = useRef<HTMLButtonElement>(null);
  const [actionHoldingId, setActionHoldingIdState] = useState<string>();
  const setAction = (next: AccountAction) => { if (!hasUnresolvedSheetConfirmation()) setActionState(next); };
  const setActionHoldingId = (next: string | undefined) => { if (!hasUnresolvedSheetConfirmation()) setActionHoldingIdState(next); };
  const [settingsCanSubmit, setSettingsCanSubmit] = useState(false);
  const [settingsError, setSettingsError] = useState<string | undefined>();
  const archived = Boolean(record.account.archivedAt);
  const composite = record.account.trackingMode === "holdings";
  const cashValues = useAccountCashValues(record.account.id, composite);
  const cashOnly = composite && record.account.accountType === "cash_on_hand";
  const historyStarted = Boolean(origin.data);
  const readOnly = archived;
  const cash = useMemo(() => cashComponents(valuation), [valuation]);
  const groupsQuery = useInstrumentHoldingGroups(composite && !cashOnly, record.account.id);
  // Contract-valued (cash-like) holdings live in the products section below.
  const marketInstruments = (groupsQuery.data ?? []).filter((group) => group.key !== "cash_like").flatMap((group) => group.instruments ?? []);
  const investmentRows = marketInstruments.filter((instrument) => (instrument.holdings ?? []).some((member) => compareCanonical(member.amounts.quantity, "0") !== 0));
  const dividendHoldingsAvailable = marketInstruments.some((instrument) => !instrument.archived && (instrument.holdings ?? []).length > 0);
  const householdCurrency = valuation?.baseValue?.currency ?? "";


  const displayMoney = accountDisplayMoney(record, valuation);
  const titleAmount = displayMoney.primary
    ? `${formatAmount(displayMoney.primary.amount, displayMoney.primary.currency)}${displayMoney.primaryPartial ? ` · ${t("accounts.displaySubtotal")}` : ""}`
    : displayMoney.unavailableCurrency
      ? t("accounts.currencyUnavailable", { currency: displayMoney.unavailableCurrency })
      : t("accounts.noValue");
  const titleSecondary = displayMoney.secondary
    ? `${formatAmount(displayMoney.secondary.amount, displayMoney.secondary.currency)}${displayMoney.secondaryPartial ? ` · ${t("accounts.displaySubtotal")}` : ""}`
    : displayMoney.primary && displayMoney.pendingConversion
      ? t("accounts.pendingConversion")
      : undefined;
  const completeness = valuation
    ? valuation.complete && (valuation.displayValue?.complete ?? true)
      ? t("accounts.completeValuation")
      : t("accounts.partialValuation")
    : t("accounts.noValue");
  const asOf = valuationUpdatedAt && valuationUpdatedAt > 0 ? t("accounts.asOf", { time: formatTimestamp(valuationUpdatedAt, settings.data?.timezone, i18n.language) }) : undefined;
  const institutionLabel = record.account.institutionId ? record.institutionName || t("accounts.unassignedInstitution") : t("accounts.unassignedInstitution");

  const settingsSavePending = useRef(false);
  const saveSettings = async (request: CreateAccountRequest, extras: AccountFormExtras) => {
    if (settingsSavePending.current) return;
    settingsSavePending.current = true;
    setSettingsError(undefined);
    try {
      await updateAccount.mutateAsync({ id: record.account.id, request: toUpdateAccountRequest(request) });
      void extras;
      toast.success(t("common.saved"));
      setAction(null);
    } catch (error) {
      setSettingsError(displayError(error, t("accounts.saveError")));
    } finally {
      settingsSavePending.current = false;
    }
  };

  const missingReasons = (valuation?.missingInputs ?? []).map((item) =>
    item.kind === "fx_rate" ? t("accounts.missingFx") : item.kind === "instrument_price" ? t("accounts.missingPrice") : displayEnum(t, "overview.missingKind", item.kind),
  );

  return (
    <div className="flex min-w-0 flex-col gap-6" data-testid="account-detail">
      <PageChrome
        pageId="accounts"
        title={<span className="flex min-w-0 items-center gap-2"><AccountIcon account={record.account} className="size-8 shrink-0 text-primary" />{record.account.name}</span>}
        actions={
          <>
            <AnalyzeMenu inline accountId={record.account.id} label={record.account.name} />
            <Button ref={backTrigger} type="button" variant="outline" size="sm" onClick={() => { if (!hasUnresolvedSheetConfirmation()) onBack(); }}>
              <ArrowLeft className="size-4" aria-hidden="true" /> {t("accounts.backToList")}
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={() => { setSettingsError(undefined); setAction("settings"); }}>
              {t("accounts.settings")}
            </Button>
          </>
        }
      />
      <PageIntro
        description={`${institutionLabel} · ${displayEnum(t, "enum", record.account.accountType)}`}
        status={
          <>
            <div className="flex flex-col items-end gap-1">
              <MoneyText size="lg" className="tracking-tight">{titleAmount}</MoneyText>
              {titleSecondary && <p className="text-sm text-muted-foreground">{titleSecondary}</p>}
            </div>
            <Badge variant={valuation?.complete && (valuation.displayValue?.complete ?? true) ? "success" : "warning"}>{completeness}</Badge>
            {archived && <Badge variant="secondary">{t("common.archived")}</Badge>}
            {record.account.balanceSheetRole === "liability" && <Badge variant="outline">{t("accounts.liability")}</Badge>}
            {asOf && <p className="text-sm text-muted-foreground">{asOf}</p>}
          </>
        }
      />

      <p className="text-sm text-muted-foreground">
        {record.account.includeInNetWorth ? t("accounts.includedInNetWorth") : t("accounts.excludedFromNetWorth")}
      </p>
      {archived && (
        <p role="status" className="rounded-xl border border-border bg-muted/60 px-4 py-2.5 text-sm">
          {t("accounts.archivedReadOnly")}
        </p>
      )}
      <AccountReconciliation accountId={record.account.id} />
      {valuation && !valuation.complete && missingReasons.length > 0 && (
        <p role="status" className="text-sm text-warning-foreground">
          {t("overview.totalsExcludeMissing")} {missingReasons.join(" · ")}
        </p>
      )}

      {composite && !cashOnly && groupsQuery.isLoading && <LoadingState label={t("ui.state.loadingPage")} />}
      {composite && !cashOnly && groupsQuery.isError && (
        <ErrorState title={t("accounts.loadError")} description={t("ui.state.errorDescription")} onRetry={() => { void groupsQuery.refetch(); }} retryLabel={t("common.retryAction")} />
      )}

      {composite && record.account.balanceSheetRole !== "liability" && valuation?.composition && (
        <AccountCompositionPanel key={record.account.id} composition={valuation.composition} updatedAt={valuationUpdatedAt} />
      )}

      {composite && (
        <div data-testid="account-cash-holdings-layout" className="grid min-w-0 grid-cols-1 items-start gap-4">
          <Card className="min-w-0">
            <CardHeader>
              <CardTitle>{t("accounts.cash")}</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              {cash.length === 0 ? (
                <EmptyState title={t("accounts.noCash")} description={t("accounts.addCashBalance")} />
              ) : (
                <ul className="flex flex-col gap-2 text-sm">
                  {cash.map((component) => (
                    <li key={component.nativeCurrency} className="flex items-center justify-between">
                      <span>{component.nativeCurrency}</span>
                      <span className="flex items-center gap-2">
                        <ComponentAmounts
                          component={component}
                          householdCurrency={householdCurrency}
                          pendingLabel={t("accounts.partialValuation")}
                        />
                        {!component.available && <Badge variant="warning">{t("accounts.missingFx")}</Badge>}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
              {!readOnly && (
                <div className="flex flex-wrap gap-2">
                  <Button ref={cashActionTrigger} type="button" onClick={() => setAction("cash")}>
                    {historyStarted ? t("accounts.reconcileBalance") : t("accounts.addCashBalance")}
                  </Button>
                  <Button type="button" variant="outline" onClick={() => setAction("deposit")}>
                    {t("accounts.depositOrWithdraw")}
                  </Button>
                  <ActionMenu
                    label={t("review.moreActions")}
                    items={[
                      { id: "fx", label: t("accounts.convert"), onSelect: () => setAction("fx") },
                      { id: "transfer", label: t("accounts.transfer"), onSelect: () => setAction("transfer") },
                    ]}
                  />
                </div>
              )}
            </CardContent>
          </Card>
          {!cashOnly && !groupsQuery.isLoading && !groupsQuery.isError && <Card className="min-w-0">
            <CardHeader>
              <CardTitle>{t("accounts.investments")}</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              {investmentRows.length === 0 ? (
                <EmptyState title={t("accounts.noHoldings")} description={t("accounts.buyInvestment")} />
              ) : (
                <HoldingGroupsList scope="account" groups={groupsQuery.data ?? []} accountId={record.account.id} accountName={record.account.name}
                  onDividend={readOnly ? undefined : (holdingId) => { setActionHoldingId(holdingId); setAction("dividend"); }} />
              )}
              {!readOnly && (
                <div className="flex flex-wrap gap-2">
                  <Button type="button" onClick={() => setAction("buy")}>
                    {t("accounts.buyInvestment")}
                  </Button>
                  {investmentRows.length > 0 && (
                    <Button type="button" variant="outline" onClick={() => setAction("sell")}>
                      {t("accounts.sellInvestment")}
                    </Button>
                  )}
                  {dividendHoldingsAvailable && (
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => {
                        setActionHoldingId(undefined);
                        setAction("dividend");
                      }}
                    >
                      {t("accounts.cashDividend")}
                    </Button>
                  )}
                  <Button type="button" variant="ghost" onClick={() => setAction("position")}>
                    {t("accounts.recordPosition")}
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>}
        </div>
      )}

      <AccountProductsSection
        record={record}
        cashAmount={cash.find((component) => component.nativeCurrency === record.account.defaultCurrency)?.nativeAmount}
      />

      {composite && !cashValues.isError && (cashValues.data?.length ?? 0) > 0 && (
        <Card data-testid="account-cash-history">
          <CardHeader><CardTitle>{t("accounts.cashHistory")}</CardTitle></CardHeader>
          <CardContent>
            <div className="overflow-x-auto">
              <div className="min-w-0 overflow-x-auto"><table className="w-full text-left text-sm">
                <caption className="sr-only">{t("accounts.cashHistory")}</caption>
                <thead>
                  <tr className="border-b border-border text-muted-foreground">
                    <th className="py-2 pr-4 font-medium">{t("accounts.cashHistoryType")}</th>
                    <th className="py-2 pr-4 font-medium">{t("accounts.cashHistoryChange")}</th>
                    <th className="py-2 pr-4 font-medium">{t("accounts.cashHistoryBalance")}</th>
                    <th className="hidden py-2 pr-4 font-medium md:table-cell">{t("accounts.cashHistoryCurrency")}</th>
                    <th className="py-2 pr-4 font-medium">{t("accounts.cashHistoryEffectiveAt")}</th>
                    <th className="hidden py-2 font-medium lg:table-cell">{t("accounts.cashHistoryRecordedAt")}</th>
                  </tr>
                </thead>
                <tbody>
                  {(cashValues.data ?? []).map((value) => (
                    <tr key={value.id} className="border-b border-border last:border-0">
                      <td className="max-w-64 py-2 pr-4">
                        <div className="flex flex-col">
                          <span>{cashActivityLabel(t, value)}</span>
                          {value.activityReason && <span className="text-xs text-muted-foreground">{displayEnum(t, "history.reason", value.activityReason)}</span>}
                          {value.activityNote && <span className="truncate text-xs text-muted-foreground" title={value.activityNote}>{value.activityNote}</span>}
                        </div>
                      </td>
                      <td className="num py-2 pr-4 font-medium">{cashChangeLabel(value)}</td>
                      <td className="num py-2 pr-4 font-medium">{formatAmount(value.amount.amount, value.amount.currency)}</td>
                      <td className="hidden py-2 pr-4 md:table-cell">{value.amount.currency}</td>
                      <td className="py-2 pr-4">
                        <time dateTime={value.effectiveAt}>{formatTimestamp(value.effectiveAt, settings.data?.timezone, i18n.language)}</time>
                      </td>
                      <td className="hidden py-2 text-muted-foreground lg:table-cell">
                        <time dateTime={value.createdAt}>{formatTimestamp(value.createdAt, settings.data?.timezone, i18n.language)}</time>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table></div>
            </div>
          </CardContent>
        </Card>
      )}

      {!composite && (
        <Card className="min-w-0">
          <CardHeader>
            <CardTitle>{displayEnum(t, "enum", record.account.accountType)}</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <p className="text-sm text-muted-foreground">
              {record.account.trackingMode === "manual_value"
                ? t("accounts.lastValuation", { amount: titleAmount })
                : t("accounts.currentBalance", { amount: titleAmount })}
            </p>
            {!readOnly && (
              <Button type="button" className="self-start" onClick={() => setAction("simple")}>
                {record.account.trackingMode === "manual_value" ? t("accounts.updateValue") : t("accounts.updateBalance")}
              </Button>
            )}
          </CardContent>
        </Card>
      )}

      {action && action !== "settings" && (
        <AccountActionSheet
          action={action}
          holdingId={actionHoldingId}
          record={record}
          valuation={valuation}
          historyStarted={historyStarted}
          onClose={() => {
            setAction(null);
            setActionHoldingId(undefined);
          }}
          finalFocus={action === "cash" ? () => cashActionTrigger.current?.isConnected ? cashActionTrigger.current : backTrigger.current?.isConnected ? backTrigger.current : document.getElementById("main-content") : undefined}
          onRecorded={() => {
            setAction(null);
            setActionHoldingId(undefined);
          }}
        />
      )}

      <Sheet
        open={action === "settings"}
        onOpenChange={(open) => {
          if (settingsSavePending.current) return;
          if (!open) setAction(null);
          if (open) setSettingsError(undefined);
        }}
      >
        <SheetContent size="lg" closeDisabled={updateAccount.isPending}>
          <SheetHeader className="shrink-0">
            <SheetTitle>{t("accounts.settings")}</SheetTitle>
          </SheetHeader>
          <div>
            {archived ? (
              <p className="text-sm text-muted-foreground">{t("accounts.archivedReadOnly")}</p>
            ) : (
              <AccountForm
                key={record.account.id}
                formId="account-settings-form"
                hideSubmit
                onCanSubmitChange={setSettingsCanSubmit}
                record={record}
                submitLabel={t("common.save")}
                isSubmitting={updateAccount.isPending}
                submissionError={settingsError}
                onSubmit={saveSettings}
              />
            )}
            <div className="mt-6 border-t border-border pt-4">
              <AlertDialog>
                <AlertDialogTrigger disabled={updateAccount.isPending} className={cn(buttonVariants({ variant: "outline", size: "sm" }))}>
                  {archived ? t("accounts.restore") : t("accounts.archive")}
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>{archived ? t("accounts.restore") : t("accounts.archive")}</AlertDialogTitle>
                    <AlertDialogDescription>{record.account.name}</AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
                    <AlertDialogAction
                      disabled={updateAccount.isPending}
                      onClick={() =>
                        archiveAccount.mutate(
                          { id: record.account.id, archived: !archived },
                          {
                            onSuccess: () => {
                              toast.success(archived ? t("accounts.restore") : t("accounts.archive"));
                              setAction(null);
                            },
                            onError: (error) => toast.error(displayError(error, t("accounts.saveError"))),
                          },
                        )
                      }
                    >
                      {archived ? t("accounts.restore") : t("accounts.archive")}
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </div>
          </div>
          <SheetFooter className="shrink-0 border-t border-border">
            <Button type="button" variant="outline" disabled={updateAccount.isPending} onClick={() => setAction(null)}>{t("common.cancel")}</Button>
            {!archived && <Button type="submit" form="account-settings-form" disabled={updateAccount.isPending || !settingsCanSubmit}>{updateAccount.isPending ? t("common.pending") : t("common.save")}</Button>}
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </div>
  );
}
