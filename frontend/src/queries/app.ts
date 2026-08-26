import { useQuery } from "@tanstack/react-query";
import { Service as AppService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/app";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";

/**
 * useAppInfo exercises the queries/ -> lib/wails.callService -> generated
 * binding pipeline. Each page-specific hook follows the same shape.
 */
export function useAppInfo() {
  return useQuery({
    queryKey: queryKeys.app.info,
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
    queryKey: queryKeys.app.startup,
    queryFn: () => callService(() => AppService.Startup()),
  });
}
