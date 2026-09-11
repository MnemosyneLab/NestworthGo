import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Plus } from "lucide-react";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Button, buttonVariants } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { PageChrome } from "@/components/layout/PageChrome";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { useSettings } from "@/queries/settings";
import {
  useInstruments,
  useCreateInstrument,
  useUpdateInstrument,
  useArchiveInstrument,
  useCurrentInstrumentQuote,
  useAppendManualInstrumentQuote,
} from "@/queries/investments";
import { InstrumentForm } from "@/features/investments/InstrumentForm";
import { displayEnum, displayError } from "@/lib/display";
import { formatAmount } from "@/lib/money";
import { formatTimestamp } from "@/lib/time";
import { groupByInstrumentType } from "@/lib/groupByInstrumentType";
import { cn } from "@/lib/utils";
import { EntityIcon } from "@/components/icons/EntityIcon";
import type { InstrumentDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
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
  const appendQuote = useAppendManualInstrumentQuote();
  const [unitPrice, setUnitPrice] = useState("");
  const [quotedAt, setQuotedAt] = useState("");

  const submit = () => {
    if (!unitPrice.trim()) {
      return;
    }
    appendQuote.mutate(
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
        <DatePicker id={`quote-date-${instrumentId}`} value={quotedAt} onChange={setQuotedAt} />
      </div>
      {appendQuote.isError && (
        <p role="alert" className="text-sm text-destructive">
          {displayError(appendQuote.error, t("portfolio.updateError"))}
        </p>
      )}
      <Button onClick={submit} disabled={appendQuote.isPending || !unitPrice.trim()}>
        {appendQuote.isPending ? t("common.pending") : t("portfolio.setPrice")}
      </Button>
    </div>
  );
}

function QuoteQuality({
  sourceKind,
  sourceKey,
  delayed,
}: {
  sourceKind?: string;
  sourceKey?: string;
  delayed?: boolean;
}) {
  const { t } = useTranslation();
  const source = sourceKind ? displayEnum(t, "portfolio", sourceKind) : "";
  const label = sourceKey ? `${source} · ${sourceKey}` : source;
  return (
    <span className="flex flex-wrap items-center justify-end gap-1 text-xs text-muted-foreground">
      {delayed ? <Badge variant="warning">{t("charts.delayed")}</Badge> : null}
      {label ? <span>{label}</span> : null}
    </span>
  );
}

function InstrumentRow({
  instrument,
  groupTestId,
  onEdit,
  onArchive,
  onViewHistory,
  onSync,
  isSyncing,
}: {
  instrument: InstrumentDTO;
  groupTestId: string;
  onEdit: () => void;
  onArchive: () => void;
  onViewHistory?: () => void;
  onSync?: () => void;
  isSyncing?: boolean;
}) {
  const { t, i18n } = useTranslation();
  const settings = useSettings();
  const quote = useCurrentInstrumentQuote(instrument.id);
  const quoteTime = quote.data?.quotedAt ?? quote.data?.createdAt;
  const rowTestId = groupTestId.startsWith("market-data-group-") ? `saved-instrument-${instrument.id}` : undefined;

  return (
    <li
      className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-2 rounded-md border border-border px-3 py-3 text-sm lg:grid-cols-[minmax(0,1fr)_minmax(15rem,auto)_auto]"
      data-testid={rowTestId}
    >
      <span className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
        <EntityIcon iconKey={instrument.iconKey} kind="instrument" className="size-5 text-primary" />
        <span className="font-medium">{instrument.name}</span>
        <Badge variant="secondary">{instrument.quoteCurrency}</Badge>
        <Badge variant={instrument.quoteSource === "manual" ? "outline" : "success"}>
          {displayEnum(t, "portfolio", instrument.quoteSource)}
        </Badge>
        {instrument.archivedAt && <Badge variant="secondary">{t("common.archived")}</Badge>}
      </span>
      <span className="col-span-2 row-start-2 flex min-w-0 flex-col items-end justify-end gap-1 text-right lg:col-span-1 lg:col-start-2 lg:row-start-1">
        {quote.isLoading ? (
          <span className="text-muted-foreground">{t("portfolio.priceLoading")}</span>
        ) : quote.data ? (
          <>
            <span>
              <span className="font-medium">{t("portfolio.latestPrice", { value: formatAmount(quote.data.unitPrice, quote.data.currency) })}</span>
              {quoteTime && (
                <span className="ml-2 text-xs text-muted-foreground">
                  {t("portfolio.quotedAsOf", {
                    time: formatTimestamp(quoteTime, settings.data?.timezone, i18n.language),
                  })}
                </span>
              )}
            </span>
            <QuoteQuality sourceKind={quote.data.sourceKind} sourceKey={quote.data.sourceKey} delayed={quote.data.delayed} />
          </>
        ) : (
          <span className="text-muted-foreground">{t("portfolio.noCurrentPrice")}</span>
        )}
      </span>
      <span className="col-start-2 row-start-1 flex shrink-0 items-center justify-end gap-2 lg:col-start-3">
        {onViewHistory && (
          <Button type="button" variant="outline" size="sm" onClick={onViewHistory}>
            {t("charts.viewHistory")}
          </Button>
        )}
        {onSync && instrument.quoteSource === "provider" && !instrument.archivedAt && (
          <Button type="button" variant="outline" size="sm" onClick={onSync} disabled={isSyncing}>
            {isSyncing ? t("marketData.syncingInstrument") : t("marketData.syncInstrument")}
          </Button>
        )}
        {!instrument.archivedAt && (
          <Button type="button" variant="outline" size="sm" onClick={onEdit}>
            {t("common.edit")}
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

export function InstrumentManagement({
  pageId,
  active,
  enableSearch = false,
  groupTestIdPrefix = "instrument-type-group",
  onViewHistory,
  onSyncInstrument,
  syncingInstrumentId,
}: {
  pageId: string;
  active: boolean;
  enableSearch?: boolean;
  groupTestIdPrefix?: string;
  onViewHistory?: (instrument: InstrumentDTO) => void;
  onSyncInstrument?: (instrument: InstrumentDTO) => void;
  syncingInstrumentId?: string;
}) {
  const { t } = useTranslation();
  const instruments = useInstruments();
  const createInstrument = useCreateInstrument();
  const updateInstrument = useUpdateInstrument();
  const archiveInstrument = useArchiveInstrument();
  const [open, setOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<InstrumentDTO | null>(null);
  const [showPriceForm, setShowPriceForm] = useState(false);
  const [query, setQuery] = useState("");

  const filtered = useMemo(() => {
    const items = instruments.data ?? [];
    const needle = query.trim().toLowerCase();
    if (!needle) {
      return items;
    }
    return items.filter((instrument) => {
      const haystack = [instrument.name, instrument.symbol, instrument.providerSymbol, instrument.quoteCurrency].filter(Boolean).join(" ").toLowerCase();
      return haystack.includes(needle);
    });
  }, [instruments.data, query]);

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
      <PageChrome
        pageId={pageId}
        enabled={active}
        actions={
          <Sheet open={open} onOpenChange={setOpen}>
            <SheetTrigger className={buttonVariants({ size: "sm" })}>
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
        }
      />

      {enableSearch && (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="instrument-search">{t("marketData.searchInstruments")}</Label>
          <Input
            id="instrument-search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={t("marketData.searchInstrumentsPlaceholder")}
          />
        </div>
      )}

      {filtered.length === 0 ? (
        <EmptyState title={t("portfolio.noInstruments")} description={t("portfolio.noInstrumentsDescription")} />
      ) : (
        <div className="flex flex-col gap-4">
          {groupByInstrumentType(filtered).map((group) => (
            <section key={group.key} className="flex flex-col gap-2" data-testid={`${groupTestIdPrefix}-${group.key}`}>
              <h2 className="text-sm font-medium text-muted-foreground">{displayEnum(t, "enum", group.key)}</h2>
              <ul className="flex flex-col gap-2">
                {group.items.map((instrument) => (
                  <InstrumentRow
                    key={instrument.id}
                    instrument={instrument}
                    groupTestId={`${groupTestIdPrefix}-${group.key}`}
                    onViewHistory={onViewHistory ? () => onViewHistory(instrument) : undefined}
                    onSync={onSyncInstrument ? () => onSyncInstrument(instrument) : undefined}
                    isSyncing={syncingInstrumentId === instrument.id}
                    onEdit={() => {
                      setShowPriceForm(false);
                      setEditTarget(instrument);
                    }}
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
            </section>
          ))}
        </div>
      )}
      <Sheet
        open={Boolean(editTarget)}
        onOpenChange={(openSheet) => {
          if (!openSheet) {
            setEditTarget(null);
            setShowPriceForm(false);
          }
        }}
      >
        <SheetContent>
          <SheetHeader>
            <SheetTitle>{t("portfolio.editInstrument")}</SheetTitle>
          </SheetHeader>
          {editTarget && (
            <InstrumentForm
              key={editTarget.id}
              instrument={editTarget}
              submitLabel={t("common.save")}
              isSubmitting={updateInstrument.isPending}
              submissionError={updateInstrument.isError ? displayError(updateInstrument.error, t("portfolio.updateError")) : undefined}
              onSubmit={(request) =>
                updateInstrument.mutate(
                  { id: editTarget.id, request },
                  {
                    onSuccess: () => {
                      toast.success(t("common.saved"));
                      setShowPriceForm(false);
                      setEditTarget(null);
                    },
                  },
                )
              }
            />
          )}
          {editTarget && (
            <div className="mt-5 border-t border-border pt-5">
              <Button type="button" variant="outline" onClick={() => setShowPriceForm((value) => !value)}>
                {t("portfolio.setPrice")}
              </Button>
              {showPriceForm && (
                <div className="mt-4">
                  <ManualQuoteForm
                    key={`${editTarget.id}-price`}
                    instrumentId={editTarget.id}
                    currency={editTarget.quoteCurrency}
                    onSaved={() => setShowPriceForm(false)}
                  />
                </div>
              )}
            </div>
          )}
        </SheetContent>
      </Sheet>
    </div>
  );
}
