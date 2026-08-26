import { useEffect } from "react";
import { useUiStore, type Appearance } from "@/stores/ui";

function systemPrefersDark(): boolean {
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;
}

function applyThemeClass(appearance: Appearance) {
  const isDark = appearance === "dark" || (appearance === "system" && systemPrefersDark());
  document.documentElement.classList.toggle("dark", isDark);
}

/**
 * useTheme applies the Tailwind `dark` class to <html> based on the
 * current appearance preference, and — only in "system" mode — reacts to
 * the OS-level prefers-color-scheme change, matching Fyne's
 * ApplyTheme(fyneApplication, preference) behavior (technical design
 * Sec9). SettingsService (Phase 5) is the source of truth for the
 * persisted preference; this hook only applies whatever the current
 * value is.
 */
export function useTheme() {
  const appearance = useUiStore((state) => state.appearance);

  useEffect(() => {
    applyThemeClass(appearance);
    if (appearance !== "system") {
      return;
    }
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const listener = () => applyThemeClass(appearance);
    media.addEventListener("change", listener);
    return () => media.removeEventListener("change", listener);
  }, [appearance]);

  return appearance;
}
