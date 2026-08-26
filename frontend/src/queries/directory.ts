import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service as DirectoryService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/directory";
import { callService } from "@/lib/wails";
import { overviewQueryKey } from "@/queries/portfolio";

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

function invalidateDirectory(queryClient: ReturnType<typeof useQueryClient>, entity: "members" | "institutions" | "groups") {
  void queryClient.invalidateQueries({ queryKey: ["directory", entity] });
  void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
}

export function useCreateMember() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => callService(() => DirectoryService.CreateMember(name)),
    onSuccess: () => invalidateDirectory(queryClient, "members"),
  });
}

export function useUpdateMember() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => callService(() => DirectoryService.UpdateMember(id, name)),
    onSuccess: () => invalidateDirectory(queryClient, "members"),
  });
}

export function useArchiveMember() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, archived }: { id: string; archived: boolean }) => callService(() => DirectoryService.ArchiveMember(id, archived)),
    onSuccess: () => invalidateDirectory(queryClient, "members"),
  });
}

export function useCreateInstitution() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, iconKey }: { name: string; iconKey: string }) => callService(() => DirectoryService.CreateInstitution(name, iconKey)),
    onSuccess: () => invalidateDirectory(queryClient, "institutions"),
  });
}

export function useUpdateInstitution() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => callService(() => DirectoryService.UpdateInstitution(id, name)),
    onSuccess: () => invalidateDirectory(queryClient, "institutions"),
  });
}

export function useArchiveInstitution() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, archived }: { id: string; archived: boolean }) => callService(() => DirectoryService.ArchiveInstitution(id, archived)),
    onSuccess: () => invalidateDirectory(queryClient, "institutions"),
  });
}

export function useCreateGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, iconKey }: { name: string; iconKey: string }) => callService(() => DirectoryService.CreateGroup(name, iconKey)),
    onSuccess: () => invalidateDirectory(queryClient, "groups"),
  });
}

export function useUpdateGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => callService(() => DirectoryService.UpdateGroup(id, name)),
    onSuccess: () => invalidateDirectory(queryClient, "groups"),
  });
}

export function useArchiveGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, archived }: { id: string; archived: boolean }) => callService(() => DirectoryService.ArchiveGroup(id, archived)),
    onSuccess: () => invalidateDirectory(queryClient, "groups"),
  });
}
