import { expect, it } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { ReviewEvidenceTree } from "./ReviewEvidenceTree";
import { ReviewPriority } from "./ReviewPriority";
import type { ReviewDailyResult } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";

it("distinguishes repeated metrics and their individual votes in a mixed AND/OR rule", () => {
  const result = {
    on: "2026-10-12", truth: "true",
    predicate: { all: [{ any: [
      { leaf: { metric: "price_change", operator: "<", value: "-5", lookbackDays: 7 } },
      { leaf: { metric: "price_change", operator: ">", value: "10", lookbackDays: 30 } },
    ] }, { leaf: { metric: "cost", operator: ">", value: "100", currency: "USD" } }] },
    leaves: [
      { metric: "price_change", truth: "false", actual: "-2", threshold: "-5" },
      { metric: "price_change", truth: "true", actual: "12", threshold: "10" },
      { metric: "cost", truth: "unknown", threshold: "100", currency: "USD", reason: "historical_amount_unknown" },
    ],
  } as ReviewDailyResult;
  render(<ReviewEvidenceTree result={result} />);
  expect(screen.getByText(/7 days/)).toBeInTheDocument();
  expect(screen.getByText(/30 days/)).toBeInTheDocument();
  const seven = screen.getByText(/7 days/).closest("li")!;
  expect(within(seven).getByText("Not matched")).toBeInTheDocument();
  expect(seven).toHaveTextContent("< -5% · Observed: -2.00%");
  const thirty = screen.getByText(/30 days/).closest("li")!;
  expect(within(thirty).getByText("Matched")).toBeInTheDocument();
  expect(thirty).toHaveTextContent("> 10% · Observed: 12.00%");
  expect(screen.getAllByText("Unknown").length).toBeGreaterThan(0);
});

it.each([["warning", "P1 · High"], ["notice", "P2 · Normal"], ["p0", "P0 · Highest"]])("renders priority %s explicitly", (severity, label) => {
  render(<ReviewPriority severity={severity} />);
  expect(screen.getByText(label)).toBeInTheDocument();
});
