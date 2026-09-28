import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { displayError } from "@/lib/display";
import { useSaveReservation } from "@/queries/liquidity";
import type { LiquiditySourceDTO, ReservationDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import { sourceName } from "./sourceName";

type ReservationFormProps = {
  source: LiquiditySourceDTO | null;
  sources?: LiquiditySourceDTO[];
  accountNames?: Record<string, string>;
  reservation?: ReservationDTO;
  onSaved: () => void;
  onCancel: () => void;
  onPendingChange?: (pending: boolean) => void;
};
type FieldErrors = Partial<Record<"source" | "label" | "amount", string>>;

export function ReservationForm({ source, sources = [], accountNames = {}, reservation, onSaved, onCancel, onPendingChange }: ReservationFormProps) {
  const { t } = useTranslation();
  const save = useSaveReservation();
  const eligibleSources = sources.filter((item) => !item.excluded);
  const [selectedSourceKey, setSelectedSourceKey] = useState(eligibleSources.length === 1 ? eligibleSources[0].sourceKey : "");
  const selectedSource = source ?? eligibleSources.find((item) => item.sourceKey === selectedSourceKey) ?? null;
  const [label, setLabel] = useState(reservation?.label ?? "");
  const [amount, setAmount] = useState(reservation?.amount.amount ?? "");
  const [errors, setErrors] = useState<FieldErrors>({});
  const [error, setError] = useState<string>();
  const pending = useRef(false);
  const currency = reservation?.amount.currency ?? selectedSource?.nativeCurrency;
  const describeSource = (item: LiquiditySourceDTO) => [accountNames[item.accountId], sourceName(t, item), item.nativeCurrency].filter(Boolean).join(" · ");

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending.current) return;
    const nextErrors: FieldErrors = {};
    if (!selectedSource && !reservation) nextErrors.source = t("availableFunds.reservationSourceRequired");
    if (!label.trim()) nextErrors.label = t("availableFunds.reservationLabelRequired");
    // Keep monetary values as strings. The service supports up to four fractional digits.
    const normalizedAmount = amount.trim();
    if (!/^(0|[1-9]\d{0,11})(\.\d{1,4})?$/.test(normalizedAmount) || !/[1-9]/.test(normalizedAmount)) {
      nextErrors.amount = t("availableFunds.reservationAmountPositive");
    }
    setErrors(nextErrors);
    setError(undefined);
    if (Object.keys(nextErrors).length > 0) {
      const field = nextErrors.source ? "reservation-source" : nextErrors.label ? "reserve-label" : "reserve-amount";
      event.currentTarget.querySelector<HTMLElement>(`#${field}`)?.focus();
      return;
    }
    pending.current = true;
    onPendingChange?.(true);
    try {
      await save.mutateAsync({
        id: reservation?.id,
        sourceRef: reservation?.sourceRef ?? selectedSource!.sourceRef,
        expectedRevision: reservation?.revision ?? 0,
        label: label.trim(),
        amount: normalizedAmount,
      });
      onSaved();
    } catch (cause) {
      const message = displayError(cause, t("availableFunds.loadError"));
      const field = (cause as { wireError?: { field?: string } })?.wireError?.field;
      if (field === "label" || field === "amount") setErrors({ [field]: message });
      else setError(message);
    } finally {
      pending.current = false;
      onPendingChange?.(false);
    }
  }

  return (
    <form onSubmit={(event) => { void submit(event); }} noValidate className="flex flex-1 flex-col gap-5">
      <div className="space-y-2">
        {source || reservation ? <>
          <p className="text-sm font-medium">{t("availableFunds.reservationSource")}</p>
          <p className="text-sm text-muted-foreground">{selectedSource ? describeSource(selectedSource) : t("availableFunds.reservationSourceUnavailable")}</p>
        </> : <>
          <Label htmlFor="reservation-source">{t("availableFunds.reservationSource")}</Label>
          <NativeSelect id="reservation-source" autoFocus value={selectedSourceKey} disabled={save.isPending} aria-invalid={Boolean(errors.source)} aria-describedby={errors.source ? "reservation-source-error" : undefined} onChange={(event) => { setSelectedSourceKey(event.target.value); setErrors((current) => ({ ...current, source: undefined })); }}>
            <option value="">{t("availableFunds.chooseSource")}</option>
            {eligibleSources.map((item) => <option key={item.sourceKey} value={item.sourceKey}>{describeSource(item)}</option>)}
          </NativeSelect>
          {errors.source && <p id="reservation-source-error" role="alert" className="text-sm text-destructive">{errors.source}</p>}
        </>}
      </div>
      <div className="space-y-2">
        <Label htmlFor="reserve-label">{t("availableFunds.reservationLabel")}</Label>
        <Input id="reserve-label" autoFocus={Boolean(source || reservation)} value={label} disabled={save.isPending} aria-invalid={Boolean(errors.label)} aria-describedby={errors.label ? "reserve-label-error" : undefined} onChange={(event) => { setLabel(event.target.value); setErrors((current) => ({ ...current, label: undefined })); }} />
        {errors.label && <p id="reserve-label-error" role="alert" className="text-sm text-destructive">{errors.label}</p>}
      </div>
      <div className="space-y-2">
        <Label htmlFor="reserve-amount">{t("availableFunds.reservationAmount")}</Label>
        <div className="flex items-center gap-3">
          <Input id="reserve-amount" inputMode="decimal" value={amount} disabled={save.isPending} aria-invalid={Boolean(errors.amount)} aria-describedby={errors.amount ? "reserve-amount-error reserve-currency" : "reserve-currency"} onChange={(event) => { setAmount(event.target.value); setErrors((current) => ({ ...current, amount: undefined })); }} />
          <span id="reserve-currency" className="text-sm font-medium text-muted-foreground">{currency}</span>
        </div>
        {errors.amount && <p id="reserve-amount-error" role="alert" className="text-sm text-destructive">{errors.amount}</p>}
      </div>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <SheetFooter>
        <Button type="button" variant="outline" disabled={save.isPending} onClick={onCancel}>{t("common.cancel")}</Button>
        <Button type="submit" disabled={save.isPending}>{t("availableFunds.confirm")}</Button>
      </SheetFooter>
    </form>
  );
}

export function ReservationSheet({ source, reservation, accountNames, open, onOpenChange }: {
  source: LiquiditySourceDTO | null;
  accountNames?: Record<string, string>;
  reservation?: ReservationDTO;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const [pending, setPending] = useState(false);
  if (!source && !reservation) return null;
  return (
    <Sheet open={open} onOpenChange={(next) => { if (!pending) onOpenChange(next); }}>
      <SheetContent className="overflow-y-auto">
        <SheetHeader>
          <SheetTitle>{t(reservation ? "availableFunds.editReservation" : "availableFunds.addReservation")}</SheetTitle>
          <SheetDescription>{t("availableFunds.reservationHelp")}</SheetDescription>
        </SheetHeader>
        {open && <ReservationForm key={reservation ? `${reservation.id}:${reservation.revision}` : source?.sourceKey} source={source} reservation={reservation} accountNames={accountNames} onSaved={() => onOpenChange(false)} onCancel={() => onOpenChange(false)} onPendingChange={setPending} />}
      </SheetContent>
    </Sheet>
  );
}
