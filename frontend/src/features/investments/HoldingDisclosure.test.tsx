import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { HoldingDisclosure } from "./HoldingDisclosure";
import { Button } from "@/components/ui/button";

it("shares aligned identity, canonical hit target, keyboard state and independent actions", async () => {
  const action = vi.fn();
  function Row() {
    const [expanded, setExpanded] = useState(false);
    return <><div className="flex items-center">
      <HoldingDisclosure name="Synthetic holding" symbol="SYN" expanded={expanded} controls="synthetic-detail" onToggle={() => setExpanded(value => !value)} />
      <Button onClick={action}>Row actions</Button>
    </div>{expanded && <div id="synthetic-detail">Details</div>}</>;
  }
  render(<Row />);
  const trigger = screen.getByRole("button", { name: "Show details for Synthetic holding" });
  expect(trigger).toHaveClass("size-9", "shrink-0");
  expect(trigger.parentElement).toHaveClass("flex", "items-center");
  expect(trigger.querySelector("svg")).toHaveAttribute("aria-hidden", "true");
  await userEvent.tab();
  expect(trigger).toHaveFocus();
  await userEvent.keyboard("{Enter}");
  expect(trigger).toHaveAttribute("aria-expanded", "true");
  expect(trigger).toHaveAccessibleName("Hide details for Synthetic holding");
  expect(document.getElementById(trigger.getAttribute("aria-controls")!)).toHaveTextContent("Details");
  expect(trigger).toHaveClass("bg-muted");
  await userEvent.tab();
  await userEvent.keyboard("{Enter}");
  expect(action).toHaveBeenCalledOnce();
  expect(trigger).toHaveAttribute("aria-expanded", "true");
  await userEvent.tab({ shift: true });
  await userEvent.keyboard(" ");
  expect(trigger).toHaveAttribute("aria-expanded", "false");
  expect(trigger).toHaveFocus();
  expect(screen.queryByText("Details")).not.toBeInTheDocument();
});
