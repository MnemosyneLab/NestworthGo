import { useEffect } from "react";
import { useUiStore, type Appearance, type Accent } from "@/stores/ui";

function systemPrefersDark(): boolean {
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;
}

function applyThemeClass(appearance: Appearance, accent: Accent) {
  const isDark = appearance === "dark" || (appearance === "system" && systemPrefersDark());
  document.documentElement.classList.toggle("dark", isDark);
  document.documentElement.setAttribute("data-accent", accent);
}

/**
 * useTheme applies the Tailwind `dark` class and `data-accent` attribute to
 * <html> based on the current appearance/accent preference, and — only in
 * "system" appearance mode — reacts to the OS-level prefers-color-scheme
 * change. SettingsService is the source of truth for the persisted
 * preference; this hook only applies whatever the current value is.
 */
export function useTheme() {
  const appearance = useUiStore((state) => state.appearance);
  const accent = useUiStore((state) => state.accent);

  useEffect(() => {
    applyThemeClass(appearance, accent);
    if (appearance !== "system") {
      return;
    }
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const listener = () => applyThemeClass(appearance, accent);
    media.addEventListener("change", listener);
    return () => media.removeEventListener("change", listener);
  }, [appearance, accent]);

  return appearance;
}
