import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
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

it("keeps the visible close affordance in the non-shrinking sticky title region while content scrolls", async () => {
  const visible = viewport(360, 240, 80, 20);
  render(<Example />);
  const trigger = screen.getByRole("button", { name: "Open details" });
  await userEvent.click(trigger);
  const panel = screen.getByRole("dialog");
  const close = screen.getByRole("button", { name: "Close" });
  const header = close.parentElement!;
  expect(within(header).getByRole("heading", { name: "Account details" })).toBeInTheDocument();
  expect(header).toHaveClass("sticky", "shrink-0", "items-start");
  expect(close).toHaveClass("inline-flex", "min-h-8", "shrink-0");
  expect(close).not.toHaveClass("absolute");
  expect(close.querySelector("svg")).toHaveClass("size-4");
  expect(within(close).getByText("Close")).not.toHaveClass("sr-only");
  expect(screen.getAllByRole("button", { name: "Close" })).toHaveLength(1);
  act(() => {
    panel.scrollTop = 500;
    fireEvent.scroll(panel);
    Object.assign(visible, { width: 300, offsetLeft: 140 });
    visible.dispatchEvent(new Event("resize"));
  });
  // This is a structural regression guard, not a native pixel assertion.
  expect(close.parentElement).toBe(header);
  expect(header.parentElement).toBe(panel);
  expect(close).toBeVisible();
  await userEvent.click(close);
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await waitFor(() => expect(trigger).toHaveFocus());
});

it.each(["explicit", "autofocus"])("preserves %s form focus and propagates closeDisabled to the header", async focus => {
  const form = (disabled: boolean) => <Sheet defaultOpen>
    <SheetContent closeDisabled={disabled} initialFocus={focus === "explicit" ? () => document.getElementById("account-name") : undefined}>
      <SheetHeader><SheetTitle>New account</SheetTitle><SheetDescription>Account form</SheetDescription></SheetHeader>
      <label>Name<input id="account-name" autoFocus={focus === "autofocus"} /></label>
    </SheetContent>
  </Sheet>;
  const view = render(form(false));
  await waitFor(() => expect(screen.getByRole("textbox", { name: "Name" })).toHaveFocus());
  view.rerender(form(true));
  expect(screen.getByRole("button", { name: "Close" })).toBeDisabled();
  view.rerender(form(false));
  expect(screen.getByRole("button", { name: "Close" })).toBeEnabled();
  await userEvent.click(screen.getByRole("button", { name: "Close" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
});

it("defaults to the first usable content control without a caller focus override", async () => {
  render(<Sheet><SheetTrigger>Open form</SheetTrigger><SheetContent>
    <SheetHeader><SheetTitle>Form</SheetTitle><SheetDescription>Default focus</SheetDescription></SheetHeader>
    <div hidden><input aria-label="Hidden ancestor" /></div>
    <input type="hidden" />
    <fieldset disabled><input aria-label="Disabled fieldset" /></fieldset>
    <input tabIndex={-1} aria-label="Not tabbable" />
    <label>Name<input /></label>
  </SheetContent></Sheet>);
  const trigger = screen.getByRole("button", { name: "Open form" });
  await userEvent.click(trigger);
  const name = screen.getByRole("textbox", { name: "Name" });
  await waitFor(() => expect(name).toHaveFocus());
  await userEvent.keyboard("Draft name");
  expect(name).toHaveValue("Draft name");
  await userEvent.tab({ shift: true });
  expect(screen.getByRole("button", { name: "Close" })).toHaveFocus();
  await userEvent.keyboard("{Enter}");
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  await waitFor(() => expect(trigger).toHaveFocus());
});

it("preserves an explicit opt-out of initial focus", async () => {
  render(<Sheet><SheetTrigger>Open form</SheetTrigger><SheetContent initialFocus={false}>
    <SheetHeader><SheetTitle>Form</SheetTitle><SheetDescription>No automatic focus</SheetDescription></SheetHeader>
    <label>Name<input /></label>
  </SheetContent></Sheet>);
  const trigger = screen.getByRole("button", { name: "Open form" });
  await userEvent.click(trigger);
  await screen.findByRole("dialog");
  expect(trigger).toHaveFocus();
});

it("keeps touch opening focused on the popup instead of opening a field keyboard", async () => {
  render(<Sheet><SheetTrigger>Open form</SheetTrigger><SheetContent>
    <SheetHeader><SheetTitle>Form</SheetTitle><SheetDescription>Touch focus</SheetDescription></SheetHeader>
    <label>Name<input /></label>
  </SheetContent></Sheet>);
  await userEvent.pointer([{ keys: "[TouchA>]", target: screen.getByRole("button", { name: "Open form" }) }, { keys: "[/TouchA]" }]);
  await waitFor(() => expect(screen.getByRole("dialog")).toHaveFocus());
});
