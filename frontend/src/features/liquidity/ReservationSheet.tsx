import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Sheet, SheetContent, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { displayError } from "@/lib/display";
import {
  useSaveReservation
} from "@/queries/liquidity";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { LiquiditySourceDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import { sourceName } from "./sourceName";


export function ReservationSheet({
  source, reservation,
  open,
  onOpenChange,
}: {
  source: LiquiditySourceDTO | null;
  reservation?: import("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models").ReservationDTO;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const save = useSaveReservation();
  const [label, setLabel] = useState(reservation?.label ?? "");
  const [amount, setAmount] = useState(reservation?.amount.amount ?? "");
  const [error, setError] = useState<string>();
  if (!source && !reservation) {
    return null;
  }
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>{t(reservation ? "availableFunds.editReservation" : "availableFunds.addReservation")}</SheetTitle>
        </SheetHeader>
        <p className="text-sm text-muted-foreground">{source ? sourceName(t, source) : ""} · {reservation?.amount.currency ?? source?.nativeCurrency}</p>
        <Label htmlFor="reserve-label">{t("availableFunds.reservationLabel")}</Label>
        <Input id="reserve-label" value={label} onChange={(event) => setLabel(event.target.value)} />
        <Label htmlFor="reserve-amount">{t("availableFunds.reservationAmount")}</Label>
        <Input id="reserve-amount" value={amount} onChange={(event) => setAmount(event.target.value)} />
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <SheetFooter>
          <Button type="button" disabled={save.isPending} onClick={() => {
            if (save.isPending) return;
            setError(undefined);
            void save.mutateAsync({ id: reservation?.id, sourceRef: reservation?.sourceRef ?? source!.sourceRef, expectedRevision: reservation?.revision ?? 0, label, amount }).then(() => onOpenChange(false)).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
          }}>{t("availableFunds.confirm")}</Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
