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
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { PageIntro } from "@/components/layout/PageHeader";
import { PageChrome } from "@/components/layout/PageChrome";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { useAccounts } from "@/queries/accounts";
import {
  useInstruments,
  useCreateHolding,
  useAllHoldingsFlat,
} from "@/queries/investments";
import { useHoldingGainsByAccounts } from "@/queries/analytics";
import { displayEnum, displayError } from "@/lib/display";
import { compareCanonical, formatAmount } from "@/lib/money";

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

/** Investments is the household holdings index. Instrument identity, quotes,
 * and provider settings live on Market Data.
 */
export function InvestmentsPage() {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-6">
      <PageChrome pageId="investments" title={t("nav.holdings")} />
      <PageIntro description={t("portfolio.description")} />
      <HoldingsTab />
    </div>
  );
}
