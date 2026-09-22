import { describe, it, expect } from "vitest";
import { cashVersusProceeds, sourceStateLabel, sourceMatchesFilter } from "./liquidityDisplay";
import type { LiquiditySourceDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

const horizon = "2026-09-20";
function cash(status: string, amount: string | null): LiquiditySourceDTO {
  const money = amount === null ? null : { amount, currency: "USD" };
  return { excluded: false, sourceRef: { kind: "account_cash" }, normalRoute: { actionRequired: "withdraw" }, bucketResults: [{ horizonOn: horizon, selectedRoute: money ? { actionRequired: "withdraw" } : null, status, netNative: money, netBase: money }] } as LiquiditySourceDTO;
}
describe("cash breakdown completeness", () => {
  it("does not show unknown cash as zero", () => {
    expect(cashVersusProceeds([cash("partial", null)], horizon, "Unknown", "USD").cash).toBe("Unknown");
  });
  it("does not present the known portion as a full total", () => {
    expect(cashVersusProceeds([cash("complete", "100"), cash("partial", null)], horizon, "Unknown", "USD").cash).toBe("Unknown");
  });
  it("keeps proven unavailable cash at zero", () => {
    expect(cashVersusProceeds([cash("complete", null)], horizon, "Unknown", "USD").cash).toBe("$0.00");
  });
});

it("keeps unknown availability separate from a confirmed lock", () => {
  const source = { ...cash("partial", null), displayState: "needs_info", reasons: ["unknown access"] } as LiquiditySourceDTO;
  expect(sourceStateLabel((key) => key, source)).toBe("availableFunds.filterNeedsInfo");
  expect(sourceMatchesFilter(source, horizon, "needs_info")).toBe(true);
  expect(sourceMatchesFilter(source, horizon, "locked")).toBe(false);
});

it("keeps native cash grouped by currency without converted values", () => {
  const eur = cash("complete", "120");
  eur.nativeCurrency = "EUR";
  eur.bucketResults![0].netNative = { amount: "100", currency: "EUR" };
  const usd = cash("complete", "50");
  usd.nativeCurrency = "USD";
  expect(cashVersusProceeds([eur, usd], horizon, "Unknown", "USD", "native").cash).toBe("€100.00 · $50.00");
  expect(cashVersusProceeds([eur, usd], horizon, "Unknown", "USD").cash).toBe("$170.00");
  expect(cashVersusProceeds([eur], horizon, "Unknown", "USD", "native").proceeds).toBe("€0.00");
});
it("never substitutes native money for missing base FX", () => {
  const eur = cash("partial", "100");
  eur.bucketResults![0].netBase = null;
  expect(cashVersusProceeds([eur], horizon, "Unknown", "USD").cash).toBe("Unknown");
});

it("does not classify an unknown holding's placeholder action as cash", () => {
  const holding = cash("unavailable", null);
  holding.sourceRef.kind = "holding";
  holding.normalRoute!.actionRequired = "none";
  expect(cashVersusProceeds([cash("complete", "100"), holding], horizon, "Unknown", "USD")).toEqual({cash:"$100.00", proceeds:"Unknown"});
});
