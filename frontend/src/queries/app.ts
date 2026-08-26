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
