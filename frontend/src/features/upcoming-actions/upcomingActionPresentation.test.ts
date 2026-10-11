import { describe, expect, it } from "vitest";
import i18n from "@/i18n";
import { upcomingItem } from "@/test/upcomingActions";
import { upcomingNavigation, upcomingTitle, supportedUpcomingAction } from "./upcomingActionPresentation";
describe("upcoming presentation", () => {
  it("uses receipt evidence and handles unknown kinds without a default action", () => {
    const item = upcomingItem();
    expect(upcomingTitle(i18n.t.bind(i18n), item)).toContain("2 days");
    expect(supportedUpcomingAction(upcomingItem({ kind: "future_kind" }))).toBe(false);
    expect(upcomingNavigation({ kind: "unknown", productId: "unsafe", sourceRef: null, reservationId: null })).toBeNull();
  });
  it("maps typed navigation without a financial command", () => {
    expect(upcomingNavigation(upcomingItem().primaryTarget)).toEqual({ page: "available-funds", focus: { kind: "product", productId: "product-1" } });
    expect(upcomingNavigation({ kind: "reservations", productId: null, sourceRef: null, reservationId: "missing-source" })).toEqual({ page: "available-funds", focus: { kind: "reservations", reservationId: "missing-source" } });
  });
});
