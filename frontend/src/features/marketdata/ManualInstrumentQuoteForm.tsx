import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ymdInTimeZone } from "@/features/insights/calendar";
import { displayError } from "@/lib/display";
import { metalPriceSuffix } from "@/lib/preciousMetals";
import { useHistoryOrigin } from "@/queries/history";
import { useAppendManualInstrumentQuote } from "@/queries/investments";

export function ManualInstrumentQuoteForm({
  instrumentId,
  currency,
  quantityUnit,
  onSaved, initialDate,
}: {
  instrumentId: string;
  currency: string;
  quantityUnit?: string;
  onSaved: () => void;
  initialDate?: string;
}) {
  const { t } = useTranslation();
  const origin = useHistoryOrigin();
  const today = ymdInTimeZone(new Date(), origin.data?.timezone ?? "UTC");
  const appendQuote = useAppendManualInstrumentQuote();
  const [unitPrice, setUnitPrice] = useState("");
  const [quotedAt, setQuotedAt] = useState(initialDate ?? "");

  const submit = () => {
    if (!unitPrice.trim()) {
      return;
    }
    appendQuote.mutate(
      { instrumentId, unitPrice: unitPrice.trim(), quotedAt: quotedAt },
      {
        onSuccess: () => {
          toast.success(t("portfolio.priceSaved"));
          setUnitPrice("");
          setQuotedAt(initialDate ?? "");
          onSaved();
        },
      },
    );
  };

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`quote-price-${instrumentId}`}>{t("portfolio.unitPrice")} ({currency}{metalPriceSuffix({quantityUnit}, t)})</Label>
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
        <DatePicker id={`quote-date-${instrumentId}`} max={today} value={quotedAt} onChange={setQuotedAt} />
        <p className="text-xs text-muted-foreground">{t("connections.manualDateZone")}</p>
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
