import { useTranslation } from "react-i18next";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { DirectoryEntityList } from "@/features/directory/DirectoryEntityList";
import {
  useMembers,
  useInstitutions,
  useGroups,
  useCreateMember,
  useUpdateMember,
  useArchiveMember,
  useSetMemberAvatar,
  useCreateInstitution,
  useUpdateInstitution,
  useArchiveInstitution,
  useSetInstitutionLogo,
  useCreateGroup,
  useUpdateGroup,
  useArchiveGroup,
  useSetGroupLogo,
} from "@/queries/directory";
import { attachPendingImage } from "@/queries/media";
import type { DirectoryCreatePayload, DirectoryCreateResult } from "@/features/directory/DirectoryEntityList";
import { PageHeader } from "@/components/layout/PageHeader";

/**
 * DirectoryPage groups Members, Institutions, and Groups under one nav entry
 * with three tabs.
 */
export function DirectoryPage() {
  const { t } = useTranslation();

  const members = useMembers();
  const createMember = useCreateMember();
  const updateMember = useUpdateMember();
  const archiveMember = useArchiveMember();
  const setMemberAvatar = useSetMemberAvatar();

  const institutions = useInstitutions();
  const createInstitution = useCreateInstitution();
  const updateInstitution = useUpdateInstitution();
  const archiveInstitution = useArchiveInstitution();
  const setInstitutionLogo = useSetInstitutionLogo();

  const groups = useGroups();
  const createGroup = useCreateGroup();
  const updateGroup = useUpdateGroup();
  const archiveGroup = useArchiveGroup();
  const setGroupLogo = useSetGroupLogo();

  const saveMember = async (payload: DirectoryCreatePayload): Promise<DirectoryCreateResult> => {
    const created = await createMember.mutateAsync(payload.name);
    try {
      await attachPendingImage(created.id, payload.pendingImage, (args) => setMemberAvatar.mutateAsync(args));
      return { mediaSaved: true };
    } catch {
      return { mediaSaved: false };
    }
  };

  const saveInstitution = async (payload: DirectoryCreatePayload): Promise<DirectoryCreateResult> => {
    const created = await createInstitution.mutateAsync({ name: payload.name, iconKey: payload.iconKey ?? "" });
    try {
      await attachPendingImage(created.id, payload.pendingImage, (args) => setInstitutionLogo.mutateAsync(args));
      return { mediaSaved: true };
    } catch {
      return { mediaSaved: false };
    }
  };

  const saveGroup = async (payload: DirectoryCreatePayload): Promise<DirectoryCreateResult> => {
    const created = await createGroup.mutateAsync({ name: payload.name, iconKey: payload.iconKey ?? "" });
    try {
      await attachPendingImage(created.id, payload.pendingImage, (args) => setGroupLogo.mutateAsync(args));
      return { mediaSaved: true };
    } catch {
      return { mediaSaved: false };
    }
  };

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("nav.directory")} description={t("directory.description")} />
      <Tabs defaultValue="members">
        <TabsList>
          <TabsTrigger value="members">{t("nav.members")}</TabsTrigger>
          <TabsTrigger value="institutions">{t("nav.institutions")}</TabsTrigger>
          <TabsTrigger value="groups">{t("nav.groups")}</TabsTrigger>
        </TabsList>
        <TabsContent value="members">
          <DirectoryEntityList
          entities={members.data ?? undefined}
          isLoading={members.isLoading}
          isError={members.isError}
          onCreate={saveMember}
          onUpdate={(id, name) => updateMember.mutateAsync({ id, name }).then(() => undefined)}
          onArchive={(id, archived) => archiveMember.mutateAsync({ id, archived }).then(() => undefined)}
          onSetImage={(id, pendingImage) => attachPendingImage(id, pendingImage, (args) => setMemberAvatar.mutateAsync(args))}
          onRetry={() => members.refetch()}
          entityLabel={t("nav.members")}
          createLabel={t("onboarding.memberNamePlaceholder")}
          emptyLabel={t("members.createDescription")}
          />
        </TabsContent>
        <TabsContent value="institutions">
          <DirectoryEntityList
          entities={institutions.data ?? undefined}
          isLoading={institutions.isLoading}
          isError={institutions.isError}
          onCreate={saveInstitution}
          onUpdate={(id, name) => updateInstitution.mutateAsync({ id, name }).then(() => undefined)}
          onArchive={(id, archived) => archiveInstitution.mutateAsync({ id, archived }).then(() => undefined)}
          onSetImage={(id, pendingImage) => attachPendingImage(id, pendingImage, (args) => setInstitutionLogo.mutateAsync(args))}
          onRetry={() => institutions.refetch()}
          entityLabel={t("nav.institutions")}
          createLabel={t("institutions.createTitle")}
          emptyLabel={t("institutions.createDescription")}
          supportsIcon
          />
        </TabsContent>
        <TabsContent value="groups">
          <DirectoryEntityList
          entities={groups.data ?? undefined}
          isLoading={groups.isLoading}
          isError={groups.isError}
          onCreate={saveGroup}
          onUpdate={(id, name) => updateGroup.mutateAsync({ id, name }).then(() => undefined)}
          onArchive={(id, archived) => archiveGroup.mutateAsync({ id, archived }).then(() => undefined)}
          onSetImage={(id, pendingImage) => attachPendingImage(id, pendingImage, (args) => setGroupLogo.mutateAsync(args))}
          onRetry={() => groups.refetch()}
          entityLabel={t("nav.groups")}
          createLabel={t("groups.createTitle")}
          emptyLabel={t("groups.createDescription")}
          supportsIcon
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
