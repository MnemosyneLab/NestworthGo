import { beforeEach, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AccountCompositionPanel } from "./AccountCompositionPanel";
import { CompositionChart } from "@/components/charts/CompositionChart";
import fixture from "../../../../testdata/current/account-composition-shares.json";

// Exercise the real EChart wrapper and its dispatched canvas actions without
// asking jsdom to draw. The default-consumer test guards the opt-in boundary.
const canvas = vi.hoisted(() => ({ setOption: vi.fn(), dispatchAction: vi.fn(), on: vi.fn(), off: vi.fn(), dispose: vi.fn(), resize: vi.fn() }));
vi.mock("echarts/core", async (importOriginal) => {
  const actual = await importOriginal<typeof import("echarts/core")>();
  return { ...actual, init: () => canvas };
});
beforeEach(() => vi.clearAllMocks());

function pieRows(): Array<{ name: string; selected?: boolean }> {
  return canvas.setOption.mock.lastCall?.[0].series[0].data ?? [];
}

it("routes tail interactions to the remainder while preserving original identity and clearing collapse state", async () => {
  const user = userEvent.setup();
  render(<AccountCompositionPanel composition={fixture} />);
  await user.click(screen.getByRole("radio", { name: "By detail" }));
  await user.click(screen.getByRole("button", { name: "Show all 12 items" }));
  const firstKey = "account-composition:holding:item-10";
  const secondKey = "account-composition:holding:item-11";
  const remainder = "account-composition:remainder";
  const first = screen.getByTestId(`composition-slice-${firstKey}`);
  const second = screen.getByTestId(`composition-slice-${secondKey}`);
  expect(first).toHaveTextContent("item-10");
  expect(first).toHaveTextContent("$28.00");
  expect(second).toHaveTextContent("item-11");
  expect(second).toHaveTextContent("$12.00");
  await user.hover(first);
  expect(canvas.dispatchAction).toHaveBeenLastCalledWith({ type: "highlight", seriesIndex: 0, name: remainder });
  expect(first).toHaveAttribute("aria-pressed", "false");
  await user.click(first);
  expect(first).toHaveAttribute("aria-pressed", "true");
  expect(second).toHaveAttribute("aria-pressed", "false");
  expect(pieRows().find((row) => row.name === remainder)?.selected).toBe(true);
  expect(pieRows().some((row) => row.name === firstKey)).toBe(false);
  await user.click(second);
  expect(second).toHaveAttribute("aria-pressed", "true");
  expect(first).toHaveAttribute("aria-pressed", "false");
  expect(pieRows().find((row) => row.name === remainder)?.selected).toBe(true);

  await user.click(screen.getByRole("button", { name: "Show top 10" }));
  expect(screen.queryByTestId(`composition-slice-${secondKey}`)).not.toBeInTheDocument();
  expect(screen.getByTestId(`composition-slice-${remainder}`)).toHaveAttribute("aria-pressed", "false");
  expect(pieRows().every((row) => row.selected === false)).toBe(true);
  expect(canvas.dispatchAction).toHaveBeenLastCalledWith({ type: "downplay", seriesIndex: 0 });
  await user.click(screen.getByRole("button", { name: "Show all 12 items" }));
  expect(screen.getByTestId(`composition-slice-${firstKey}`)).toHaveAttribute("aria-pressed", "false");
  expect(screen.getByTestId(`composition-slice-${secondKey}`)).toHaveAttribute("aria-pressed", "false");
  expect(pieRows().every((row) => row.selected === false)).toBe(true);
});

it("leaves other composition chart consumers' selection and hover behavior unchanged", async () => {
  const user = userEvent.setup();
  render(<CompositionChart title="Allocation" currency="USD" centerValue="2" centerCaption="Subtotal" ariaLabel="Allocation" summary="Allocation" items={[
    { key: "a", label: "A", amount: "1", shareBps: 5000 },
    { key: "b", label: "B", amount: "1", shareBps: 5000 },
  ]} />);
  const first = screen.getByTestId("composition-slice-a");
  await user.hover(first);
  expect(first).toHaveAttribute("aria-pressed", "true");
  await user.click(first);
  expect(first).toHaveAttribute("aria-pressed", "true");
  expect(canvas.dispatchAction).not.toHaveBeenCalled();
  expect(pieRows().every((row) => row.selected === undefined)).toBe(true);
  expect(canvas.setOption).toHaveBeenCalledTimes(1);
});
