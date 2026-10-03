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

function show(width = 440, contentWidth = 900, columnWidth = 208) {
  const view = render(<ComparisonScroll><table><tbody><tr>
    <th data-account-column className="sticky left-0"><button>Account detail</button></th><td>Full comparison amounts</td>
  </tr></tbody></table></ComparisonScroll>);
  const region = screen.getByRole("region", { name: "Scrollable historical comparison" });
  const column = region.querySelector<HTMLElement>("[data-account-column]")!;
  // jsdom has no layout engine. Supply the browser's measured column width
  // and computed sticky/static state, including the compact CSS override.
  vi.spyOn(column, "getBoundingClientRect").mockReturnValue({ width: columnWidth } as DOMRect);
  const size = (nextWidth: number, nextContentWidth: number) => {
    column.style.position = nextWidth < 360 ? "static" : "sticky";
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
  expect(region.scrollLeft).toBe(174);
  expect(left).toBeEnabled();
  await userEvent.click(right);
  expect(region.scrollLeft).toBe(348);
  await userEvent.click(right);
  expect(region.scrollLeft).toBe(460);
  expect(right).toBeDisabled();
  await userEvent.click(left);
  await userEvent.click(left);
  await userEvent.click(left);
  expect(region.scrollLeft).toBe(0);
  expect(left).toBeDisabled();
});

it.each([
  { width: 440, columnWidth: 208 },
  { width: 440, columnWidth: 176 },
  { width: 260, columnWidth: 208 },
].flatMap(size => ["buttons", "arrows"].map(input => ({ ...size, input }))))(
  "continuously reveals content in both directions ($width viewport, $columnWidth column, $input)",
  async ({ width, columnWidth, input }) => {
    const contentWidth = 900;
    const { region } = show(width, contentWidth, columnWidth);
    const occluded = width < 360 ? 0 : columnWidth;
    const move = async (direction: "left" | "right") => {
      if (input === "buttons") await userEvent.click(screen.getByRole("button", { name: `Scroll ${direction}` }));
      else fireEvent.keyDown(region, { key: direction === "left" ? "ArrowLeft" : "ArrowRight" });
    };
    let previousEnd = width;
    let steps = 0;
    while (region.scrollLeft < contentWidth - width) {
      const previous = region.scrollLeft;
      await move("right");
      expect(region.scrollLeft).toBeGreaterThan(previous);
      // Each next unobscured range overlaps the previous range. The old
      // 75%-of-entire-viewport step skipped 98px with a 208px sticky column.
      expect(region.scrollLeft + occluded).toBeLessThan(previousEnd);
      previousEnd = region.scrollLeft + width;
      expect(++steps).toBeLessThan(20);
    }
    expect(previousEnd).toBe(contentWidth);
    let previousStart = region.scrollLeft + occluded;
    while (region.scrollLeft > 0) {
      const previous = region.scrollLeft;
      await move("left");
      expect(region.scrollLeft).toBeLessThan(previous);
      expect(region.scrollLeft + width).toBeGreaterThan(previousStart);
      previousStart = region.scrollLeft + occluded;
      expect(++steps).toBeLessThan(40);
    }
    expect(previousStart).toBe(occluded);
    expect(screen.getByRole("button", { name: "Scroll left" })).toBeDisabled();
  },
);

it("handles arrow keys on the scroll region without stealing keys from row controls", () => {
  const { region } = show();
  region.focus();
  fireEvent.keyDown(region, { key: "ArrowRight" });
  expect(region.scrollLeft).toBe(174);
  fireEvent.keyDown(screen.getByRole("button", { name: "Account detail" }), { key: "ArrowRight" });
  fireEvent.keyDown(region, { key: "ArrowRight", metaKey: true });
  expect(region.scrollLeft).toBe(174);
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
