import { useEffect, useRef, useState, type ComponentProps, type KeyboardEvent, type PointerEvent } from "react";

type Props = Pick<ComponentProps<"div">, "aria-label" | "aria-labelledby" | "aria-controls" | "aria-valuetext" | "title"> & {
  /** Saved preference; the visible width may be temporarily clamped by bounds. */
  value: number;
  min: number;
  max: number;
  onValueChange: (value: number) => void;
  onCollapse: () => void;
};

/** A vertical splitter on the left edge of a right-hand panel. Its owner keeps
 * the preferred width and supplies bounds from the available workspace. */
export function PanelResizeHandle({ value, min, max, onValueChange, onCollapse, ...labelProps }: Props) {
  const drag = useRef<{ id: number; x: number; width: number; preferredWidth: number } | null>(null);
  const [dragging, setDragging] = useState(false);
  const clamp = (width: number) => Math.max(min, Math.min(max, Math.round(width)));
  const width = clamp(value);
  useEffect(() => {
    if (!dragging) return;
    const style = document.body.style;
    const { cursor, userSelect } = style;
    style.cursor = "col-resize";
    style.userSelect = "none";
    return () => { style.cursor = cursor; style.userSelect = userSelect; };
  }, [dragging]);

  function stop(element: HTMLDivElement) {
    const previous = drag.current;
    drag.current = null;
    setDragging(false);
    if (previous && element.hasPointerCapture(previous.id)) element.releasePointerCapture(previous.id);
  }
  function start(event: PointerEvent<HTMLDivElement>) {
    if (event.button !== 0 || event.isPrimary === false || drag.current) return;
    event.preventDefault();
    event.currentTarget.focus({ preventScroll: true });
    event.currentTarget.setPointerCapture(event.pointerId);
    drag.current = { id: event.pointerId, x: event.clientX, width, preferredWidth: value };
    setDragging(true);
  }
  function move(event: PointerEvent<HTMLDivElement>) {
    const current = drag.current;
    if (current?.id === event.pointerId) onValueChange(clamp(current.width + current.x - event.clientX));
  }
  function finish(event: PointerEvent<HTMLDivElement>) {
    if (drag.current?.id === event.pointerId) stop(event.currentTarget);
  }
  function keyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const step = event.shiftKey ? 64 : 16;
    switch (event.key) {
      case "ArrowLeft": onValueChange(clamp(width + step)); break;
      case "ArrowRight": onValueChange(clamp(width - step)); break;
      case "Home": onValueChange(min); break;
      case "End": onValueChange(max); break;
      case "Enter": stop(event.currentTarget); onCollapse(); break;
      case "Escape":
        if (!drag.current) return;
        // Roll back the preference, not a temporary viewport constraint. The
        // owner and aria-valuenow still render the restored value within bounds.
        onValueChange(drag.current.preferredWidth);
        stop(event.currentTarget);
        break;
      default: return;
    }
    event.preventDefault();
  }
  return <div {...labelProps} role="separator" aria-orientation="vertical" tabIndex={0}
    aria-valuenow={width} aria-valuemin={min} aria-valuemax={max}
    data-dragging={dragging || undefined}
    className="group absolute inset-y-0 -left-1.5 z-10 w-3 cursor-col-resize touch-none select-none outline-none hover:bg-primary/10 focus-visible:bg-primary/15 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary/50 data-[dragging]:bg-primary/15"
    onPointerDown={start} onPointerMove={move} onPointerUp={finish} onPointerCancel={finish} onLostPointerCapture={finish} onKeyDown={keyDown}>
    <span aria-hidden="true" className="pointer-events-none absolute left-1/2 top-1/2 h-8 w-1 -translate-x-1/2 -translate-y-1/2 rounded-full bg-muted-foreground/40 group-hover:bg-primary group-focus-visible:bg-primary group-data-[dragging]:bg-primary" />
  </div>;
}
