import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { DateRangeControl } from "./DateRangeControl";

function Control() {
  const [value, onChange] = useState({ from: "2026-09-21", to: "2026-09-27" });
  return <DateRangeControl value={value} onChange={onChange} min="2026-09-01" max="2026-09-27" />;
}
it("moves a week backward and forward and disables future periods", async () => {
  render(<Control />);
  expect(screen.getByRole("button", { name: "Next period" })).toBeDisabled();
  await userEvent.click(screen.getByRole("button", { name: "Previous period" }));
  expect(screen.getByText("2026-09-14 – 2026-09-20")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Last week" })).toHaveAttribute("aria-pressed", "true");
  await userEvent.click(screen.getByRole("button", { name: "Next period" }));
  expect(screen.getByText("2026-09-21 – 2026-09-27")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "1D" }));
  await userEvent.click(screen.getByRole("button", { name: "Previous period" }));
  expect(screen.getByText("2026-09-26")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "Next period" }));
  expect(screen.getByText("2026-09-27")).toBeInTheDocument();
});
