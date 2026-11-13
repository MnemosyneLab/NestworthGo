import { useId, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button, buttonVariants } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { ErrorState, LoadingState } from "@/components/layout/PageState";
import { ymdInTimeZone } from "@/features/insights/calendar";
import { displayError } from "@/lib/display";
import { instrumentDisplayLabel } from "@/lib/instrumentDisplay";
import { formatCivilDateRange } from "@/lib/time";
import { useHistoryOrigin } from "@/queries/history";
import { useAppendManualFXQuote, useInstruments } from "@/queries/investments";
import { useSupportedCurrencies } from "@/queries/settings";
import type { HealthFocus } from "@/app/navigation";
import { ManualInstrumentQuoteForm } from "./ManualInstrumentQuoteForm";

function ManualFXQuoteForm({ onSaved, focus }: { onSaved: () => void; focus?: HealthFocus }) {
  const { t } = useTranslation();
  const currencies = useSupportedCurrencies();
  const origin = useHistoryOrigin();
  const today = ymdInTimeZone(new Date(), origin.data?.timezone ?? "UTC");
  const appendQuote = useAppendManualFXQuote();
  const options = currencies.data ?? [];
  const [baseCurrency, setBaseCurrency] = useState(focus?.currencyA ?? "");
  const [quoteCurrency, setQuoteCurrency] = useState(focus?.currencyB ?? "");
  const [rate, setRate] = useState("");
  const [effectiveDate, setEffectiveDate] = useState(focus?.rangeStart ?? "");
  const directionId = useId();
  const selectedBaseCurrency = baseCurrency || options[0] || "";
  const selectedQuoteCurrency = quoteCurrency || options[1] || options[0] || "";
  const invalidPair = !selectedBaseCurrency || !selectedQuoteCurrency || selectedBaseCurrency === selectedQuoteCurrency;

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (invalidPair || !rate.trim()) return;
    appendQuote.mutate({
      baseCurrency: selectedBaseCurrency,
      quoteCurrency: selectedQuoteCurrency,
      rate: rate.trim(),
      quotedAt: effectiveDate,
    }, { onSuccess: () => { toast.success(t("common.saved")); onSaved(); } });
  };

  return (
    <form onSubmit={submit} className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="manual-fx-base">{t("marketData.baseCurrency")}</Label>
        <NativeSelect id="manual-fx-base" value={selectedBaseCurrency} onChange={(event) => setBaseCurrency(event.target.value)}>
          {options.map((currency) => <option key={currency} value={currency}>{currency}</option>)}
        </NativeSelect>
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="manual-fx-quote">{t("marketData.quoteCurrency")}</Label>
        <NativeSelect id="manual-fx-quote" value={selectedQuoteCurrency} onChange={(event) => setQuoteCurrency(event.target.value)}>
          {options.map((currency) => <option key={currency} value={currency}>{currency}</option>)}
        </NativeSelect>
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="manual-fx-rate">{t("marketData.rate")}</Label>
        <Input id="manual-fx-rate" inputMode="decimal" value={rate} onChange={(event) => setRate(event.target.value)} aria-describedby={directionId} />
        {!invalidPair && <p id={directionId} className="num text-sm text-muted-foreground">{t("marketData.latestRate", { base: selectedBaseCurrency, rate: rate.trim() || "…", quote: selectedQuoteCurrency })}</p>}
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="manual-fx-date">{t("marketData.effectiveDate")}</Label>
        <DatePicker id="manual-fx-date" max={today} value={effectiveDate} onChange={setEffectiveDate} />
        <p className="text-xs text-muted-foreground">{t("connections.manualDateZone")}</p>
      </div>
      {invalidPair && <p className="text-sm text-destructive">{t("marketData.invalidPair")}</p>}
      {appendQuote.isError && <p role="alert" className="text-sm text-destructive">{displayError(appendQuote.error, t("marketData.loadError"))}</p>}
      <Button type="submit" disabled={appendQuote.isPending || invalidPair || !rate.trim()} className="mt-auto w-full">
        {appendQuote.isPending ? t("common.pending") : t("marketData.saveManualFX")}
      </Button>
    </form>
  );
}

export function ManualQuoteEntry({ focus }: { focus?: HealthFocus }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const isInstrument = Boolean(focus?.instrumentId);
  const title = t(isInstrument ? "portfolio.setPrice" : "marketData.manualFXTitle");
  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger className={buttonVariants(focus ? { size: "sm", variant: "outline" } : { size: "sm" })}>
        {focus ? t("dataHealth.addManual") : title}
      </SheetTrigger>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          {focus?.rangeStart && <SheetDescription>{formatCivilDateRange(focus.rangeStart, focus.rangeEnd || focus.rangeStart)}</SheetDescription>}
          {!isInstrument && <p className="text-sm text-muted-foreground">{t("marketData.manualFXDescription")}</p>}
        </SheetHeader>
        {isInstrument && focus?.instrumentId
          ? <FocusedInstrumentQuote instrumentId={focus.instrumentId} initialDate={focus.rangeStart} onSaved={() => setOpen(false)} />
          : <ManualFXQuoteForm focus={focus} onSaved={() => setOpen(false)} />}
      </SheetContent>
    </Sheet>
  );
}

function FocusedInstrumentQuote({ instrumentId, initialDate, onSaved }: { instrumentId: string; initialDate?: string; onSaved: () => void }) {
  const { t } = useTranslation();
  const instruments = useInstruments(true);
  if (instruments.isLoading) return <LoadingState label={t("common.loading")} />;
  if (instruments.isError) return <ErrorState title={t("portfolio.loadError")} description={t("ui.state.errorDescription")} onRetry={() => void instruments.refetch()} retryLabel={t("common.retryAction")} />;
  const instrument = instruments.data?.find(item => item.id === instrumentId);
  if (!instrument || instrument.archivedAt || instrument.managedProductId || instrument.quoteSource === "agent") return <p role="status">{t("connections.objectUnavailable")}</p>;
  return <>
    <p className="font-medium">{instrumentDisplayLabel(instrument, instrument.name)}</p>
    <ManualInstrumentQuoteForm instrumentId={instrument.id} currency={instrument.quoteCurrency} quantityUnit={instrument.quantityUnit} initialDate={initialDate} onSaved={onSaved} />
  </>;
}
