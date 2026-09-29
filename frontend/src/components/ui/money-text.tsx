import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const moneyVariants = cva("num whitespace-nowrap tracking-tight", {
  variants: {
    size: {
      sm: "text-sm font-medium",
      md: "text-lg font-semibold",
      lg: "text-2xl font-bold",
      xl: "text-4xl font-extrabold",
    },
    tone: {
      neutral: "text-foreground",
      muted: "text-muted-foreground",
      positive: "text-gain-positive",
      negative: "text-gain-negative",
    },
  },
  defaultVariants: { size: "sm", tone: "neutral" },
});

export interface MoneyTextProps extends React.HTMLAttributes<HTMLSpanElement>, VariantProps<typeof moneyVariants> {}

/**
 * MoneyText renders an already-formatted money string with tabular figures
 * so columns of amounts line up. Formatting stays in lib/money.
 */
export function MoneyText({ className, size, tone, ...props }: MoneyTextProps) {
  return <span className={cn(moneyVariants({ size, tone }), className)} {...props} />;
}

/** Picks positive/negative tone from a canonical decimal string; zero and empty stay neutral. */
export function toneForAmount(amount: string | null | undefined): "neutral" | "positive" | "negative" {
  if (!amount) return "neutral";
  const trimmed = amount.trim();
  if (trimmed.startsWith("-")) return /[1-9]/.test(trimmed) ? "negative" : "neutral";
  return /[1-9]/.test(trimmed) ? "positive" : "neutral";
}

/** Gain/loss text color for a signed decimal (amount or rate); zero and empty stay default. */
export function signToneClass(value: string | null | undefined): string | undefined {
  const tone = toneForAmount(value);
  return tone === "positive" ? "text-gain-positive" : tone === "negative" ? "text-gain-negative" : undefined;
}
