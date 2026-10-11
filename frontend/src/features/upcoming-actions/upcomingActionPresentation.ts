import type { TFunction } from "i18next";
import type { NavigationTarget } from "@/app/navigation";
import type { UpcomingActionDTO, UpcomingCountsDTO, UpcomingTargetDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

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
  if (item.phase === "receipt_due") return t((item.daysFromToday ?? 0) < 0 ? "upcomingActions.overdue" : "upcomingActions.dueToday", { count: Math.abs(item.daysFromToday ?? 0) });
  if (item.phase === "today" || item.phase === "upcoming") return t(item.anchorKind === "maturity" ? "upcomingActions.maturity" : "upcomingActions.eligibility", { date: item.anchorOn });
  return t(`upcomingActions.${item.phase}`, { date: item.estimatedReceiptOn });
}
export function upcomingButton(t: TFunction, target: UpcomingTargetDTO, phase?: string): string {
  if (target.kind === "reservations") return t("upcomingActions.checkReservations");
  if (phase === "receipt_due") return t("upcomingActions.verifyReceipt");
  return t("upcomingActions.viewTerms");
}


/** Display only: keep the precise DTO instant and its reported zone untouched. */
export function upcomingReadTime(asOf: string, timezone: string, language: string): string | null {
  const parts = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d{1,9})?(Z|[+-]\d{2}:\d{2})$/.exec(asOf);
  if (!parts || !timezone || timezone === "system") return null;
  const [, yearText, monthText, dayText, hourText, minuteText, secondText, offset] = parts;
  const year = Number(yearText), month = Number(monthText), day = Number(dayText);
  const leap = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const monthDays = [31, leap ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];
  if (month < 1 || month > 12 || day < 1 || day > monthDays[month - 1] || Number(hourText) > 23 || Number(minuteText) > 59 || Number(secondText) > 59) return null;
  if (offset !== "Z" && (Number(offset.slice(1, 3)) > 23 || Number(offset.slice(4, 6)) > 59)) return null;
  const instant = new Date(asOf);
  if (!Number.isFinite(instant.getTime())) return null;
  try {
    // Match the app's formatTimestamp convention without its browser-zone/ISO fallback.
    return new Intl.DateTimeFormat(language, { dateStyle: "medium", timeStyle: "short", timeZone: timezone }).format(instant);
  } catch { return null; }
}
export function upcomingReadLabel(t: TFunction, asOf: string, timezone: string, language: string): string {
  const formatted = upcomingReadTime(asOf, timezone, language);
  return formatted === null ? t("upcomingActions.asOfUnavailable") : t("upcomingActions.asOf", { asOf: formatted, timezone });
}
export function upcomingCountLabels(t: TFunction, counts: UpcomingCountsDTO) {
  return {
    ...counts,
    receipts: t("upcomingActions.receiptCount", { count: counts.receiptDue }),
    reservationIssuesText: t("upcomingActions.reservationIssueCount", { count: counts.reservationIssues }),
    schedules: t("upcomingActions.scheduleCount", { count: counts.scheduled }),
    dataIssues: t("upcomingActions.dataIssueCount", { count: counts.dataChecks }),
    notes: t("upcomingActions.noteCount", { count: counts.scopeNotes }),
  };
}
