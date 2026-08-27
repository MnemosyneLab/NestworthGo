import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Plus } from "lucide-react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/layout/PageHeader";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { useAccounts } from "@/queries/accounts";
import {
  useInstruments,
  useCreateInstrument,
  useArchiveInstrument,
  useCreateHolding,
  useAllHoldingsFlat,
  useCurrentInstrumentQuote,
  useSaveManualInstrumentQuote,
} from "@/queries/investments";
import { useHoldingGainsByAccounts } from "@/queries/analytics";
import { InstrumentForm } from "@/features/investments/InstrumentForm";
import { displayEnum, displayError } from "@/lib/display";
import { formatAmount } from "@/lib/money";
import { cn } from "@/lib/utils";
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

function ManualQuoteForm({
  instrumentId,
  currency,
  onSaved,
}: {
  instrumentId: string;
  currency: string;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const saveQuote = useSaveManualInstrumentQuote();
  const [unitPrice, setUnitPrice] = useState("");
  const [quotedAt, setQuotedAt] = useState("");

  const submit = () => {
    if (!unitPrice.trim()) {
      return;
    }
    saveQuote.mutate(
      { instrumentId, unitPrice: unitPrice.trim(), quotedAt: quotedAt ? new Date(`${quotedAt}T00:00:00`).toISOString() : "" },
      {
        onSuccess: () => {
          toast.success(t("portfolio.priceSaved"));
          onSaved();
        },
      },
    );
  };

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`quote-price-${instrumentId}`}>{t("portfolio.unitPrice")}</Label>
        <Input
          id={`quote-price-${instrumentId}`}
          value={unitPrice}
          onChange={(event) => setUnitPrice(event.target.value)}
          inputMode="decimal"
          placeholder={currency}
        />
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`quote-date-${instrumentId}`}>{t("portfolio.quotedAt")}</Label>
        <Input id={`quote-date-${instrumentId}`} type="date" value={quotedAt} onChange={(event) => setQuotedAt(event.target.value)} />
      </div>
      {saveQuote.isError && (
        <p role="alert" className="text-sm text-destructive">
          {displayError(saveQuote.error, t("portfolio.updateError"))}
        </p>
      )}
      <Button onClick={submit} disabled={saveQuote.isPending || !unitPrice.trim()}>
        {saveQuote.isPending ? t("common.pending") : t("portfolio.setPrice")}
      </Button>
    </div>
  );
}

function InstrumentRow({
  instrument,
  onSetPrice,
  onArchive,
}: {
  instrument: {
    id: string;
    name: string;
    quoteCurrency: string;
    quoteSource: string;
    archivedAt?: string | null;
  };
  onSetPrice: () => void;
  onArchive: () => void;
}) {
  const { t, i18n } = useTranslation();
  const quote = useCurrentInstrumentQuote(instrument.id);
  const quoteTime = quote.data?.quotedAt ?? quote.data?.createdAt;

  return (
    <li className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-border px-3 py-3 text-sm">
      <span className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
        <span className="font-medium">{instrument.name}</span>
        <Badge variant="secondary">{instrument.quoteCurrency}</Badge>
        <Badge variant={instrument.quoteSource === "manual" ? "outline" : "success"}>
          {displayEnum(t, "portfolio", instrument.quoteSource)}
        </Badge>
        {instrument.archivedAt && <Badge variant="secondary">{t("common.archived")}</Badge>}
      </span>
      <span className="flex min-w-[15rem] flex-1 flex-wrap items-center justify-end gap-x-3 gap-y-1 text-right">
        {quote.isLoading ? (
          <span className="text-muted-foreground">{t("portfolio.priceLoading")}</span>
        ) : quote.data ? (
          <span>
            <span className="font-medium">{t("portfolio.latestPrice", { value: formatAmount(quote.data.unitPrice, quote.data.currency) })}</span>
            {quoteTime && (
              <span className="ml-2 text-xs text-muted-foreground">
                {t("portfolio.quotedAsOf", {
                  time: new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium", timeStyle: "short" }).format(new Date(quoteTime)),
                })}
              </span>
            )}
          </span>
        ) : (
          <span className="text-muted-foreground">{t("portfolio.noCurrentPrice")}</span>
        )}
      </span>
      <span className="flex shrink-0 items-center gap-2">
        {instrument.quoteSource === "manual" && !instrument.archivedAt && (
          <Button type="button" variant="outline" size="sm" onClick={onSetPrice}>
            {t("portfolio.setPrice")}
          </Button>
        )}
        <AlertDialog>
          <AlertDialogTrigger className={cn(buttonVariants({ variant: "outline", size: "sm" }))}>
            {instrument.archivedAt ? t("common.active") : t("common.archive")}
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{instrument.archivedAt ? t("common.active") : t("common.archive")}</AlertDialogTitle>
              <AlertDialogDescription>{instrument.name}</AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
              <AlertDialogAction onClick={onArchive}>{instrument.archivedAt ? t("common.active") : t("common.archive")}</AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </span>
    </li>
  );
}

