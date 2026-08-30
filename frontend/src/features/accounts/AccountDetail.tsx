import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowLeft } from "lucide-react";
import { Button, buttonVariants } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { PageHeader } from "@/components/layout/PageHeader";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { AccountForm, type AccountFormExtras } from "@/features/accounts/AccountForm";
import { AccountActionSheet, type AccountAction } from "@/features/accounts/AccountActionSheets";
import { useArchiveAccount, useUpdateAccount, toUpdateAccountRequest } from "@/queries/accounts";
import { useHoldingsByAccounts, useInstruments } from "@/queries/investments";
import { useHistoryOrigin } from "@/queries/history";
import { formatAmount } from "@/lib/money";
import { nativeMoney } from "@/features/accounts/nativeMoney";
import { formatTimestamp } from "@/lib/time";
import { useSettings } from "@/queries/settings";
import { displayEnum, displayError } from "@/lib/display";
import { cn } from "@/lib/utils";
import { toast } from "sonner";
import type { AccountRecordDTO, AccountValuationDTO, HoldingDTO, ValuationComponentDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import type { CreateAccountRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account/models";
import { EntityIcon } from "@/components/icons/EntityIcon";
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
  return (valuation?.components ?? []).filter((component) => !component.instrumentId);
}

function holdingRows(
  holdings: HoldingDTO[],
  valuation: AccountValuationDTO | undefined,
  instruments: { id: string; name: string; type: string }[],
): {
  holding: HoldingDTO;
  instrumentName: string;
  instrumentType: string;
  instrumentActive: boolean;
  component?: ValuationComponentDTO;
}[] {
  const byHoldingId = new Map((valuation?.components ?? []).filter((component) => component.holdingId).map((component) => [component.holdingId as string, component]));
  const instrumentById = new Map(instruments.map((instrument) => [instrument.id, instrument]));
  return holdings
    .filter((holding) => !holding.archivedAt)
    .map((holding) => {
      const instrument = instrumentById.get(holding.instrumentId);
      const component = byHoldingId.get(holding.id);
      return {
        holding,
        instrumentName: component?.instrumentName || instrument?.name || holding.instrumentId,
        instrumentType: instrument?.type ?? "",
        instrumentActive: Boolean(instrument),
        component,
      };
    });
}

/**
 * AccountDetail is the in-workspace full-width Account view. Composite
 * accounts share one Cash + Investments layout; Simple accounts show a
 * single current value. Amounts come from backend valuation components.
 */
export function AccountDetail({
  record,
  valuation,
  valuationUpdatedAt,
  onBack,
}: {
  record: AccountRecordDTO;
  valuation?: AccountValuationDTO;
  valuationUpdatedAt?: number;
  onBack: () => void;
}) {
  const { t, i18n } = useTranslation();
  const origin = useHistoryOrigin();
  const settings = useSettings();
  const holdingsQuery = useHoldingsByAccounts([record.account.id]);
  const instruments = useInstruments();
  const updateAccount = useUpdateAccount();
  const archiveAccount = useArchiveAccount();
  const [action, setAction] = useState<AccountAction>(null);
  const [actionHoldingId, setActionHoldingId] = useState<string>();
  const [settingsError, setSettingsError] = useState<string | undefined>();
  const archived = Boolean(record.account.archivedAt);
  const composite = record.account.trackingMode === "holdings";
  const historyStarted = Boolean(origin.data);
  const readOnly = archived;
  const cash = useMemo(() => cashComponents(valuation), [valuation]);
  const rows = useMemo(
    () => holdingRows(holdingsQuery.data?.[record.account.id] ?? [], valuation, instruments.data ?? []),
    [holdingsQuery.data, record.account.id, valuation, instruments.data],
  );
  const dividendHoldingsAvailable = rows.some((row) => row.instrumentActive);

  const native = nativeMoney(record, valuation);
  const titleAmount = native
    ? formatAmount(native.amount, native.currency)
    : valuation?.baseValue
      ? formatAmount(valuation.baseValue.amount, valuation.baseValue.currency)
      : t("accounts.noValue");
  const titleSecondary = native && valuation?.baseValue
    ? formatAmount(valuation.baseValue.amount, valuation.baseValue.currency)
    : native && !valuation?.baseValue
      ? t("accounts.pendingConversion")
      : undefined;
  const completeness = valuation
    ? valuation.complete
      ? t("accounts.completeValuation")
      : t("accounts.partialValuation")
    : t("accounts.noValue");
  const asOf = valuationUpdatedAt && valuationUpdatedAt > 0 ? t("accounts.asOf", { time: formatTimestamp(valuationUpdatedAt, settings.data?.timezone, i18n.language) }) : undefined;
  const institutionLabel = record.account.institutionId ? record.institutionName || t("accounts.unassignedInstitution") : t("accounts.unassignedInstitution");

  const saveSettings = async (request: CreateAccountRequest, extras: AccountFormExtras) => {
    setSettingsError(undefined);
    try {
      await updateAccount.mutateAsync({ id: record.account.id, request: toUpdateAccountRequest(request) });
      void extras;
      toast.success(t("common.saved"));
      setAction(null);
    } catch (error) {
      setSettingsError(displayError(error, t("accounts.saveError")));
    }
  };

  const missingReasons = (valuation?.missingInputs ?? []).map((item) =>
    item.kind === "fx_rate" ? t("accounts.missingFx") : item.kind === "instrument_price" ? t("accounts.missingPrice") : displayEnum(t, "overview.missingKind", item.kind),
  );

  return (
    <div className="flex flex-col gap-6" data-testid="account-detail">
      <PageHeader
        title={<span className="flex items-center gap-2"><EntityIcon iconKey={record.account.iconKey} kind="account" className="size-6 text-primary" />{record.account.name}</span>}
        description={`${institutionLabel} · ${displayEnum(t, "enum", record.account.accountType)}`}
        status={
          <div className="flex flex-wrap items-center gap-2">
            <p className="text-2xl font-semibold tracking-tight">{titleAmount}</p>
            {titleSecondary && <p className="text-sm text-muted-foreground">{titleSecondary}</p>}
            <Badge variant={valuation?.complete ? "success" : "warning"}>{completeness}</Badge>
            {archived && <Badge variant="secondary">{t("common.archived")}</Badge>}
            {record.account.balanceSheetRole === "liability" && <Badge variant="outline">{t("accounts.liability")}</Badge>}
            {asOf && <p className="text-sm text-muted-foreground">{asOf}</p>}
          </div>
        }
        actions={
          <>
            <Button type="button" variant="outline" onClick={onBack}>
              <ArrowLeft className="size-4" aria-hidden="true" /> {t("accounts.backToList")}
            </Button>
            <Button type="button" variant="outline" onClick={() => setAction("settings")}>
              {t("accounts.settings")}
            </Button>
          </>
        }
      />

      <p className="text-sm text-muted-foreground">
        {record.account.includeInNetWorth ? t("accounts.includedInNetWorth") : t("accounts.excludedFromNetWorth")}
      </p>
      {archived && (
        <p role="status" className="rounded-md border border-border bg-muted/40 px-3 py-2 text-sm">
          {t("accounts.archivedReadOnly")}
        </p>
      )}
      {valuation && !valuation.complete && missingReasons.length > 0 && (
        <p role="status" className="text-sm text-warning-foreground">
          {t("overview.totalsExcludeMissing")} {missingReasons.join(" · ")}
        </p>
      )}

      {composite && (holdingsQuery.isLoading || instruments.isLoading) && <LoadingState label={t("ui.state.loadingPage")} />}
      {composite && holdingsQuery.isError && (
        <ErrorState title={t("accounts.loadError")} description={t("ui.state.errorDescription")} onRetry={() => holdingsQuery.refetch()} retryLabel={t("common.retryAction")} />
      )}

      {composite && !holdingsQuery.isLoading && !holdingsQuery.isError && (
        <div className="grid gap-4 lg:grid-cols-2">
          <Card>
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
                        {component.nativeAmount
                          ? formatAmount(component.nativeAmount, component.nativeCurrency)
                          : t("accounts.partialValuation")}
                        {!component.available && <Badge variant="warning">{t("accounts.missingFx")}</Badge>}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
              {!readOnly && (
                <div className="flex flex-wrap gap-2">
                  <Button type="button" onClick={() => setAction("cash")}>
                    {historyStarted ? t("accounts.reconcileBalance") : t("accounts.addCashBalance")}
                  </Button>
                  <Button type="button" variant="outline" onClick={() => setAction("deposit")}>
                    {t("accounts.depositOrWithdraw")}
                  </Button>
                  <Button type="button" variant="outline" onClick={() => setAction("fx")}>
                    {t("accounts.convert")}
                  </Button>
                  <Button type="button" variant="outline" onClick={() => setAction("transfer")}>
                    {t("accounts.transfer")}
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t("accounts.investments")}</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              {rows.length === 0 ? (
                <EmptyState title={t("accounts.noHoldings")} description={t("accounts.buyInvestment")} />
              ) : (
                <table className="w-full text-left text-sm">
                  <caption className="sr-only">{t("accounts.investments")}</caption>
                  <thead>
                    <tr className="border-b border-border text-muted-foreground">
                      <th className="py-2 font-medium">{t("history.instrument")}</th>
                      <th className="py-2 font-medium">{t("accounts.instrumentType")}</th>
                      <th className="py-2 font-medium">{t("accounts.quantity")}</th>
                      <th className="py-2 font-medium">{t("accounts.marketValue")}</th>
                      {!readOnly && <th className="py-2 font-medium"><span className="sr-only">{t("accounts.cashDividend")}</span></th>}
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((row) => (
                      <tr key={row.holding.id} className="border-b border-border last:border-0">
                        <td className="py-2">{row.instrumentName}</td>
                        <td className="py-2">{row.instrumentType ? displayEnum(t, "enum", row.instrumentType) : t("accounts.noValue")}</td>
                        <td className="py-2">{formatAmount(row.holding.quantity)}</td>
                        <td className="py-2">
                          {row.component?.available && row.component.nativeAmount
                            ? formatAmount(row.component.nativeAmount, row.component.nativeCurrency)
                            : t("accounts.missingPrice")}
                        </td>
                        {!readOnly && (
                          <td className="py-2 text-right">
                            {row.instrumentActive ? (
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                onClick={() => {
                                  setActionHoldingId(row.holding.id);
                                  setAction("dividend");
                                }}
                              >
                                {t("accounts.cashDividend")}
                              </Button>
                            ) : null}
                          </td>
                        )}
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
              {!readOnly && (
                <div className="flex flex-wrap gap-2">
                  <Button type="button" onClick={() => setAction("buy")}>
                    {t("accounts.buyInvestment")}
                  </Button>
                  {rows.length > 0 && (
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
          </Card>
        </div>
      )}

      {!composite && (
        <Card>
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
          onRecorded={() => {
            setAction(null);
            setActionHoldingId(undefined);
          }}
        />
      )}

      <Sheet
        open={action === "settings"}
        onOpenChange={(open) => {
          if (!open) setAction(null);
          if (open) setSettingsError(undefined);
        }}
      >
        <SheetContent>
          <SheetHeader>
            <SheetTitle>{t("accounts.settings")}</SheetTitle>
          </SheetHeader>
          <div className="overflow-y-auto">
            {archived ? (
              <p className="text-sm text-muted-foreground">{t("accounts.archivedReadOnly")}</p>
            ) : (
              <AccountForm
                key={record.account.id}
                record={record}
                submitLabel={t("common.save")}
                isSubmitting={updateAccount.isPending}
                submissionError={settingsError}
                onSubmit={saveSettings}
              />
            )}
            <div className="mt-6 border-t border-border pt-4">
              <AlertDialog>
                <AlertDialogTrigger className={cn(buttonVariants({ variant: "outline", size: "sm" }))}>
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
        </SheetContent>
      </Sheet>
    </div>
  );
}
