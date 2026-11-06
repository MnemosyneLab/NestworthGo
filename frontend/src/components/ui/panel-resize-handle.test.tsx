import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { PanelResizeHandle } from "./panel-resize-handle";

function mount(onCollapse = vi.fn()) {
  function Panel() {
    const [width, setWidth] = useState(400);
    return <PanelResizeHandle aria-label="Assistant" aria-controls="panel" value={width} min={320} max={600} onValueChange={setWidth} onCollapse={onCollapse} />;
  }
  const view = render(<Panel />);
  const handle = screen.getByRole("separator", { name: "Assistant" });
  let capture: number | null = null;
  handle.setPointerCapture = vi.fn(id => { capture = id; });
  handle.hasPointerCapture = id => capture === id;
  handle.releasePointerCapture = vi.fn(() => { capture = null; });
  return { ...view, handle, onCollapse };
}

it("drags the right panel within bounds using only the captured primary pointer", () => {
  const { handle } = mount();
  fireEvent.pointerDown(handle, { pointerId: 2, button: 2, clientX: 900 });
  expect(handle.setPointerCapture).not.toHaveBeenCalled();
  fireEvent.pointerDown(handle, { pointerId: 1, button: 0, clientX: 900, isPrimary: true });
  expect(handle).toHaveFocus();
  expect(handle.setPointerCapture).toHaveBeenCalledWith(1);
  expect(document.body.style.cursor).toBe("col-resize");
  expect(document.body.style.userSelect).toBe("none");
  fireEvent.pointerMove(handle, { pointerId: 2, clientX: 100 });
  expect(handle).toHaveAttribute("aria-valuenow", "400");
  fireEvent.pointerMove(handle, { pointerId: 1, clientX: 800 });
  expect(handle).toHaveAttribute("aria-valuenow", "500");
  fireEvent.pointerMove(handle, { pointerId: 1, clientX: 100 });
  expect(handle).toHaveAttribute("aria-valuenow", "600");
  fireEvent.pointerMove(handle, { pointerId: 1, clientX: 1600 });
  expect(handle).toHaveAttribute("aria-valuenow", "320");
  fireEvent.pointerUp(handle, { pointerId: 1 });
  expect(handle.releasePointerCapture).toHaveBeenCalledWith(1);
  expect(document.body.style.cursor).toBe("");
  expect(document.body.style.userSelect).toBe("");
  fireEvent.pointerMove(handle, { pointerId: 1, clientX: 400 });
  expect(handle).toHaveAttribute("aria-valuenow", "320");
});

it("offers directional, accelerated and endpoint keys with an accessible collapse action", async () => {
  const { handle, onCollapse } = mount();
  const user = userEvent.setup();
  await user.tab();
  expect(handle).toHaveFocus();
  expect(handle).toHaveAttribute("aria-orientation", "vertical");
  expect(handle).toHaveAttribute("aria-controls", "panel");
  expect(handle).toHaveAttribute("aria-valuemin", "320");
  expect(handle).toHaveAttribute("aria-valuemax", "600");
  await user.keyboard("{ArrowLeft}");
  expect(handle).toHaveAttribute("aria-valuenow", "416");
  await user.keyboard("{Shift>}{ArrowRight}{/Shift}");
  expect(handle).toHaveAttribute("aria-valuenow", "352");
  await user.keyboard("{Home}{ArrowRight}");
  expect(handle).toHaveAttribute("aria-valuenow", "320");
  await user.keyboard("{End}{ArrowLeft}");
  expect(handle).toHaveAttribute("aria-valuenow", "600");
  await user.keyboard("{Enter}");
  expect(onCollapse).toHaveBeenCalledOnce();
});

it("restores the initial width when Escape cancels a drag", () => {
  const { handle } = mount();
  fireEvent.pointerDown(handle, { pointerId: 1, button: 0, clientX: 900, isPrimary: true });
  fireEvent.pointerMove(handle, { pointerId: 1, clientX: 800 });
  expect(handle).toHaveAttribute("aria-valuenow", "500");
  fireEvent.keyDown(handle, { key: "Escape" });
  expect(handle).toHaveAttribute("aria-valuenow", "400");
  expect(handle.releasePointerCapture).toHaveBeenCalledWith(1);
  expect(document.body.style.cursor).toBe("");
});

it.each(["pointerCancel", "lostPointerCapture", "unmount"])("cleans up an interrupted drag on %s", event => {
  const { handle, unmount } = mount();
  document.body.style.cursor = "crosshair";
  document.body.style.userSelect = "text";
  try {
    fireEvent.pointerDown(handle, { pointerId: 1, button: 0, clientX: 900, isPrimary: true });
    expect(document.body.style.cursor).toBe("col-resize");
    if (event === "unmount") unmount();
    else if (event === "pointerCancel") fireEvent.pointerCancel(handle, { pointerId: 1 });
    else fireEvent.lostPointerCapture(handle, { pointerId: 1 });
    expect(document.body.style.cursor).toBe("crosshair");
    expect(document.body.style.userSelect).toBe("text");
  } finally {
    document.body.style.cursor = "";
    document.body.style.userSelect = "";
  }
});
