import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { ArrowDown, ArrowUp, Plus } from "lucide-react";
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
  type SortingState,
} from "@tanstack/react-table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Button, buttonVariants } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { PageIntro } from "@/components/layout/PageHeader";
import { PageChrome } from "@/components/layout/PageChrome";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { useAccounts } from "@/queries/accounts";
import { useSettings } from "@/queries/settings";
import {
  useInstruments,
  useCreateInstrument,
  useUpdateInstrument,
  useArchiveInstrument,
  useCreateHolding,
  useAllHoldingsFlat,
  useCurrentInstrumentQuote,
  useAppendManualInstrumentQuote,
} from "@/queries/investments";
import { useHoldingGainsByAccounts } from "@/queries/analytics";
import { InstrumentForm } from "@/features/investments/InstrumentForm";
import { displayEnum, displayError } from "@/lib/display";
import { compareCanonical, formatAmount } from "@/lib/money";
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

function InstrumentRow({
  instrument,
  onEdit,
  onArchive,
}: {
  instrument: InstrumentDTO;
  onEdit: () => void;
  onArchive: () => void;
}) {
  const { t, i18n } = useTranslation();
  const settings = useSettings();
  const quote = useCurrentInstrumentQuote(instrument.id);
  const quoteTime = quote.data?.quotedAt ?? quote.data?.createdAt;

  return (
    <li className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-3 gap-y-2 rounded-md border border-border px-3 py-3 text-sm lg:grid-cols-[minmax(0,1fr)_minmax(15rem,auto)_auto]">
      <span className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
        <EntityIcon iconKey={instrument.iconKey} kind="instrument" className="size-5 text-primary" />
        <span className="font-medium">{instrument.name}</span>
        <Badge variant="secondary">{instrument.quoteCurrency}</Badge>
        <Badge variant={instrument.quoteSource === "manual" ? "outline" : "success"}>
          {displayEnum(t, "portfolio", instrument.quoteSource)}
        </Badge>
        {instrument.archivedAt && <Badge variant="secondary">{t("common.archived")}</Badge>}
      </span>
      <span className="col-span-2 row-start-2 flex min-w-0 flex-wrap items-center justify-end gap-x-3 gap-y-1 text-right lg:col-span-1 lg:col-start-2 lg:row-start-1">
        {quote.isLoading ? (
          <span className="text-muted-foreground">{t("portfolio.priceLoading")}</span>
        ) : quote.data ? (
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
        ) : (
          <span className="text-muted-foreground">{t("portfolio.noCurrentPrice")}</span>
        )}
      </span>
      <span className="col-start-2 row-start-1 flex shrink-0 items-center justify-end gap-2 lg:col-start-3">
        {!instrument.archivedAt && <Button type="button" variant="outline" size="sm" onClick={onEdit}>{t("common.edit")}</Button>}
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

function InstrumentsTab({ active }: { active: boolean }) {
  const { t } = useTranslation();
  const instruments = useInstruments();
  const createInstrument = useCreateInstrument();
  const updateInstrument = useUpdateInstrument();
  const archiveInstrument = useArchiveInstrument();
  const [open, setOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<InstrumentDTO | null>(null);
  const [showPriceForm, setShowPriceForm] = useState(false);

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
        pageId="investments"
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

      {(instruments.data ?? []).length === 0 ? (
        <EmptyState title={t("portfolio.noInstruments")} description={t("portfolio.noInstrumentsDescription")} />
      ) : (
        <div className="flex flex-col gap-4">
          {groupByInstrumentType(instruments.data ?? []).map((group) => (
            <section key={group.key} className="flex flex-col gap-2" data-testid={`instrument-type-group-${group.key}`}>
              <h2 className="text-sm font-medium text-muted-foreground">{displayEnum(t, "enum", group.key)}</h2>
              <ul className="flex flex-col gap-2">
                {group.items.map((instrument) => (
                  <InstrumentRow
                    key={instrument.id}
                    instrument={instrument}
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
        onOpenChange={(open) => {
          if (!open) {
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

type HoldingsIndexRow = {
  id: string;
  instrument: string;
  account: string;
  quantity: string;
  cost?: string;
  currentValue?: string;
  unrealized?: string;
  costLabel: string;
  currentLabel: string;
  unrealizedLabel?: string;
  missingLabel?: string;
  gainNegative: boolean;
};

const holdingsColumnHelper = createColumnHelper<HoldingsIndexRow>();

function HoldingsIndexTable({ rows }: { rows: HoldingsIndexRow[] }) {
  const { t } = useTranslation();
  const [sorting, setSorting] = useState<SortingState>([{ id: "currentValue", desc: true }]);
  const columns = useMemo(
    () => [
      holdingsColumnHelper.accessor("instrument", {
        header: t("history.instrument"),
        cell: (info) => info.getValue(),
      }),
      holdingsColumnHelper.accessor("account", {
        header: t("history.accountSelect"),
        cell: (info) => info.getValue(),
      }),
      holdingsColumnHelper.accessor("quantity", {
        header: t("portfolio.quantity"),
        sortingFn: (left, right, columnId) => compareCanonical(String(left.getValue(columnId) ?? "0"), String(right.getValue(columnId) ?? "0")),
        cell: (info) => formatAmount(info.getValue()),
      }),
      holdingsColumnHelper.accessor("cost", {
        header: t("portfolio.cost"),
        sortingFn: (left, right, columnId) => compareCanonical(String(left.getValue(columnId) ?? ""), String(right.getValue(columnId) ?? "")),
        sortUndefined: "last",
        cell: (info) => info.row.original.costLabel,
      }),
      holdingsColumnHelper.accessor("currentValue", {
        header: t("portfolio.currentValue"),
        sortingFn: (left, right, columnId) => compareCanonical(String(left.getValue(columnId) ?? ""), String(right.getValue(columnId) ?? "")),
        sortUndefined: "last",
        cell: (info) => info.row.original.currentLabel,
      }),
      holdingsColumnHelper.accessor("unrealized", {
        header: t("portfolio.unrealizedGain"),
        sortingFn: (left, right, columnId) => compareCanonical(String(left.getValue(columnId) ?? ""), String(right.getValue(columnId) ?? "")),
        sortUndefined: "last",
        cell: (info) =>
          info.row.original.missingLabel ? (
            <Badge variant="secondary">{info.row.original.missingLabel}</Badge>
          ) : (
            <span className={info.row.original.gainNegative ? "text-gain-negative" : "text-gain-positive"}>
              {info.row.original.unrealizedLabel ?? t("accounts.noValue")}
            </span>
          ),
      }),
    ],
    [t],
  );
  // TanStack Table's stateful table instance is intentionally consumed directly.
  // eslint-disable-next-line react-hooks/incompatible-library -- the API is the table's supported sorting boundary
  const table = useReactTable({
    data: rows,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  return (
    <div className="overflow-x-auto rounded-lg border border-border">
      <table className="w-full min-w-[48rem] text-left text-sm" data-testid="holdings-table">
        <caption className="sr-only">{t("portfolio.holdingsTableLabel")}</caption>
        <thead>
          {table.getHeaderGroups().map((headerGroup) => (
            <tr key={headerGroup.id} className="border-b border-border bg-muted/40">
              {headerGroup.headers.map((header) => {
                const sorted = header.column.getIsSorted();
                return (
                  <th key={header.id} className="px-3 py-2 font-medium text-muted-foreground">
                    <button type="button" className="inline-flex items-center gap-1" onClick={header.column.getToggleSortingHandler()}>
                      {flexRender(header.column.columnDef.header, header.getContext())}
                      {sorted === "asc" && <ArrowUp className="size-3" aria-hidden="true" />}
                      {sorted === "desc" && <ArrowDown className="size-3" aria-hidden="true" />}
                    </button>
                  </th>
                );
              })}
            </tr>
          ))}
        </thead>
        <tbody>
          {table.getRowModel().rows.map((row) => (
            <tr key={row.id} className="border-b border-border last:border-0">
              {row.getVisibleCells().map((cell) => (
                <td key={cell.id} className="px-3 py-3">
                  {flexRender(cell.column.columnDef.cell, cell.getContext())}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function HoldingsTab({ active }: { active: boolean }) {
  const { t } = useTranslation();
  const accounts = useAccounts({});
  const instruments = useInstruments();
  const createHolding = useCreateHolding();
  const [open, setOpen] = useState(false);
  const [accountId, setAccountId] = useState("");
  const [instrumentId, setInstrumentId] = useState("");
  const [quantity, setQuantity] = useState("");
  const [formError, setFormError] = useState<string | undefined>();

  const holdingsAccounts = (accounts.data ?? []).filter(
    (record) => record.account.trackingMode === "holdings" && record.account.accountType !== "cash_on_hand" && !record.account.archivedAt,
  );
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
      <PageChrome
        pageId="investments"
        enabled={active}
        actions={
          <Sheet open={open} onOpenChange={setOpen}>
            <SheetTrigger className={buttonVariants({ size: "sm" })}>
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
        }
      />

      {allHoldings.length === 0 ? (
        <EmptyState title={t("portfolio.noHoldings")} description={t("portfolio.noHoldingsDescription")} />
      ) : (
        <HoldingsIndexTable
          rows={allHoldings.map((holding) => {
            const gain = holdingGains.byHoldingId.get(holding.id);
            return {
              id: holding.id,
              instrument: holding.instrumentName ?? t("portfolio.unknownInstrument"),
              account: accountNameById.get(holding.accountId) ?? t("accounts.none"),
              quantity: holding.quantity,
              cost: gain?.totalCost?.amount,
              currentValue: gain?.available ? gain.currentValue?.amount : undefined,
              unrealized: gain?.available ? gain.unrealizedGain?.amount : undefined,
              costLabel: gain?.totalCost ? formatAmount(gain.totalCost.amount, gain.totalCost.currency) : gain ? t("accounts.noValue") : "…",
              currentLabel:
                gain?.available && gain.currentValue
                  ? formatAmount(gain.currentValue.amount, gain.currentValue.currency)
                  : gain
                    ? t("accounts.noValue")
                    : "…",
              unrealizedLabel:
                gain?.available && gain.unrealizedGain
                  ? formatAmount(gain.unrealizedGain.amount, gain.unrealizedGain.currency)
                  : undefined,
              missingLabel:
                gain && !gain.available
                  ? gain.missingReason
                    ? displayEnum(t, "portfolio.missingReason", gain.missingReason)
                    : t("portfolio.unavailable")
                  : undefined,
              gainNegative: Boolean(gain?.unrealizedGain?.amount.startsWith("-")),
            };
          })}
        />
      )}
    </div>
  );
}

/** Investments owns instrument identity and holding positions under one
 * portfolio destination, while quotes and realized gain remain in Insights.
 */
export function InvestmentsPage() {
  const { t } = useTranslation();
  const [tab, setTab] = useState("instruments");
  return (
    <div className="flex flex-col gap-6">
      <PageChrome pageId="investments" title={t("nav.instruments")} />
      <PageIntro description={t("portfolio.description")} />
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="instruments">{t("portfolio.instrumentsTab")}</TabsTrigger>
          <TabsTrigger value="holdings">{t("portfolio.holdingsTab")}</TabsTrigger>
        </TabsList>
        <TabsContent value="instruments">
          <InstrumentsTab active={tab === "instruments"} />
        </TabsContent>
        <TabsContent value="holdings">
          <HoldingsTab active={tab === "holdings"} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
