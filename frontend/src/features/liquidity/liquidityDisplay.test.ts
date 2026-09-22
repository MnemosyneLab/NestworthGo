import { describe, it, expect } from "vitest";
import { cashVersusProceeds } from "./liquidityDisplay";
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
