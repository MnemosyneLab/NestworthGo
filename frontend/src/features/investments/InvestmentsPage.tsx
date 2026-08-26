import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Plus } from "lucide-react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { useAccounts } from "@/queries/accounts";
import {
  useInstruments,
  useCreateInstrument,
  useArchiveInstrument,
  useHoldingsByAccounts,
  useCreateHolding,
} from "@/queries/investments";
import { useHoldingGainsByAccounts } from "@/queries/analytics";
import { InstrumentForm } from "@/features/investments/InstrumentForm";
import { formatAmount } from "@/lib/money";

function InstrumentsTab() {
  const { t } = useTranslation();
  const instruments = useInstruments();
  const createInstrument = useCreateInstrument();
  const archiveInstrument = useArchiveInstrument();
  const [open, setOpen] = useState(false);

  return (
    <div className="flex flex-col gap-4">
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetTrigger className="self-start">
          <span className="inline-flex h-9 items-center gap-2 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground">
            <Plus className="size-4" /> {t("common.add")}
          </span>
        </SheetTrigger>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>{t("nav.investments")}</SheetTitle>
          </SheetHeader>
          <InstrumentForm
            isSubmitting={createInstrument.isPending}
            onSubmit={(request) =>
              createInstrument.mutate(request, {
                onSuccess: () => {
                  toast.success(t("common.add"));
                  setOpen(false);
                },
              })
            }
          />
        </SheetContent>
      </Sheet>

      {instruments.data && instruments.data.length === 0 && <p className="text-sm text-muted-foreground">{t("accounts.emptyDescription", { defaultValue: "No instruments yet" })}</p>}

      <ul className="flex flex-col gap-2">
        {(instruments.data ?? []).map((instrument) => (
          <li key={instrument.id} className="flex items-center justify-between rounded-md border border-border px-3 py-2 text-sm">
            <span className="flex items-center gap-2">
              {instrument.name}
              <Badge variant="secondary">{instrument.quoteCurrency}</Badge>
              <Badge variant={instrument.quoteSource === "manual" ? "outline" : "success"}>{instrument.quoteSource}</Badge>
              {instrument.archivedAt && <Badge variant="secondary">{t("common.archived")}</Badge>}
            </span>
            <Button
              variant="outline"
              size="sm"
              onClick={() => archiveInstrument.mutate({ id: instrument.id, archived: !instrument.archivedAt })}
            >
              {instrument.archivedAt ? t("common.active") : t("accounts.archive")}
            </Button>
          </li>
        ))}
      </ul>
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

  const holdingsAccounts = (accounts.data ?? []).filter((record) => record.account.trackingMode === "holdings" && !record.account.archivedAt);
  const accountIds = holdingsAccounts.map((record) => record.account.id);
  const holdingsByAccount = useHoldingsByAccounts(accountIds);
  const holdingGains = useHoldingGainsByAccounts(accountIds);

  const accountNameById = new Map(holdingsAccounts.map((record) => [record.account.id, record.account.name]));
  const instrumentNameById = new Map((instruments.data ?? []).map((instrument) => [instrument.id, instrument]));

  const allHoldings = Object.entries(holdingsByAccount.data ?? {}).flatMap(([accId, holdings]) => (holdings ?? []).map((holding) => ({ ...holding, accountId: accId })));

  const submit = () => {
    if (!accountId || !instrumentId || !quantity) {
      return;
    }
    createHolding.mutate(
      { accountId, instrumentId, quantity },
      {
        onSuccess: () => {
          toast.success(t("common.add"));
          setOpen(false);
          setAccountId("");
          setInstrumentId("");
          setQuantity("");
        },
      },
    );
  };

  if (holdingsAccounts.length === 0) {
    return <p className="text-sm text-muted-foreground">Create a Holdings-mode Account first to add holdings.</p>;
  }

  return (
    <div className="flex flex-col gap-4">
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetTrigger className="self-start">
          <span className="inline-flex h-9 items-center gap-2 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground">
            <Plus className="size-4" /> {t("common.add")}
          </span>
        </SheetTrigger>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>{t("nav.investments")}</SheetTitle>
          </SheetHeader>
          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="holding-account">Account</Label>
              <select id="holding-account" value={accountId} onChange={(event) => setAccountId(event.target.value)} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
                <option value="">{t("accounts.none")}</option>
                {holdingsAccounts.map((record) => (
                  <option key={record.account.id} value={record.account.id}>
                    {record.account.name}
                  </option>
                ))}
              </select>
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="holding-instrument">Instrument</Label>
              <select id="holding-instrument" value={instrumentId} onChange={(event) => setInstrumentId(event.target.value)} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
                <option value="">{t("accounts.none")}</option>
                {(instruments.data ?? []).map((instrument) => (
                  <option key={instrument.id} value={instrument.id}>
                    {instrument.name}
                  </option>
                ))}
              </select>
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="holding-quantity">Quantity</Label>
              <Input id="holding-quantity" value={quantity} onChange={(event) => setQuantity(event.target.value)} inputMode="decimal" />
            </div>
            {/* "Add holding" has no dedicated i18n catalog key yet; keep this
                small action label alongside the form's other short labels. */}
            <Button onClick={submit} disabled={createHolding.isPending}>
              Add holding
            </Button>
          </div>
        </SheetContent>
      </Sheet>

      {allHoldings.length === 0 && <p className="text-sm text-muted-foreground">No holdings yet</p>}

      <ul className="flex flex-col gap-2">
        {allHoldings.map((holding) => {
          const gain = holdingGains.byHoldingId.get(holding.id);
          return (
            <li key={holding.id} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 rounded-md border border-border px-3 py-2 text-sm">
              <span>
                {instrumentNameById.get(holding.instrumentId)?.name ?? holding.instrumentId} — {accountNameById.get(holding.accountId)}
              </span>
              <span className="text-muted-foreground">{formatAmount(holding.quantity)}</span>
              {gain ? (
                gain.available ? (
                  <>
                    <span className="text-muted-foreground" title="Cost">
                      {formatAmount(gain.totalCost.amount, gain.totalCost.currency)}
                    </span>
                    <span title="Current value">{gain.currentValue ? formatAmount(gain.currentValue.amount, gain.currentValue.currency) : "—"}</span>
                    <span
                      title="Unrealized gain"
                      className={gain.unrealizedGain && Number(gain.unrealizedGain.amount) < 0 ? "text-gain-negative" : "text-gain-positive"}
                    >
                      {gain.unrealizedGain ? formatAmount(gain.unrealizedGain.amount, gain.unrealizedGain.currency) : "—"}
                    </span>
                  </>
                ) : (
                  <Badge variant="secondary">{gain.missingReason || "Gain unavailable"}</Badge>
                )
              ) : (
                <span className="text-muted-foreground">…</span>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}

/** InvestmentsPage implements Instruments + Holdings end to end. Manual and
 * current quotes are surfaced on Analytics and History pages; this page owns
 * identity (Instrument) and position (Holding) management. */
export function InvestmentsPage() {
  const { t } = useTranslation();
  return (
    <Tabs defaultValue="instruments">
      <TabsList>
        <TabsTrigger value="instruments">{t("nav.investments")}</TabsTrigger>
        <TabsTrigger value="holdings">Holdings</TabsTrigger>
      </TabsList>
      <TabsContent value="instruments">
        <InstrumentsTab />
      </TabsContent>
      <TabsContent value="holdings">
        <HoldingsTab />
      </TabsContent>
    </Tabs>
  );
}
