import { expect, it } from "vitest";
import { isClosedDate, parseCivilDate, previousMonthEnd, shiftCivilDate } from "./dates";

it("rejects incomplete, normalized and out-of-range civil dates", () => {
  for (const value of ["", "2026-8-01", "2026-08-32", "2026-02-29", "2026-08-01 "]) {
    expect(parseCivilDate(value)).toBeNull();
    expect(isClosedDate(value, "2026-08-01", "2026-08-09")).toBe(false);
  }
  expect(isClosedDate("2026-08-01", "2026-08-01", "2026-08-09")).toBe(true);
  expect(isClosedDate("2026-08-09", "2026-08-01", "2026-08-09")).toBe(true);
  expect(isClosedDate("2026-08-10", "2026-08-01", "2026-08-09")).toBe(false);
  expect(isClosedDate("2026-07-31", "2026-08-01", "2026-08-09")).toBe(false);
});

it("steps civil labels across DST and leap-day boundaries without local midnight", () => {
  expect(shiftCivilDate("2026-09-05", 1)).toBe("2026-09-06");
  expect(shiftCivilDate("2026-09-07", -1)).toBe("2026-09-06");
  expect(shiftCivilDate("2024-03-01", -1)).toBe("2024-02-29");
  expect(shiftCivilDate("2026-12-31", 1)).toBe("2027-01-01");
  expect(shiftCivilDate("", 1)).toBe("");
});

it("chooses the previous civil month end including January and leap years", () => {
  expect(previousMonthEnd("2026-01-15")).toBe("2025-12-31");
  expect(previousMonthEnd("2024-03-15")).toBe("2024-02-29");
  expect(previousMonthEnd("2026-09-06")).toBe("2026-08-31");
});
