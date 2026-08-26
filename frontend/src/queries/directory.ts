import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service as DirectoryService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/directory";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";
import { invalidateDirectoryChange } from "@/queries/invalidation";

/** useMembers loads active Members for ownership selection in the Account
 * form. Directory management pages use the same service boundary. */
export function useMembers(includeArchived = false) {
  return useQuery({
    queryKey: queryKeys.directory.members(Boolean(includeArchived)),
    queryFn: () => callService(() => DirectoryService.ListMembers(Boolean(includeArchived))),
  });
}

export function useInstitutions(includeArchived = false) {
  return useQuery({
    queryKey: queryKeys.directory.institutions(Boolean(includeArchived)),
    queryFn: () => callService(() => DirectoryService.ListInstitutions(Boolean(includeArchived))),
  });
}

export function useGroups(includeArchived = false) {
  return useQuery({
    queryKey: queryKeys.directory.groups(Boolean(includeArchived)),
    queryFn: () => callService(() => DirectoryService.ListGroups(Boolean(includeArchived))),
  });
}

export function useCreateMember() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => callService(() => DirectoryService.CreateMember(name)),
    onSuccess: () => invalidateDirectoryChange(queryClient, "members"),
  });
}

export function useUpdateMember() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => callService(() => DirectoryService.UpdateMember(id, name)),
    onSuccess: () => invalidateDirectoryChange(queryClient, "members"),
  });
}

export function useArchiveMember() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, archived }: { id: string; archived: boolean }) => callService(() => DirectoryService.ArchiveMember(id, archived)),
    onSuccess: () => invalidateDirectoryChange(queryClient, "members"),
  });
}

export function useCreateInstitution() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, iconKey }: { name: string; iconKey: string }) => callService(() => DirectoryService.CreateInstitution(name, iconKey)),
    onSuccess: () => invalidateDirectoryChange(queryClient, "institutions"),
  });
}

export function useUpdateInstitution() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => callService(() => DirectoryService.UpdateInstitution(id, name)),
    onSuccess: () => invalidateDirectoryChange(queryClient, "institutions"),
  });
}

export function useArchiveInstitution() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, archived }: { id: string; archived: boolean }) => callService(() => DirectoryService.ArchiveInstitution(id, archived)),
    onSuccess: () => invalidateDirectoryChange(queryClient, "institutions"),
  });
}

export function useCreateGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, iconKey }: { name: string; iconKey: string }) => callService(() => DirectoryService.CreateGroup(name, iconKey)),
    onSuccess: () => invalidateDirectoryChange(queryClient, "groups"),
  });
}

export function useUpdateGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => callService(() => DirectoryService.UpdateGroup(id, name)),
    onSuccess: () => invalidateDirectoryChange(queryClient, "groups"),
  });
}

export function useArchiveGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, archived }: { id: string; archived: boolean }) => callService(() => DirectoryService.ArchiveGroup(id, archived)),
    onSuccess: () => invalidateDirectoryChange(queryClient, "groups"),
  });
}

export function useSetMemberAvatar() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, mediaAssetId }: { id: string; mediaAssetId: string }) =>
      callService(() => DirectoryService.SetMemberAvatar(id, mediaAssetId)),
    onSuccess: () => invalidateDirectoryChange(queryClient, "members"),
  });
}

export function useSetInstitutionLogo() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, mediaAssetId }: { id: string; mediaAssetId: string }) =>
      callService(() => DirectoryService.SetInstitutionLogo(id, mediaAssetId)),
    onSuccess: () => invalidateDirectoryChange(queryClient, "institutions"),
  });
}

export function useSetGroupLogo() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, mediaAssetId }: { id: string; mediaAssetId: string }) =>
      callService(() => DirectoryService.SetGroupLogo(id, mediaAssetId)),
    onSuccess: () => invalidateDirectoryChange(queryClient, "groups"),
  });
}
