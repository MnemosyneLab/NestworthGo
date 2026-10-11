import type { TFunction } from "i18next";
import type { NavigationTarget } from "@/app/navigation";
import type { UpcomingActionDTO, UpcomingTargetDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

const phases = ["receipt_due", "blocked", "today", "upcoming", "awaiting_receipt", "scheduled_receipt", "eligible", "current_issue"];
const kinds = ["product_schedule", "source_access", "reservation_shortfall", "reservation_unresolved"];
export function supportedUpcomingAction(item: UpcomingActionDTO) {
  return kinds.includes(item.kind) && phases.includes(item.phase) && ["attention", "scheduled", "eligible"].includes(item.group);
}
export function upcomingNavigation(target: UpcomingTargetDTO): NavigationTarget | null {
  if (target.kind === "product" && target.productId) return { page: "available-funds", focus: { kind: "product", productId: target.productId } };
  if (target.kind === "source_policy" && target.sourceRef) return { page: "available-funds", focus: { kind: "policy", sourceRef: target.sourceRef } };
  if (target.kind === "reservations") return { page: "available-funds", focus: { kind: "reservations", ...(target.sourceRef ? { sourceRef: target.sourceRef } : {}), ...(target.reservationId ? { reservationId: target.reservationId } : {}) } };
  if (target.kind === "data_health") return { page: "data-health" };
  return null;
}
export function upcomingTitle(t: TFunction, item: UpcomingActionDTO): string {
  if (!supportedUpcomingAction(item)) return t("upcomingActions.unsupported");
  if (item.kind === "reservation_shortfall" || item.kind === "reservation_unresolved") return t(`upcomingActions.${item.kind}`);
  if (item.phase === "receipt_due") return t((item.daysFromToday ?? 0) < 0 ? "upcomingActions.overdue" : "upcomingActions.dueToday", { days: Math.abs(item.daysFromToday ?? 0) });
  if (item.phase === "today" || item.phase === "upcoming") return t(item.anchorKind === "maturity" ? "upcomingActions.maturity" : "upcomingActions.eligibility", { date: item.anchorOn });
  return t(`upcomingActions.${item.phase}`, { date: item.estimatedReceiptOn });
}
export function upcomingButton(t: TFunction, target: UpcomingTargetDTO, phase?: string): string {
  if (target.kind === "reservations") return t("upcomingActions.checkReservations");
  if (phase === "receipt_due") return t("upcomingActions.verifyReceipt");
  return t("upcomingActions.viewTerms");
}
