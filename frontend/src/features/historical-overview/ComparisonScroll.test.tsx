import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ComparisonScroll } from "./ComparisonScroll";
import i18n from "@/i18n";

let resize: () => void;
const disconnect = vi.fn();
beforeEach(() => {
  void i18n.changeLanguage("en");
  vi.stubGlobal("ResizeObserver", class {
    constructor(callback: () => void) { resize = callback; }
    observe() {}
    disconnect = disconnect;
  });
});
afterEach(() => vi.unstubAllGlobals());

function show(width = 440, contentWidth = 900) {
  const view = render(<ComparisonScroll><table><tbody><tr>
    <th data-account-column className="sticky left-0"><button>Account detail</button></th><td>Full comparison amounts</td>
  </tr></tbody></table></ComparisonScroll>);
  const region = screen.getByRole("region", { name: "Scrollable historical comparison" });
  const size = (nextWidth: number, nextContentWidth: number) => {
    Object.defineProperties(region, { clientWidth: { value: nextWidth, configurable: true }, scrollWidth: { value: nextContentWidth, configurable: true } });
    act(() => resize());
  };
  size(width, contentWidth);
  return { ...view, region, size };
}

it("provides explicit horizontal controls and reaches both edges without a trackpad gesture", async () => {
  const { region } = show();
  const left = screen.getByRole("button", { name: "Scroll left" });
  const right = screen.getByRole("button", { name: "Scroll right" });
  expect(left).toBeDisabled();
  expect(right).toHaveAttribute("aria-controls", region.id);
  await userEvent.click(right);
  expect(region.scrollLeft).toBe(330);
  expect(left).toBeEnabled();
  await userEvent.click(right);
  expect(region.scrollLeft).toBe(460);
  expect(right).toBeDisabled();
  await userEvent.click(left);
  await userEvent.click(left);
  expect(region.scrollLeft).toBe(0);
  expect(left).toBeDisabled();
});

it("handles arrow keys on the scroll region without stealing keys from row controls", () => {
  const { region } = show();
  region.focus();
  fireEvent.keyDown(region, { key: "ArrowRight" });
  expect(region.scrollLeft).toBe(330);
  fireEvent.keyDown(screen.getByRole("button", { name: "Account detail" }), { key: "ArrowRight" });
  fireEvent.keyDown(region, { key: "ArrowRight", metaKey: true });
  expect(region.scrollLeft).toBe(330);
  fireEvent.keyDown(region, { key: "ArrowLeft" });
  expect(region.scrollLeft).toBe(0);
});

it("rechecks overflow on resize and releases sticky names when they would obscure narrow content", () => {
  const { region, size, unmount } = show();
  expect(region).not.toHaveClass("[&_[data-account-column]]:static");
  size(260, 900);
  expect(region).toHaveClass("[&_[data-account-column]]:static");
  expect(screen.getByRole("button", { name: "Scroll right" })).toBeEnabled();
  size(1000, 900);
  expect(screen.queryByRole("button", { name: "Scroll right" })).not.toBeInTheDocument();
  expect(region).not.toHaveClass("[&_[data-account-column]]:static");
  unmount();
  expect(disconnect).toHaveBeenCalled();
});
