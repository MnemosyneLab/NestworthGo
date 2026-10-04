import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";
import i18n from "@/i18n";

// jsdom has no layout boxes. Supply a minimal box for displayed elements so
// tabbable can exercise its real visibility/disabled/radio rules in focus tests.
// This is not geometry or native visual verification.
function hasDisplayBox(element: HTMLElement | null): boolean {
  return !element || (getComputedStyle(element).display !== "none" && hasDisplayBox(element.parentElement));
}
HTMLElement.prototype.getClientRects = function () {
  return (hasDisplayBox(this) ? [new DOMRect(0, 0, 1, 1)] : []) as unknown as DOMRectList;
};

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}

if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver;
}

if (typeof window.matchMedia !== "function") {
  window.matchMedia = (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
    dispatchEvent: () => false,
  }) as MediaQueryList;
}

if (typeof HTMLCanvasElement !== "undefined") {
  HTMLCanvasElement.prototype.getContext = function getContext(this: HTMLCanvasElement, contextId: string) {
    if (contextId !== "2d") {
      return null;
    }
    return {
      canvas: this,
      fillRect: () => undefined,
      clearRect: () => undefined,
      getImageData: () => ({ data: [] }),
      putImageData: () => undefined,
      createImageData: () => [],
      setTransform: () => undefined,
      drawImage: () => undefined,
      save: () => undefined,
      fillText: () => undefined,
      restore: () => undefined,
      beginPath: () => undefined,
      moveTo: () => undefined,
      lineTo: () => undefined,
      closePath: () => undefined,
      stroke: () => undefined,
      translate: () => undefined,
      scale: () => undefined,
      rotate: () => undefined,
      arc: () => undefined,
      fill: () => undefined,
      measureText: () => ({ width: 0 }),
      transform: () => undefined,
      rect: () => undefined,
      clip: () => undefined,
    };
  } as typeof HTMLCanvasElement.prototype.getContext;
}

afterEach(() => {
  cleanup();
  // i18next is a module-level singleton; a test that changes the active
  // language (e.g. saving Settings) must not leak that into the next
  // test in the same file.
  void i18n.changeLanguage(i18n.options.lng);
});
