import { useTranslation } from "react-i18next";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { DirectoryEntityList, type DirectoryCreatePayload } from "@/features/directory/DirectoryEntityList";
import { useMembers, useInstitutions, useGroups, useCreateMember, useUpdateMember, useArchiveMember, useSetMemberIcon, useCreateInstitution, useUpdateInstitution, useArchiveInstitution, useSetInstitutionIcon, useCreateGroup, useUpdateGroup, useArchiveGroup, useSetGroupIcon } from "@/queries/directory";
import { useCatalog } from "@/queries/catalog";
import { PageHeader } from "@/components/layout/PageHeader";

export function DirectoryPage() {
  const { t } = useTranslation(); const catalog = useCatalog();
  const members = useMembers(); const createMember = useCreateMember(); const updateMember = useUpdateMember(); const archiveMember = useArchiveMember(); const setMemberIcon = useSetMemberIcon();
  const institutions = useInstitutions(); const createInstitution = useCreateInstitution(); const updateInstitution = useUpdateInstitution(); const archiveInstitution = useArchiveInstitution(); const setInstitutionIcon = useSetInstitutionIcon();
  const groups = useGroups(); const createGroup = useCreateGroup(); const updateGroup = useUpdateGroup(); const archiveGroup = useArchiveGroup(); const setGroupIcon = useSetGroupIcon();

  return <div className="flex flex-col gap-6"><PageHeader title={t("nav.directory")} description={t("directory.description")} />
    <Tabs defaultValue="members"><TabsList><TabsTrigger value="members">{t("nav.members")}</TabsTrigger><TabsTrigger value="institutions">{t("nav.institutions")}</TabsTrigger><TabsTrigger value="groups">{t("nav.groups")}</TabsTrigger></TabsList>
      <TabsContent value="members"><DirectoryEntityList kind="member" entities={members.data ?? undefined} isLoading={members.isLoading} isError={members.isError}
        onCreate={(payload: DirectoryCreatePayload) => createMember.mutateAsync({ name: payload.name, iconKey: payload.iconKey })}
        onUpdate={(id, name) => updateMember.mutateAsync({ id, name })} onArchive={(id, archived) => archiveMember.mutateAsync({ id, archived })}
        onSetIcon={(id, iconKey) => setMemberIcon.mutateAsync({ id, iconKey })} onRetry={() => members.refetch()}
        createLabel={t("onboarding.memberNamePlaceholder")} addLabel={t("members.createTitle")} emptyLabel={t("members.createDescription")} /></TabsContent>
      <TabsContent value="institutions"><DirectoryEntityList kind="institution" entities={institutions.data ?? undefined} isLoading={institutions.isLoading} isError={institutions.isError}
        institutionTypes={catalog.data?.institutionTypes ?? []}
        onCreate={(payload: DirectoryCreatePayload) => createInstitution.mutateAsync({ name: payload.name, institutionType: payload.institutionType ?? "", iconKey: payload.iconKey })}
        onUpdate={(id, name) => updateInstitution.mutateAsync({ id, name })} onArchive={(id, archived) => archiveInstitution.mutateAsync({ id, archived })}
        onSetIcon={(id, iconKey) => setInstitutionIcon.mutateAsync({ id, iconKey })} onRetry={() => institutions.refetch()}
        createLabel={t("institutions.createTitle")} addLabel={t("institutions.createTitle")} emptyLabel={t("institutions.createDescription")} /></TabsContent>
      <TabsContent value="groups"><DirectoryEntityList kind="group" entities={groups.data ?? undefined} isLoading={groups.isLoading} isError={groups.isError}
        onCreate={(payload: DirectoryCreatePayload) => createGroup.mutateAsync({ name: payload.name, iconKey: payload.iconKey })}
        onUpdate={(id, name) => updateGroup.mutateAsync({ id, name })} onArchive={(id, archived) => archiveGroup.mutateAsync({ id, archived })}
        onSetIcon={(id, iconKey) => setGroupIcon.mutateAsync({ id, iconKey })} onRetry={() => groups.refetch()}
        createLabel={t("groups.createTitle")} addLabel={t("groups.createTitle")} emptyLabel={t("groups.createDescription")} /></TabsContent>
    </Tabs></div>;
}
