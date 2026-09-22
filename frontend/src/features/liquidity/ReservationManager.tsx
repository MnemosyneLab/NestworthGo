import { sourceName } from "./sourceName";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { NativeSelect } from "@/components/ui/select";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { useReservations, useReleaseReservation } from "@/queries/liquidity";
import { displayError } from "@/lib/display";
import { formatAmount } from "@/lib/money";
import { ReservationSheet } from "./ProductSheets";
import type { LiquiditySourceDTO, ReservationDTO, SourceRefDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
const key = (s: SourceRefDTO) => `${s.kind}:${s.accountId}:${s.holdingId ?? ""}:${s.kind === "account_cash" ? s.currency : ""}`;
export function ReservationManager({ source, sources = [], open, onOpenChange }: { sources?: LiquiditySourceDTO[]; source: LiquiditySourceDTO | null; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useTranslation(); const reservations = useReservations(open); const release = useReleaseReservation();
  const [selectedSourceKey, setSelectedSourceKey] = useState("");
  const selectedSource = source ?? sources.find((item) => item.sourceKey === selectedSourceKey) ?? null;
  const [editing,setEditing] = useState<ReservationDTO | "new" | null>(null); const [error,setError] = useState<string>();
  if (editing) return <ReservationSheet key={editing === "new" ? "new" : `${editing.id}:${editing.revision}`} source={editing === "new" ? selectedSource : sources.find((item) => key(item.sourceRef) === key(editing.sourceRef)) ?? source} reservation={editing === "new" ? undefined : editing} open={open} onOpenChange={(next) => { if (!next) setEditing(null); }} />;
  const items = (reservations.data ?? []).filter((r) => !source || key(r.sourceRef) === key(source.sourceRef));
  return <Sheet open={open} onOpenChange={onOpenChange}><SheetContent className="overflow-y-auto">
    <SheetHeader><SheetTitle>{t("availableFunds.reservations")}</SheetTitle></SheetHeader>
    {source ? <p>{sourceName(t, source)}</p> : <>
      <Label htmlFor="reservation-source">{t("availableFunds.reservationSource")}</Label>
      <NativeSelect id="reservation-source" value={selectedSourceKey} onChange={(e) => setSelectedSourceKey(e.target.value)}>
        <option value="">{t("availableFunds.chooseSource")}</option>
        {sources.filter((item) => !item.excluded).map((item) => <option key={item.sourceKey} value={item.sourceKey}>{sourceName(t, item)} · {item.nativeCurrency}</option>)}
      </NativeSelect>
    </>}
    <Button disabled={!selectedSource} onClick={() => setEditing("new")}>{t("availableFunds.addReservation")}</Button>
    {!reservations.isLoading && !reservations.isError && items.length === 0 && <p>{t(source ? "availableFunds.noReservations" : "availableFunds.noReservationsChooseSource")}</p>}
    {reservations.isLoading && <p>{t("ui.state.loadingPage")}</p>}
    {reservations.isError && <p role="alert">{t("availableFunds.loadError")}</p>}
    {items.map((r) => <div key={r.id} className="my-3 flex flex-col gap-2 rounded-md border p-3">
      <p className="text-muted-foreground">{(source ? sourceName(t, source) : undefined) ?? sources.filter((s) => key(s.sourceRef) === key(r.sourceRef)).map((s) => sourceName(t, s))[0] ?? t("availableFunds.reservationSourceUnavailable")}</p>
      <p>{r.label} · {formatAmount(r.amount.amount,r.amount.currency)}</p>
      {r.releasedAt ? <p>{t("availableFunds.reservationReleased")}</p> : <div className="flex gap-2">
        <Button variant="outline" onClick={() => setEditing(r)}>{t("availableFunds.editReservation")}</Button>
        <Button variant="outline" disabled={release.isPending} onClick={() => { setError(undefined); void release.mutateAsync({ id:r.id,expectedRevision:r.revision }).catch((e) => setError(displayError(e,t("availableFunds.loadError")))); }}>{t("availableFunds.releaseReservation")}</Button>
      </div>}
    </div>)}
    {error && <p role="alert">{error}</p>}
  </SheetContent></Sheet>;
}
