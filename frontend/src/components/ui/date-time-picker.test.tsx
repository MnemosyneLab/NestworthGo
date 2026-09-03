import { expect, describe, it, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { settingsQueryKey } from "@/queries/settings";
import { DatePicker } from "./date-picker";
import { TimePicker } from "./time-picker";

function renderDatePicker(settings: { weekStart: string; dateFormat: string }, onChange = vi.fn()) {
  const queryClient = createTestQueryClient();
  queryClient.setQueryData(settingsQueryKey, settings);
  return {
    onChange,
    ...render(
      <QueryClientProvider client={queryClient}>
        <DatePicker id="test-date" value="2026-08-15" min="2026-08-10" max="2026-08-20" onChange={onChange} />
      </QueryClientProvider>,
    ),
  };
}

describe("DatePicker and TimePicker", () => {
  it("uses weekStart and dateFormat settings and disables dates outside min/max", async () => {
    renderDatePicker({ weekStart: "sunday", dateFormat: "day-first" });

    expect(screen.getByRole("button", { name: "15/08/2026" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "15/08/2026" }));
    const calendar = await screen.findByRole("grid");
    const weekdayLabels = Array.from(calendar.querySelectorAll("thead th")).map((cell) => cell.getAttribute("aria-label"));
    expect(weekdayLabels[0]).toBe("Sunday");

    const dayButton = (date: string) => calendar.querySelector(`[data-day="${date}"] button`);
    expect(dayButton("2026-08-09")).toBeDisabled();
    expect(dayButton("2026-08-10")).not.toBeDisabled();
    expect(dayButton("2026-08-20")).not.toBeDisabled();
    expect(dayButton("2026-08-21")).toBeDisabled();
  });

  it("shows one month caption and one year caption in the dropdown header", async () => {
    renderDatePicker({ weekStart: "sunday", dateFormat: "day-first" });

    await userEvent.click(screen.getByRole("button", { name: "15/08/2026" }));
    await screen.findByRole("grid");

    const month = screen.getByRole("combobox", { name: /month/i });
    const year = screen.getByRole("combobox", { name: /year/i });
    expect(month).toHaveClass("opacity-0");
    expect(year).toHaveClass("opacity-0");

    const captions = [...document.querySelectorAll('[aria-hidden="true"]')].map((el) => el.textContent?.replace(/\s+/g, " ").trim() ?? "");
    expect(captions.filter((text) => /^Aug/.test(text))).toHaveLength(1);
    expect(captions.filter((text) => text === "2026" || /^2026 /.test(text))).toHaveLength(1);
  });

  it("offers a 00-23 hour picker without a native time input", async () => {
    const onChange = vi.fn();
    const { container } = render(
      <TimePicker id="test-time" value="" onChange={onChange} />,
    );

    await userEvent.click(container.querySelector("#test-time") as HTMLElement);
    const lists = await screen.findAllByRole("list", { name: "Local time" });
    const hours = within(lists[0]).getAllByRole("button").map((button) => button.textContent);
    expect(hours).toEqual(Array.from({ length: 24 }, (_, index) => String(index).padStart(2, "0")));
    expect(within(lists[0]).queryByRole("button", { name: "24" })).not.toBeInTheDocument();
    expect(container.querySelector('input[type="time"]')).not.toBeInTheDocument();

    await userEvent.click(within(lists[0]).getByRole("button", { name: "23" }));
    expect(onChange).toHaveBeenCalledWith("23:00");
  });
});
