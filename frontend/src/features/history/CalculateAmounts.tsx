import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { currencyFractionDigits, divideCanonical, isPositiveCanonical, multiplyCanonical } from "@/lib/money";
import type { FXQuoteDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

export function isPositiveMoney(value: string): boolean {
  return isPositiveCanonical(value.trim());
}

export type FilledSide = "a" | "b" | "none" | "both-empty";

/** Returns which side Calculate should write, or why it should not. */
export function computeFilledSide(amountA: string, amountB: string): FilledSide {
  const aPositive = isPositiveMoney(amountA);
  const bPositive = isPositiveMoney(amountB);
  if (aPositive && bPositive) {
    return "none";
  }
  if (!aPositive && !bPositive) {
    return "both-empty";
  }
  return aPositive ? "b" : "a";
}

export function convertWithQuote(quote: FXQuoteDTO, amount: string, from: string, to: string): string {
  const places = currencyFractionDigits(to);
  if (quote.baseCurrency === from && quote.quoteCurrency === to) {
    return multiplyCanonical(amount, quote.rate, places);
  }
  if (quote.baseCurrency === to && quote.quoteCurrency === from) {
    return divideCanonical(amount, quote.rate, places);
  }
  return "";
}

export function tradeFillFromPrice(
  quantity: string,
  gross: string,
  unitPrice: string,
  grossCurrency: string,
  quantityPlaces = 8,
): { quantity?: string; gross?: string } | undefined {
  const side = computeFilledSide(quantity, gross);
  if (side === "none" || side === "both-empty") {
    return undefined;
  }
  if (side === "b") {
    return { gross: multiplyCanonical(quantity.trim(), unitPrice, currencyFractionDigits(grossCurrency)) };
  }
  return { quantity: divideCanonical(gross.trim(), unitPrice, quantityPlaces) };
}

export function CalculateAmounts({
  quoteHint,
  onCalculate,
  disabled = false,
}: {
  quoteHint: ReactNode;
  onCalculate: () => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-start justify-between gap-2">
      <div className="min-w-0 flex-1">{quoteHint}</div>
      <Button type="button" variant="outline" size="sm" onClick={onCalculate} disabled={disabled}>
        {t("history.calculate")}
      </Button>
    </div>
  );
}

export function notifyCalculateBlocked(t: (key: string) => string, kind: "both-empty" | "missing-quote") {
  toast.message(t(kind === "both-empty" ? "history.calculateFillOneSide" : "history.calculateMissingQuote"));
}
