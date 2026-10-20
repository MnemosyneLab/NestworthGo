import { expect, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AccountCompositionPanel } from "./AccountCompositionPanel";
import type { EChartProps } from "@/components/charts/EChart";
import fixture from "../../../../testdata/current/account-composition-shares.json";

const chart = vi.hoisted(() => ({ options: [] as unknown[] }));
vi.mock("@/components/charts/EChart", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/components/charts/EChart")>();
  const RealChart = actual.EChart;
  return { ...actual, EChart: (props: EChartProps) => {
    chart.options.push(props.option);
    return <RealChart {...props} />;
  } };
});

// Go asserts this shared synthetic wire fixture equals its actual projection.
// The two 178 amounts receive 1325 and 1324 bps. Exact half-even formatting
// renders both as 13.2%; independently allocating collapsed slices used to
// change item-01's displayed share.
it("keeps tied original shares identical across collapse, expansion, source table and tooltip", async () => {
  const user = userEvent.setup();
  render(<AccountCompositionPanel composition={fixture} />);
  await user.click(screen.getByRole("radio", { name: "By detail" }));
  const firstTie = "account-composition:holding:item-01";
  const secondTie = "account-composition:holding:item-02";
  const remainder = "account-composition:remainder";
  const tooltip = (key: string) => {
    const option = chart.options[chart.options.length - 1] as { tooltip: { formatter: (point: { name: string }) => string } };
    return option.tooltip.formatter({ name: key });
  };
  const assertOriginalShares = () => {
    expect(screen.getByTestId(`composition-slice-${firstTie}`)).toHaveTextContent("13.2%");
    expect(screen.getByTestId(`composition-slice-${secondTie}`)).toHaveTextContent("13.2%");
    expect(tooltip(firstTie)).toContain("US$178.00 (13.2%)");
    expect(tooltip(secondTie)).toContain("US$178.00 (13.2%)");
  };
  assertOriginalShares();
  expect(screen.getByTestId(`composition-slice-${remainder}`)).toHaveTextContent("US$40.00");
  expect(tooltip(remainder)).toContain("US$40.00 (3.0%)");
  await user.click(screen.getByText("View data table"));
  const table = screen.getByRole("table");
  expect(within(table).getAllByRole("row")).toHaveLength(13);
  expect(within(table).getByText("item-01").closest("tr")).toHaveTextContent("13.2%");
  expect(within(table).getByText("item-02").closest("tr")).toHaveTextContent("13.2%");
  expect(within(table).queryByText(/Remaining/)).not.toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "Show all 12 items" }));
  assertOriginalShares();
  expect(screen.queryByTestId(`composition-slice-${remainder}`)).not.toBeInTheDocument();
  expect(within(screen.getByRole("table")).getByText("item-01").closest("tr")).toHaveTextContent("13.2%");
  expect(tooltip(remainder)).toContain("US$40.00 (3.0%)");

  await user.click(screen.getByRole("button", { name: "Show top 10" }));
  assertOriginalShares();
  expect(screen.getByTestId(`composition-slice-${remainder}`)).toHaveTextContent("3.0%");
  expect(within(screen.getByRole("table")).getByText("item-02").closest("tr")).toHaveTextContent("13.2%");
});
