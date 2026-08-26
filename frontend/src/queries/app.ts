import { useQuery } from "@tanstack/react-query";
import { Service as AppService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/app";
import { callService } from "@/lib/wails";

/**
 * useAppInfo is the first real TanStack Query hook, proving the
 * queries/ -> lib/wails.callService -> generated binding pipeline
 * (implementation plan Phase 3). Later phases add one hook per page here
 * (queries/accounts.ts, queries/overview.ts, ...), each following this
 * same shape.
 */
export function useAppInfo() {
  return useQuery({
    queryKey: ["app", "info"],
    queryFn: () => callService(() => AppService.AppInfo()),
  });
}

/**
 * useStartup is the gate in front of every other bound service. When the
 * local database could not be opened, only AppService is registered, so
 * calling HouseholdService.Bootstrap would reject as a raw Wails
 * "service not found" string. Startup() returns a DTO either way.
 */
export function useStartup() {
  return useQuery({
    queryKey: ["app", "startup"],
    queryFn: () => callService(() => AppService.Startup()),
  });
}
