import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { useReservations, useReleaseReservation } from "@/queries/liquidity";
import { displayError } from "@/lib/display";
import { formatAmount } from "@/lib/money";
import { ReservationForm } from "./ReservationSheet";
import { sourceName } from "./sourceName";
import type { LiquiditySourceDTO, ReservationDTO, SourceRefDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

const key = (s: SourceRefDTO) => `${s.kind}:${s.accountId}:${s.holdingId ?? ""}:${s.kind === "account_cash" ? s.currency : ""}`;

export function ReservationManager({ source, sources = [], accountNames = {}, open, onOpenChange }: {
  sources?: LiquiditySourceDTO[];
  source: LiquiditySourceDTO | null;
  accountNames?: Record<string, string>;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const reservations = useReservations(open);
  const release = useReleaseReservation();
  const releasing = useRef(false);
  const [editing, setEditing] = useState<ReservationDTO | "new" | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();
  const [feedback, setFeedback] = useState<string>();
  const items = (reservations.data ?? []).filter((r) => !source || key(r.sourceRef) === key(source.sourceRef));
  const active = items.filter((r) => !r.releasedAt);
  const released = items.filter((r) => r.releasedAt);
  const canAdd = source ? !source.excluded : sources.some((item) => !item.excluded);
  const findSource = (reservation: ReservationDTO) => sources.find((item) => key(item.sourceRef) === key(reservation.sourceRef)) ?? (source && key(source.sourceRef) === key(reservation.sourceRef) ? source : null);

  function startEditing(value: ReservationDTO | "new") {
    setError(undefined);
    setFeedback(undefined);
    setEditing(value);
  }

  async function releaseReservation(reservation: ReservationDTO) {
    if (releasing.current) return;
    releasing.current = true;
    setError(undefined);
    setFeedback(undefined);
    try {
      await release.mutateAsync({ id: reservation.id, expectedRevision: reservation.revision });
      setFeedback(t("availableFunds.reservationReleaseSuccess", { label: reservation.label }));
    } catch (cause) {
      setError(displayError(cause, t("availableFunds.loadError")));
    } finally {
      releasing.current = false;
    }
  }

  function reservationCard(reservation: ReservationDTO) {
    const itemSource = findSource(reservation);
    const account = accountNames[reservation.sourceRef.accountId];
    return <div key={reservation.id} className="space-y-3 rounded-lg border p-4">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <p className="min-w-0 break-words font-medium">{reservation.label}</p>
        <p className="font-mono text-sm tabular-nums">{formatAmount(reservation.amount.amount, reservation.amount.currency)} <span className="text-muted-foreground">{reservation.amount.currency}</span></p>
      </div>
      <p className="text-sm text-muted-foreground">{[account, itemSource ? sourceName(t, itemSource) : t("availableFunds.reservationSourceUnavailable")].filter(Boolean).join(" · ")}</p>
      {reservation.releasedAt ? <p className="text-xs text-muted-foreground">{t("availableFunds.reservationReleased")}</p> : <div className="flex flex-wrap gap-2">
        <Button variant="outline" size="sm" disabled={release.isPending} onClick={() => startEditing(reservation)}>{t("availableFunds.editReservation")}</Button>
        <Button variant="ghost" size="sm" disabled={release.isPending} onClick={() => { void releaseReservation(reservation); }}>{t("availableFunds.releaseReservation")}</Button>
      </div>}
    </div>;
  }

  return <Sheet open={open} onOpenChange={(next) => {
    if (saving || release.isPending) return;
    if (!next) { setEditing(null); setError(undefined); setFeedback(undefined); }
    onOpenChange(next);
  }}><SheetContent className="overflow-y-auto">
    <SheetHeader>
      <SheetTitle>{t(editing ? editing === "new" ? "availableFunds.addReservation" : "availableFunds.editReservation" : "availableFunds.reservations")}</SheetTitle>
      <SheetDescription>{t("availableFunds.reservationHelp")}</SheetDescription>
    </SheetHeader>
    {editing ? <ReservationForm
      key={editing === "new" ? "new" : `${editing.id}:${editing.revision}`}
      source={editing === "new" ? source : findSource(editing)}
      sources={sources}
      accountNames={accountNames}
      reservation={editing === "new" ? undefined : editing}
      onCancel={() => setEditing(null)}
      onSaved={() => { setEditing(null); setFeedback(t("availableFunds.reservationSaved")); }}
      onPendingChange={setSaving}
    /> : <>
      {feedback && <p role="status" className="rounded-md bg-muted p-3 text-sm">{feedback}</p>}
      <Button autoFocus disabled={!canAdd || release.isPending} onClick={() => startEditing("new")}>{t("availableFunds.addReservation")}</Button>
      {!canAdd && <p className="text-sm text-muted-foreground">{t("availableFunds.reservationNoSources")}</p>}
      {reservations.isLoading && <p>{t("ui.state.loadingPage")}</p>}
      {reservations.isError && <div className="space-y-2"><p role="alert" className="text-sm text-destructive">{t("availableFunds.loadError")}</p><Button variant="outline" disabled={reservations.isFetching} onClick={() => { void reservations.refetch(); }}>{t("common.retry")}</Button></div>}
      {!reservations.isLoading && !reservations.isError && active.length === 0 && <p className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">{t("availableFunds.reservationEmpty")}</p>}
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <div className="space-y-3">{active.map(reservationCard)}</div>
      {released.length > 0 && <details className="border-t pt-4">
        <summary className="cursor-pointer text-sm text-muted-foreground">{t("availableFunds.reservationHistory", { count: released.length })}</summary>
        <div className="mt-3 space-y-3">{released.map(reservationCard)}</div>
      </details>}
    </>}
  </SheetContent></Sheet>;
}
