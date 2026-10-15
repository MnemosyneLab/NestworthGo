import type { ReactNode } from "react";
import { ArrowDownRight, ArrowUpRight, Minus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";

export type ChangeDirection = "up" | "down" | "flat";

const TONE: Record<ChangeDirection, string> = {
  up: "text-gain-positive",
  down: "text-gain-negative",
  flat: "text-muted-foreground",
};
const ICON = { up: ArrowUpRight, down: ArrowDownRight, flat: Minus } as const;

/**
 * Shared gain/loss text. Direction is never color-only: a visible arrow and the
 * signed text carry it, and screen readers get the spoken direction too.
 */
export function ChangeText({ direction, children, className, iconClassName }: {
  direction: ChangeDirection;
  children: ReactNode;
  className?: string;
  iconClassName?: string;
}) {
  const { t } = useTranslation();
  const Icon = ICON[direction];
  return <span className={cn("inline-flex items-center gap-1", TONE[direction], className)}>
    <Icon className={cn("size-[1em] shrink-0", iconClassName)} aria-hidden="true" />
    <span className="sr-only">{t(`holdingPerformance.direction.${direction}`)}</span>
    <span>{children}</span>
  </span>;
}
