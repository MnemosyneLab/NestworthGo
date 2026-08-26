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
import { persistPickedImage } from "@/queries/media";
import type { DirectoryCreatePayload } from "@/features/directory/DirectoryEntityList";

async function attachImage(
  id: string,
  pendingImage: string | undefined,
  setLogo: (args: { id: string; mediaAssetId: string }) => Promise<unknown>,
) {
  if (!pendingImage) {
    return;
  }
  await persistPickedImage(pendingImage, (mediaAssetId) => setLogo({ id, mediaAssetId }));
}

/**
 * DirectoryPage groups Members/Institutions/Groups under one nav entry
 * with three tabs, per docs/migration/wails-v3-navigation-decisions.md's
 * decision 4.
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

  const saveMember = (payload: DirectoryCreatePayload) => {
    createMember.mutate(payload.name, {
      onSuccess: (created) => {
        void attachImage(created.id, payload.pendingImage, (args) => setMemberAvatar.mutateAsync(args));
      },
    });
  };

  const saveInstitution = (payload: DirectoryCreatePayload) => {
    createInstitution.mutate(
      { name: payload.name, iconKey: payload.iconKey ?? "" },
      {
        onSuccess: (created) => {
          void attachImage(created.id, payload.pendingImage, (args) => setInstitutionLogo.mutateAsync(args));
        },
      },
    );
  };

  const saveGroup = (payload: DirectoryCreatePayload) => {
    createGroup.mutate(
      { name: payload.name, iconKey: payload.iconKey ?? "" },
      {
        onSuccess: (created) => {
          void attachImage(created.id, payload.pendingImage, (args) => setGroupLogo.mutateAsync(args));
        },
      },
    );
  };

  return (
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
          onUpdate={(id, name) => updateMember.mutate({ id, name })}
          onArchive={(id, archived) => archiveMember.mutate({ id, archived })}
          onSetImage={(id, pendingImage) => {
            void attachImage(id, pendingImage, (args) => setMemberAvatar.mutateAsync(args));
          }}
          createLabel={t("onboarding.memberNamePlaceholder", { defaultValue: "Member name" })}
          emptyLabel={t("members.createDescription")}
        />
      </TabsContent>
      <TabsContent value="institutions">
        <DirectoryEntityList
          entities={institutions.data ?? undefined}
          isLoading={institutions.isLoading}
          isError={institutions.isError}
          onCreate={saveInstitution}
          onUpdate={(id, name) => updateInstitution.mutate({ id, name })}
          onArchive={(id, archived) => archiveInstitution.mutate({ id, archived })}
          onSetImage={(id, pendingImage) => {
            void attachImage(id, pendingImage, (args) => setInstitutionLogo.mutateAsync(args));
          }}
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
          onUpdate={(id, name) => updateGroup.mutate({ id, name })}
          onArchive={(id, archived) => archiveGroup.mutate({ id, archived })}
          onSetImage={(id, pendingImage) => {
            void attachImage(id, pendingImage, (args) => setGroupLogo.mutateAsync(args));
          }}
          createLabel={t("groups.createTitle")}
          emptyLabel={t("groups.createDescription")}
          supportsIcon
        />
      </TabsContent>
    </Tabs>
  );
}
