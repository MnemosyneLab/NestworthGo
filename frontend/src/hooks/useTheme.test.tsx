import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useTheme } from "./useTheme";
import { useUiStore } from "@/stores/ui";

function mockMatchMedia(initialPrefersDark: boolean) {
  const listeners: Array<() => void> = [];
  const state = { prefersDark: initialPrefersDark };
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    get matches() {
      return query.includes("dark") ? state.prefersDark : false;
    },
    media: query,
    addEventListener: (_event: string, listener: () => void) => listeners.push(listener),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })) as unknown as typeof window.matchMedia;
  return {
    setPrefersDark: (value: boolean) => {
      state.prefersDark = value;
    },
    fire: () => listeners.forEach((listener) => listener()),
  };
}

describe("useTheme", () => {
  beforeEach(() => {
    document.documentElement.classList.remove("dark");
    useUiStore.setState({ appearance: "system" });
  });

  afterEach(() => {
    document.documentElement.classList.remove("dark");
  });

  it("applies the dark class when appearance is explicitly dark", () => {
    mockMatchMedia(false);
    useUiStore.setState({ appearance: "dark" });
    renderHook(() => useTheme());
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  it("does not apply the dark class when appearance is explicitly light", () => {
    mockMatchMedia(true);
    useUiStore.setState({ appearance: "light" });
    renderHook(() => useTheme());
    expect(document.documentElement.classList.contains("dark")).toBe(false);
  });

  it("follows the OS preference when appearance is system", () => {
    mockMatchMedia(true);
    useUiStore.setState({ appearance: "system" });
    renderHook(() => useTheme());
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });

  it("reacts to a live OS preference change while in system mode", () => {
    const media = mockMatchMedia(false);
    useUiStore.setState({ appearance: "system" });
    renderHook(() => useTheme());
    expect(document.documentElement.classList.contains("dark")).toBe(false);

    media.setPrefersDark(true);
    act(() => media.fire());
    expect(document.documentElement.classList.contains("dark")).toBe(true);
  });
});
