import { expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import i18n from "@/i18n";
import { QuoteHistoryContent } from "./QuoteHistorySheet";
import type { InstrumentDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

const query = vi.hoisted(() => ({ setRange: vi.fn() }));
vi.mock("@/components/charts/useTrendDateRange", () => ({ useTrendDateRange: () => ({
  value: { from: "2026-09-17", to: "2026-10-16" }, min: "1970-01-01", max: "2026-10-16",
  queryRange: "2026-09-17:2026-10-16", onChange: query.setRange,
}) }));
vi.mock("@/queries/investments", () => ({
  useInstrumentQuoteSeries: () => ({ isLoading: false, isError: false, data: { observations: [], outsideRange: true } }),
  useFXQuoteSeries: () => ({ isLoading: false, isError: false, data: { observations: [], outsideRange: true } }),
}));
vi.mock("@/queries/settings", () => ({ useSettings: () => ({ data: { timezone: "UTC" } }) }));

it("publishes date shortcut and show-all changes to its owner", () => {
  const onRangeChange = vi.fn();
  render(<QuoteHistoryContent target={{ kind: "instrument", instrument: {
    id: "synthetic-fund", name: "Synthetic fund", quoteCurrency: "USD", type: "fund",
  } as InstrumentDTO }} onRangeChange={onRangeChange} />);
  fireEvent.click(screen.getByRole("button", { name: i18n.t("rangeShortcuts.3m") }));
  expect(query.setRange).toHaveBeenCalledWith({ from: "2026-07-17", to: "2026-10-16" });
  expect(onRangeChange).toHaveBeenCalledWith({ from: "2026-07-17", to: "2026-10-16" });
  fireEvent.click(screen.getByRole("button", { name: i18n.t("charts.showAllRange") }));
  expect(onRangeChange).toHaveBeenCalledWith({ from: "1970-01-01", to: "2026-10-16" });
});
