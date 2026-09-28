import { useTranslation } from "react-i18next";
import { formatAmount } from "@/lib/money";
import { formatTimestamp } from "@/lib/time";
import type { ProductOperationPreviewDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
export function ProductPreview({ preview }: { preview: ProductOperationPreviewDTO }) {
  const { t } = useTranslation();
  const money = (value: {amount: string; currency: string} | null | undefined) => value ? formatAmount(value.amount, value.currency) : t("availableFunds.unknownAmount");
  return <div className="flex flex-col gap-2 rounded-md border border-border p-3 text-sm" data-testid="product-preview">
    <p>{t("availableFunds.actualTime")}: {formatTimestamp(preview.effectiveAt, preview.timezone)} · {preview.timezone}</p>
    <p>{t("availableFunds.previewCashBefore")}: {(preview.cashBefore ?? []).map(money).join(" · ") || t("availableFunds.unknownAmount")}</p>
    <p>{t("availableFunds.previewCashAfter")}: {(preview.cashAfter ?? []).map(money).join(" · ") || t("availableFunds.unknownAmount")}</p>
    <p>{t("availableFunds.previewProductBefore")}: {money(preview.productBefore)}</p>
    <p>{t("availableFunds.previewProductAfter")}: {money(preview.productAfter)}</p>
    <p>{t("availableFunds.netWorthChange")}: {preview.netWorthKnown ? money(preview.netWorthDelta) : t("availableFunds.unknownAmount")}</p>
    {(preview.reservationReleaseDetails ?? []).map((r) => <p key={r.id}>{t("availableFunds.releaseReservation")}: {r.label} · {money(r.amount)}</p>)}
    {(preview.activities?.length ?? 0) > 0 && <details className="mt-1 border-t border-border pt-3">
      <summary className="cursor-pointer font-medium">{t("availableFunds.ledgerDetails")}</summary>
      <div className="mt-3 flex flex-col gap-3 text-muted-foreground">
        {(preview.activities ?? []).map((a) => <div key={a.activity.id} className="flex flex-col gap-1">
          <p>{t(`history.kind.${a.activity.kind}`, { defaultValue: a.activity.kind })}</p>
          {a.activity.tradeDetail && <>
            <p>{t("history.detailGross")}: {money(a.activity.tradeDetail.gross)}</p>
            {a.activity.tradeDetail.fee && <p>{t("availableFunds.fee")}: {money(a.activity.tradeDetail.fee)}</p>}
          </>}
          {a.activity.dividendDetail && <p>{t("availableFunds.interestReceived")}: {money(a.activity.dividendDetail.amount)}</p>}
          {(a.effects ?? []).filter((e) => e.money).map((e) => <p key={e.id}>{t(e.direction === "added" ? "history.detailAmountAdded" : "history.detailAmountRemoved")} · {money(e.money)}</p>)}
        </div>)}
      </div>
    </details>}
    {[...(preview.warnings ?? []), ...(preview.assumptions ?? []), ...(preview.missingFields ?? [])].map((text,i) => <p key={`${i}:${text}`}>{t(`availableFunds.${text}`, { defaultValue: t("availableFunds.missingInputs") })}</p>)}
  </div>;
}