function InstrumentsTab() {
  const { t } = useTranslation();
  const instruments = useInstruments();
  const createInstrument = useCreateInstrument();
  const archiveInstrument = useArchiveInstrument();
  const [open, setOpen] = useState(false);
  const [quoteTarget, setQuoteTarget] = useState<{ id: string; name: string; currency: string } | null>(null);

  if (instruments.isLoading) {
    return <LoadingState label={t("portfolio.loading")} />;
  }

  if (instruments.isError) {
    return (
      <ErrorState
        title={t("portfolio.loadError")}
        description={t("ui.state.errorDescription")}
        onRetry={() => instruments.refetch()}
        retryLabel={t("common.retryAction")}
      />
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetTrigger className={buttonVariants({})}>
          <Plus className="size-4" aria-hidden="true" /> {t("portfolio.addInstrument")}
        </SheetTrigger>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>{t("portfolio.addInstrument")}</SheetTitle>
          </SheetHeader>
          <InstrumentForm
            isSubmitting={createInstrument.isPending}
            submissionError={createInstrument.isError ? displayError(createInstrument.error, t("portfolio.createError")) : undefined}
            onSubmit={(request) =>
              createInstrument.mutate(request, {
                onSuccess: () => {
                  toast.success(t("portfolio.instrumentCreated"));
                  setOpen(false);
                },
              })
            }
          />
        </SheetContent>
      </Sheet>

      {(instruments.data ?? []).length === 0 ? (
        <EmptyState title={t("portfolio.noInstruments")} description={t("portfolio.noInstrumentsDescription")} />
      ) : (
        <ul className="flex flex-col gap-2">
          {(instruments.data ?? []).map((instrument) => (
            <InstrumentRow
              key={instrument.id}
              instrument={instrument}
              onSetPrice={() => setQuoteTarget({ id: instrument.id, name: instrument.name, currency: instrument.quoteCurrency })}
              onArchive={() =>
                archiveInstrument.mutate(
                  { id: instrument.id, archived: !instrument.archivedAt },
                  {
                    onSuccess: () => toast.success(instrument.archivedAt ? t("common.active") : t("common.archived")),
                    onError: (error) => toast.error(displayError(error, t("portfolio.updateError"))),
                  },
                )
              }
            />
          ))}
        </ul>
      )}
      <Sheet open={Boolean(quoteTarget)} onOpenChange={(open) => !open && setQuoteTarget(null)}>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>{quoteTarget ? `${t("portfolio.setPrice")} · ${quoteTarget.name}` : t("portfolio.setPrice")}</SheetTitle>
          </SheetHeader>
          {quoteTarget && (
            <ManualQuoteForm
              key={quoteTarget.id}
              instrumentId={quoteTarget.id}
              currency={quoteTarget.currency}
              onSaved={() => setQuoteTarget(null)}
            />
          )}
        </SheetContent>
      </Sheet>
    </div>
  );
}

function HoldingsTab() {
  const { t } = useTranslation();
  const accounts = useAccounts({});
  const instruments = useInstruments();
  const createHolding = useCreateHolding();
  const [open, setOpen] = useState(false);
  const [accountId, setAccountId] = useState("");
  const [instrumentId, setInstrumentId] = useState("");
  const [quantity, setQuantity] = useState("");
  const [formError, setFormError] = useState<string | undefined>();

  const holdingsAccounts = (accounts.data ?? []).filter((record) => record.account.trackingMode === "holdings" && !record.account.archivedAt);
  const accountIds = holdingsAccounts.map((record) => record.account.id);
  const holdings = useAllHoldingsFlat(accountIds);
  const holdingGains = useHoldingGainsByAccounts(accountIds);

  if (accounts.isLoading || instruments.isLoading || holdings.isLoading || holdingGains.isLoading) {
    return <LoadingState label={t("portfolio.loading")} />;
  }

  if (accounts.isError || instruments.isError || holdings.isError || holdingGains.isError) {
    return (
      <ErrorState
        title={t("portfolio.loadError")}
        description={t("ui.state.errorDescription")}
        onRetry={() => {
          void accounts.refetch();
          void instruments.refetch();
          void holdings.refetch();
          void holdingGains.refetch();
        }}
        retryLabel={t("common.retryAction")}
      />
    );
  }

  const accountNameById = new Map(holdingsAccounts.map((record) => [record.account.id, record.account.name]));
  const allHoldings = holdings.data;

  if (holdingsAccounts.length === 0) {
    return <EmptyState title={t("portfolio.noHoldingsAccount")} description={t("portfolio.noHoldingsDescription")} />;
  }

  const submit = () => {
    if (!accountId || !instrumentId || !quantity.trim()) {
      setFormError(t("portfolio.holdingRequired"));
      return;
    }
    setFormError(undefined);
    createHolding.mutate(
      { accountId, instrumentId, quantity: quantity.trim() },
      {
        onSuccess: () => {
          toast.success(t("portfolio.holdingCreated"));
          setOpen(false);
          setAccountId("");
          setInstrumentId("");
          setQuantity("");
        },
      },
    );
  };

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-muted-foreground">{t("portfolio.holdingsIndexDescription")}</p>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetTrigger className={buttonVariants({})}>
          <Plus className="size-4" aria-hidden="true" /> {t("portfolio.addHolding")}
        </SheetTrigger>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>{t("portfolio.addHolding")}</SheetTitle>
          </SheetHeader>
          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="holding-account">{t("history.accountSelect")}</Label>
              <NativeSelect id="holding-account" value={accountId} onChange={(event) => setAccountId(event.target.value)}>
                <option value="">{t("history.accountSelectEmpty")}</option>
                {holdingsAccounts.map((record) => (
                  <option key={record.account.id} value={record.account.id}>
                    {record.account.name}
                  </option>
                ))}
              </NativeSelect>
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="holding-instrument">{t("history.instrument")}</Label>
              <NativeSelect id="holding-instrument" value={instrumentId} onChange={(event) => setInstrumentId(event.target.value)}>
                <option value="">{t("history.selectInstrument")}</option>
                {(instruments.data ?? []).map((instrument) => (
                  <option key={instrument.id} value={instrument.id}>
                    {instrument.name}
                  </option>
                ))}
              </NativeSelect>
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="holding-quantity">{t("history.quantity")}</Label>
              <Input id="holding-quantity" value={quantity} onChange={(event) => setQuantity(event.target.value)} inputMode="decimal" />
            </div>
            {(formError || createHolding.isError) && (
              <p role="alert" className="text-sm text-destructive">
                {formError ?? displayError(createHolding.error, t("portfolio.createError"))}
              </p>
            )}
            <Button onClick={submit} disabled={createHolding.isPending}>
              {createHolding.isPending ? t("common.pending") : t("portfolio.addHolding")}
            </Button>
          </div>
        </SheetContent>
      </Sheet>

      {allHoldings.length === 0 ? (
        <EmptyState title={t("portfolio.noHoldings")} description={t("portfolio.noHoldingsDescription")} />
      ) : (
        <div className="overflow-x-auto rounded-lg border border-border">
          <table className="w-full min-w-[48rem] text-left text-sm" data-testid="holdings-table">
            <caption className="sr-only">{t("portfolio.holdingsTableLabel")}</caption>
            <thead>
              <tr className="border-b border-border bg-muted/40">
                <th className="px-3 py-2 font-medium text-muted-foreground">{t("history.instrument")}</th>
                <th className="px-3 py-2 font-medium text-muted-foreground">{t("history.accountSelect")}</th>
                <th className="px-3 py-2 font-medium text-muted-foreground">{t("portfolio.quantity")}</th>
                <th className="px-3 py-2 font-medium text-muted-foreground">{t("portfolio.cost")}</th>
                <th className="px-3 py-2 font-medium text-muted-foreground">{t("portfolio.currentValue")}</th>
                <th className="px-3 py-2 font-medium text-muted-foreground">{t("portfolio.unrealizedGain")}</th>
              </tr>
            </thead>
            <tbody>
              {allHoldings.map((holding) => {
                const gain = holdingGains.byHoldingId.get(holding.id);
                const gainClass =
                  gain?.unrealizedGain && gain.unrealizedGain.amount.startsWith("-") ? "text-gain-negative" : "text-gain-positive";
                return (
                  <tr key={holding.id} className="border-b border-border last:border-0">
                    <td className="px-3 py-3 font-medium">{holding.instrumentName ?? t("portfolio.unknownInstrument")}</td>
                    <td className="px-3 py-3 text-muted-foreground">{accountNameById.get(holding.accountId) ?? t("accounts.none")}</td>
                    <td className="px-3 py-3">{formatAmount(holding.quantity)}</td>
                    {gain ? (
                      gain.available ? (
                        <>
                          <td className="px-3 py-3">{formatAmount(gain.totalCost.amount, gain.totalCost.currency)}</td>
                          <td className="px-3 py-3">
                            {gain.currentValue ? formatAmount(gain.currentValue.amount, gain.currentValue.currency) : t("accounts.noValue")}
                          </td>
                          <td className={gainClass + " px-3 py-3"}>
                            {gain.unrealizedGain ? formatAmount(gain.unrealizedGain.amount, gain.unrealizedGain.currency) : t("accounts.noValue")}
                          </td>
                        </>
                      ) : (
                        <>
                          <td className="px-3 py-3">
                            {gain.totalCost ? formatAmount(gain.totalCost.amount, gain.totalCost.currency) : t("accounts.noValue")}
                          </td>
                          <td className="px-3 py-3 text-muted-foreground">{t("accounts.noValue")}</td>
                          <td className="px-3 py-3">
                            <Badge variant="secondary">
                              {gain.missingReason ? displayEnum(t, "portfolio.missingReason", gain.missingReason) : t("portfolio.unavailable")}
                            </Badge>
                          </td>
                        </>
                      )
                    ) : (
                      <td className="px-3 py-3 text-muted-foreground" colSpan={3} aria-label={t("portfolio.loading")}>
                        …
                      </td>
                    )}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

/** Investments owns instrument identity and holding positions under one
 * portfolio destination, while quotes and realized gain remain in Insights.
 */
export function InvestmentsPage() {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("nav.instruments")} description={t("portfolio.description")} />
      <Tabs defaultValue="instruments">
        <TabsList>
          <TabsTrigger value="instruments">{t("portfolio.instrumentsTab")}</TabsTrigger>
          <TabsTrigger value="holdings">{t("portfolio.holdingsTab")}</TabsTrigger>
        </TabsList>
        <TabsContent value="instruments">
          <InstrumentsTab />
        </TabsContent>
        <TabsContent value="holdings">
          <HoldingsTab />
        </TabsContent>
      </Tabs>
    </div>
  );
}
