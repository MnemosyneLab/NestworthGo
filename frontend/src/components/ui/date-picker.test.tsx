import { fireEvent, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import i18n from "@/i18n";
import { createTestQueryClient } from "@/test/queryClient";
import { settingsQueryKey } from "@/queries/settings";
import { DatePicker } from "./date-picker";

afterEach(async () => {
  vi.useRealTimers();
  await i18n.changeLanguage("en");
});

function renderPicker(props: Partial<ComponentProps<typeof DatePicker>> = {}) {
  const client = createTestQueryClient();
  client.setQueryData(settingsQueryKey, { weekStart: "monday", dateFormat: "iso" });
  const onChange = vi.fn();
  render(
    <QueryClientProvider client={client}>
      <DatePicker value="2026-10-10" onChange={onChange} aria-label="Date" {...props} />
    </QueryClientProvider>,
  );
  return { onChange };
}

it("disables future days in the current month while keeping today selectable", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-17T12:00:00Z"));
  renderPicker();

  fireEvent.click(screen.getByRole("button", { name: "Date" }));
  const grid = screen.getByRole("grid");
  expect(grid.querySelector('[data-day="2026-10-17"] button')).toBeEnabled();
  expect(grid.querySelector('[data-day="2026-10-18"] button')).toBeDisabled();
  expect(grid.querySelector('[data-day="2026-10-31"] button')).toBeDisabled();
});

it("honors the supplied household date across a browser timezone and year edge", async () => {
  vi.useFakeTimers();
  // The browser clock is December 31 in UTC, while this test supplies
  // January 1 as the household's Kiritimati-timezone maximum.
  vi.setSystemTime(new Date("2025-12-31T12:00:00Z"));
  renderPicker({ value: "2025-12-31", max: "2026-01-01" });

  fireEvent.click(screen.getByRole("button", { name: "Date" }));
  let grid = screen.getByRole("grid");
  expect(grid.querySelector('[data-day="2025-12-31"] button')).toBeEnabled();
  fireEvent.click(screen.getByRole("button", { name: "Go to the Next Month" }));
  grid = screen.getByRole("grid");
  expect(grid.querySelector('[data-day="2026-01-01"] button')).toBeEnabled();
  expect(grid.querySelector('[data-day="2026-01-02"] button')).toBeDisabled();
});

it.each(["zh-CN", "zh-TW"])("opens the calendar with the %s date-fns locale", async (language) => {
  await i18n.changeLanguage(language);
  renderPicker();
  fireEvent.click(screen.getByRole("button", { name: "Date" }));
  expect(screen.getByRole("grid")).toBeInTheDocument();
});

it("keeps future scheduling dates selectable when explicitly allowed", async () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-10-17T12:00:00Z"));
  renderPicker({ allowFuture: true });

  fireEvent.click(screen.getByRole("button", { name: "Date" }));
  expect(screen.getByRole("grid").querySelector('[data-day="2026-10-18"] button')).toBeEnabled();
});
