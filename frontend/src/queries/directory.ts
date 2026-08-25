import { useQuery } from "@tanstack/react-query";
import { Service as DirectoryService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/directory";
import { callService } from "@/lib/wails";

/** useMembers loads active Members for ownership selection in the Account
 * form. Members/Institutions/Groups management pages themselves are
 * Phase 5 work (see the "Directory" nav grouping decision); this hook
 * only supports Phase 4's Account form. */
export function useMembers(includeArchived = false) {
  return useQuery({
    queryKey: ["directory", "members", includeArchived],
    queryFn: () => callService(() => DirectoryService.ListMembers(includeArchived)),
  });
}

export function useInstitutions(includeArchived = false) {
  return useQuery({
    queryKey: ["directory", "institutions", includeArchived],
    queryFn: () => callService(() => DirectoryService.ListInstitutions(includeArchived)),
  });
}

export function useGroups(includeArchived = false) {
  return useQuery({
    queryKey: ["directory", "groups", includeArchived],
    queryFn: () => callService(() => DirectoryService.ListGroups(includeArchived)),
  });
}
