import { beforeEach, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/i18n";
import { Counts, Notice, Pager, outcomeVariant, sessionStatusVariant } from "./ReconciliationParts";

beforeEach(async () => {
  await i18n.changeLanguage("en");
});

it("announces counts as text and keeps the progress strip decorative", () => {
  const { container } = render(<Counts counts={{ confirmed: 2, pending: 1, skipped: 1, unresolved: 0 }} />);
  expect(screen.getByRole("status")).toHaveTextContent("2 confirmed · 1 pending · 1 skipped · 0 unresolved");
  const strip = container.querySelector('[aria-hidden="true"]');
  expect(strip?.children).toHaveLength(2);
  expect((strip?.children[0] as HTMLElement).style.width).toBe("50%");
});

it("omits the progress strip when there is nothing to count", () => {
  const { container } = render(<Counts counts={{ confirmed: 0, pending: 0, skipped: 0, unresolved: 0 }} />);
  expect(container.querySelector('[aria-hidden="true"]')).toBeNull();
});

it("exposes the requested live-region role on notices", () => {
  render(<Notice tone="warning" role="alert">Check this</Notice>);
  expect(screen.getByRole("alert")).toHaveTextContent("Check this");
});

it("shows pagination only when more than one page exists and pages through it", async () => {
  const onPage = vi.fn();
  const { rerender } = render(<Pager page={0} total={25} onPage={onPage} />);
  expect(screen.queryByRole("button")).not.toBeInTheDocument();
  rerender(<Pager page={0} total={26} onPage={onPage} />);
  expect(screen.getByRole("button", { name: "Previous" })).toBeDisabled();
  await userEvent.click(screen.getByRole("button", { name: "Next" }));
  expect(onPage).toHaveBeenCalledWith(1);
  rerender(<Pager page={1} total={26} onPage={onPage} />);
  expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
});

it("maps statuses and outcomes to distinct badge variants", () => {
  expect(sessionStatusVariant("completed")).toBe("success");
  expect(sessionStatusVariant("paused")).toBe("warning");
  expect(sessionStatusVariant("in_progress")).toBe("default");
  expect(outcomeVariant("confirmed")).toBe("success");
  expect(outcomeVariant("unresolved")).toBe("warning");
  expect(outcomeVariant("pending")).toBe("outline");
  expect(outcomeVariant("skipped")).toBe("secondary");
});
