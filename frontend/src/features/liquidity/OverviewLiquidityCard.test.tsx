import { render, screen } from "@testing-library/react";
import { it, expect, vi, beforeEach } from "vitest";
import { OverviewLiquidityCard } from "./OverviewLiquidityCard";

const state = vi.hoisted(() => ({ bucket: {} as { status: string; fullUnreserved: { amount: string; currency: string } | null; knownUnreservedSubtotal: { amount: string; currency: string } | null } }));
vi.mock("@/queries/liquidity", () => ({ useLiquidityOverview: () => ({ data: { buckets: [state.bucket, {}, state.bucket] } }) }));
beforeEach(() => {
  state.bucket = { status: "partial", fullUnreserved: null, knownUnreservedSubtotal: { amount: "6648.76", currency: "CNY" } };
});
it("emphasizes known unreserved amounts and qualifies incomplete totals", () => {
  render(<OverviewLiquidityCard />);
  expect(screen.getByTestId("overview-liquidity-today")).toHaveTextContent("CN¥6,648.76");
  expect(screen.getByTestId("overview-liquidity-30")).toHaveTextContent("CN¥6,648.76");
  expect(screen.getAllByText(/Known amounts only;/)).toHaveLength(2);
});
it("does not invent zero when there is no known amount", () => {
  state.bucket.knownUnreservedSubtotal = null;
  render(<OverviewLiquidityCard />);
  expect(screen.getByTestId("overview-liquidity-today")).toHaveTextContent("Unknown");
});
it("prefers a complete total, including zero, without a partial qualifier", () => {
  state.bucket.status = "complete";
  state.bucket.fullUnreserved = { amount: "0", currency: "CNY" };
  render(<OverviewLiquidityCard />);
  expect(screen.getByTestId("overview-liquidity-today")).toHaveTextContent("CN¥0.00");
  expect(screen.queryByText(/Known amounts only;/)).not.toBeInTheDocument();
});
