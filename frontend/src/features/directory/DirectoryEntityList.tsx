import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Plus } from "lucide-react";
import { toast } from "sonner";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { cn } from "@/lib/utils";
import { IconPicker } from "@/components/forms/IconPicker";
import { ImagePicker } from "@/components/forms/ImagePicker";

interface Entity {
  id: string;
  name: string;
  archivedAt?: string | null;
  iconKey?: string | null;
  logoAssetId?: string | null;
  avatarAssetId?: string | null;
}

export type DirectoryCreatePayload = {
  name: string;
  iconKey?: string;
  pendingImage?: string;
};

/**
 * DirectoryEntityList is the one shared implementation behind the
 * Members/Institutions/Groups tabs: create, rename, archive, optional
 * icon key, and native image pick (avatar/logo).
 */
export function DirectoryEntityList<T extends Entity>({
  entities,
  isLoading,
  isError,
  onCreate,
  onUpdate,
  onArchive,
  onSetImage,
  createLabel,
  emptyLabel,
  supportsIcon = false,
}: {
  entities: T[] | undefined;
  isLoading: boolean;
  isError: boolean;
  onCreate: (payload: DirectoryCreatePayload) => void;
  onUpdate: (id: string, name: string) => void;
  onArchive: (id: string, archived: boolean) => void;
  onSetImage: (id: string, pendingImage: string) => void;
  createLabel: string;
  emptyLabel: string;
  supportsIcon?: boolean;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [iconKey, setIconKey] = useState("");
  const [pendingImage, setPendingImage] = useState<string | undefined>(undefined);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editingName, setEditingName] = useState("");
  const [imageTargetId, setImageTargetId] = useState<string | null>(null);
  const [rowPendingImage, setRowPendingImage] = useState<string | undefined>(undefined);

  const submitCreate = (event: React.FormEvent) => {
    event.preventDefault();
    if (!name.trim()) {
      return;
    }
    onCreate({ name: name.trim(), iconKey: iconKey || undefined, pendingImage });
    setName("");
    setIconKey("");
    setPendingImage(undefined);
  };

  return (
    <div className="flex flex-col gap-4">
      <form onSubmit={submitCreate} className="flex flex-col gap-2" aria-label={createLabel}>
        <div className="flex items-center gap-2">
          <Input value={name} onChange={(event) => setName(event.target.value)} placeholder={createLabel} aria-label={t("accounts.name")} />
          <Button type="submit">
            <Plus className="size-4" /> {t("common.add")}
          </Button>
        </div>
        {supportsIcon && <IconPicker id={`${createLabel}-icon`} value={iconKey} onChange={setIconKey} />}
        <ImagePicker label={t("common.media")} value={pendingImage} onChange={setPendingImage} />
      </form>

      {isLoading && <p className="text-sm text-muted-foreground">{t("common.comingSoon")}</p>}
      {isError && (
        <p role="alert" className="text-sm text-destructive">
          {t("accounts.loadError")}
        </p>
      )}
      {entities && entities.length === 0 && <p className="text-sm text-muted-foreground">{emptyLabel}</p>}

      {entities && entities.length > 0 && (
        <ul className="flex flex-col gap-2">
          {entities.map((entity) => {
            const archived = Boolean(entity.archivedAt);
            const isEditing = editingId === entity.id;
            return (
              <li key={entity.id} className="flex flex-col gap-2 rounded-md border border-border px-3 py-2">
                <div className="flex items-center justify-between gap-2">
                  <span className="flex items-center gap-2 text-sm text-foreground">
                    {isEditing ? (
                      <Input
                        value={editingName}
                        onChange={(event) => setEditingName(event.target.value)}
                        aria-label={t("accounts.name")}
                      />
                    ) : (
                      entity.name
                    )}
                    {archived && <Badge variant="secondary">{t("common.archived")}</Badge>}
                  </span>
                  <div className="flex items-center gap-2">
                    {isEditing ? (
                      <Button
                        type="button"
                        size="sm"
                        onClick={() => {
                          if (editingName.trim()) {
                            onUpdate(entity.id, editingName.trim());
                            setEditingId(null);
                            toast.success(t("common.saved"));
                          }
                        }}
                      >
                        {t("common.save")}
                      </Button>
                    ) : (
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => {
                          setEditingId(entity.id);
                          setEditingName(entity.name);
                        }}
                      >
                        {t("common.edit")}
                      </Button>
                    )}
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        setImageTargetId(entity.id);
                        setRowPendingImage(undefined);
                      }}
                    >
                      {t("common.media")}
                    </Button>
                    <AlertDialog>
                      <AlertDialogTrigger className={cn(buttonVariants({ variant: "outline", size: "sm" }))}>
                        {archived ? t("common.active") : t("common.archive")}
                      </AlertDialogTrigger>
                      <AlertDialogContent>
                        <AlertDialogHeader>
                          <AlertDialogTitle>{archived ? t("common.active") : t("common.archive")}</AlertDialogTitle>
                          <AlertDialogDescription>{entity.name}</AlertDialogDescription>
                        </AlertDialogHeader>
                        <AlertDialogFooter>
                          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
                          <AlertDialogAction
                            onClick={() => {
                              onArchive(entity.id, !archived);
                              toast.success(archived ? t("common.active") : t("common.archive"));
                            }}
                          >
                            {archived ? t("common.active") : t("common.archive")}
                          </AlertDialogAction>
                        </AlertDialogFooter>
                      </AlertDialogContent>
                    </AlertDialog>
                  </div>
                </div>
                {imageTargetId === entity.id && (
                  <div className="flex items-end gap-2">
                    <ImagePicker
                      label={t("common.media")}
                      value={rowPendingImage}
                      existingAssetId={entity.logoAssetId ?? entity.avatarAssetId}
                      onChange={setRowPendingImage}
                    />
                    <Button
                      type="button"
                      size="sm"
                      disabled={!rowPendingImage}
                      onClick={() => {
                        if (rowPendingImage) {
                          onSetImage(entity.id, rowPendingImage);
                          setImageTargetId(null);
                          setRowPendingImage(undefined);
                          toast.success(t("common.saved"));
                        }
                      }}
                    >
                      {t("common.save")}
                    </Button>
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
