import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from "./sheet";

afterEach(() => vi.unstubAllGlobals());

function viewport(width = 640, height = 360, offsetLeft = 0, offsetTop = 0) {
  const value = Object.assign(new EventTarget(), { width, height, offsetLeft, offsetTop, scale: 2 });
  vi.stubGlobal("visualViewport", value);
  return value;
}

function Example({ side = "right", closeDisabled = false }: { side?: "left" | "right"; closeDisabled?: boolean }) {
  return <Sheet>
    <SheetTrigger>Open details</SheetTrigger>
    <SheetContent side={side} closeDisabled={closeDisabled}>
      <SheetHeader><SheetTitle>Account details</SheetTitle><SheetDescription>Read only</SheetDescription></SheetHeader>
      <p>Long account history</p>
    </SheetContent>
  </Sheet>;
}

// jsdom does not lay out pixels. These tests cover viewport event wiring,
// positioning constraints and dialog interaction; native zoom QA checks fit.
it.each(["left", "right"] as const)("anchors a %s sheet to the magnified, panned visible bounds", async side => {
  viewport(640, 360, 180, 24);
  render(<Example side={side} />);
  await userEvent.click(screen.getByRole("button", { name: "Open details" }));
  const panel = await screen.findByRole("dialog");
  expect(panel.parentElement).toHaveStyle({ left: "180px", top: "24px", width: "640px", height: "360px" });
  expect(panel.parentElement).toHaveClass("fixed", "overflow-hidden", "pointer-events-none");
  expect(panel).toHaveClass("absolute", `${side}-0`, "h-full", "w-full", "max-w-md", "overflow-y-auto", "pointer-events-auto");
  expect(panel).not.toHaveClass("fixed");
  await waitFor(() => expect(screen.getByRole("button", { name: "Close" })).toHaveFocus());
});

it("follows zoom, pan and Actual Size while open and reads fresh bounds on reopen", async () => {
  const visible = viewport(1280, 720);
  render(<Example />);
  const trigger = screen.getByRole("button", { name: "Open details" });
  await userEvent.click(trigger);
  const frame = screen.getByRole("dialog").parentElement!;
  act(() => {
    Object.assign(visible, { width: 640, height: 360 });
    visible.dispatchEvent(new Event("resize"));
    Object.assign(visible, { offsetLeft: 320, offsetTop: 50 });
    visible.dispatchEvent(new Event("scroll"));
  });
  expect(frame).toHaveStyle({ left: "320px", top: "50px", width: "640px", height: "360px" });
  act(() => {
    Object.assign(visible, { width: 1280, height: 720, offsetLeft: 0, offsetTop: 0, scale: 1 });
    visible.dispatchEvent(new Event("resize"));
  });
  expect(frame).toHaveStyle({ left: "0px", top: "0px", width: "1280px", height: "720px" });
  await userEvent.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await waitFor(() => expect(trigger).toHaveFocus());
  Object.assign(visible, { width: 500, height: 300 });
  await userEvent.click(trigger);
  expect(screen.getByRole("dialog").parentElement).toHaveStyle({ width: "500px", height: "300px" });
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await waitFor(() => expect(trigger).toHaveFocus());
});

it("keeps the CSS viewport fallback and keyboard dismissal without VisualViewport", async () => {
  vi.stubGlobal("visualViewport", undefined);
  render(<Example />);
  const trigger = screen.getByRole("button", { name: "Open details" });
  trigger.focus();
  await userEvent.keyboard("{Enter}");
  const frame = screen.getByRole("dialog").parentElement!;
  expect(frame).toHaveClass("fixed", "inset-0");
  expect(frame.style.width).toBe("");
  expect(frame.style.transform).toBe("");
  await userEvent.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await waitFor(() => expect(trigger).toHaveFocus());
});

it("preserves disabled Close and outside dismissal of normal sheets", async () => {
  viewport(1280, 720);
  render(<Example closeDisabled />);
  await userEvent.click(screen.getByRole("button", { name: "Open details" }));
  expect(screen.getByRole("button", { name: "Close" })).toBeDisabled();
  // Pointer-transparent positioning frame leaves backdrop outside-press intact.
  await userEvent.click(screen.getByRole("dialog").parentElement!.previousElementSibling!);
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
});

it("removes both viewport subscriptions when the portal is disposed", async () => {
  const visible = viewport();
  const add = vi.spyOn(visible, "addEventListener");
  const remove = vi.spyOn(visible, "removeEventListener");
  const view = render(<Example />);
  await userEvent.click(screen.getByRole("button", { name: "Open details" }));
  const listeners = add.mock.calls.filter(([event]) => event === "resize" || event === "scroll");
  expect(listeners).toHaveLength(2);
  view.unmount();
  for (const [event, handler] of listeners) expect(remove).toHaveBeenCalledWith(event, handler);
});
