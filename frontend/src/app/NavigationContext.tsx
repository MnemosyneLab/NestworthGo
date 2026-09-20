import { createContext, useContext } from "react";
import type { NavigationTarget, HealthFocus } from "./navigation";

export interface ObjectNavigation {
  open: (target: NavigationTarget) => void;
  openHealth: (focus: HealthFocus) => void;
}
export const NavigationContext = createContext<ObjectNavigation | null>(null);
export const useObjectNavigation = () => useContext(NavigationContext);
