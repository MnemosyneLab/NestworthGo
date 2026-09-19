import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { AssetValueSummary } from "./AssetValueSummary";

it("shows backend period change without adding plus signs to balances", () => {
 render(<AssetValueSummary summary={{ beginningValue: { amount: "100", currency: "USD" }, endingValue: { amount: "125", currency: "USD" }, change: { amount: "25", currency: "USD" }, changeRate: "0.25" }} />);
 expect(screen.getByText("+25.00%")).toBeInTheDocument();
 expect(screen.getByText("$100.00")).toBeInTheDocument();
 expect(screen.getByText(/not an investment return/)).toBeInTheDocument();
});

it.each(["incomplete", "nonpositive_beginning"])("does not calculate a missing %s rate in the browser", (reason) => {
 render(<AssetValueSummary summary={{ beginningValue: { amount: "100", currency: "USD" }, endingValue: { amount: "125", currency: "USD" }, changeRate: null, changeRateMissingReason: reason }} />);
 expect(screen.queryByText("+25.00%")).not.toBeInTheDocument();
 expect(screen.getByText(/Unavailable because/)).toBeInTheDocument();
});
