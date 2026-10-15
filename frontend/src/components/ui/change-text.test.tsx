import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { ChangeText } from "./change-text";

it.each([
  ["up", "Up", "text-gain-positive", "lucide-arrow-up-right"],
  ["down", "Down", "text-gain-negative", "lucide-arrow-down-right"],
  ["flat", "Unchanged", "text-muted-foreground", "lucide-minus"],
] as const)("shows %s with an icon, spoken direction and tone", (direction, spoken, tone, icon) => {
  const { container } = render(<ChangeText direction={direction}>1.00%</ChangeText>);
  expect(screen.getByText("1.00%")).toBeInTheDocument();
  expect(screen.getByText(spoken)).toHaveClass("sr-only");
  expect(container.firstElementChild).toHaveClass(tone);
  expect(container.querySelector(`svg.${icon}`)).toHaveAttribute("aria-hidden", "true");
});
