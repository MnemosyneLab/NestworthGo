import { useTranslation } from "react-i18next";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { DirectoryEntityList } from "@/features/directory/DirectoryEntityList";
import {
  useMembers,
  useInstitutions,
  useGroups,
  useCreateMember,
  useArchiveMember,
  useCreateInstitution,
  useArchiveInstitution,
  useCreateGroup,
  useArchiveGroup,
} from "@/queries/directory";

/**
 * DirectoryPage groups Members/Institutions/Groups under one nav entry
 * with three tabs, per docs/migration/wails-v3-navigation-decisions.md's
 * decision 4. Each tab is the same DirectoryEntityList component wired to
 * its own DirectoryService methods (implementation plan Phase 5).
 */
export function DirectoryPage() {
  const { t } = useTranslation();

  const members = useMembers();
  const createMember = useCreateMember();
  const archiveMember = useArchiveMember();

  const institutions = useInstitutions();
  const createInstitution = useCreateInstitution();
  const archiveInstitution = useArchiveInstitution();

  const groups = useGroups();
  const createGroup = useCreateGroup();
  const archiveGroup = useArchiveGroup();

  return (
    <Tabs defaultValue="members">
      <TabsList>
        <TabsTrigger value="members">{t("nav.members")}</TabsTrigger>
        <TabsTrigger value="institutions">{t("nav.institutions")}</TabsTrigger>
        <TabsTrigger value="groups">{t("nav.groups")}</TabsTrigger>
      </TabsList>
      <TabsContent value="members">
        <DirectoryEntityList
          entities={members.data}
          isLoading={members.isLoading}
          isError={members.isError}
          onCreate={(name) => createMember.mutate(name)}
          onArchive={(id, archived) => archiveMember.mutate({ id, archived })}
          createLabel={t("onboarding.memberNamePlaceholder", { defaultValue: "Member name" })}
          emptyLabel={t("common.comingSoon")}
        />
      </TabsContent>
      <TabsContent value="institutions">
        <DirectoryEntityList
          entities={institutions.data}
          isLoading={institutions.isLoading}
          isError={institutions.isError}
          onCreate={(name) => createInstitution.mutate({ name, iconKey: "" })}
          onArchive={(id, archived) => archiveInstitution.mutate({ id, archived })}
          createLabel={t("nav.institutions")}
          emptyLabel={t("common.comingSoon")}
        />
      </TabsContent>
      <TabsContent value="groups">
        <DirectoryEntityList
          entities={groups.data}
          isLoading={groups.isLoading}
          isError={groups.isError}
          onCreate={(name) => createGroup.mutate({ name, iconKey: "" })}
          onArchive={(id, archived) => archiveGroup.mutate({ id, archived })}
          createLabel={t("nav.groups")}
          emptyLabel={t("common.comingSoon")}
        />
      </TabsContent>
    </Tabs>
  );
}
