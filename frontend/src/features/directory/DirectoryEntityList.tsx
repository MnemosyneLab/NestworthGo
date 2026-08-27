import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Plus } from "lucide-react";
import { toast } from "sonner";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
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
import { displayError } from "@/lib/display";
import { IconPicker } from "@/components/forms/IconPicker";
import { ImagePicker } from "@/components/forms/ImagePicker";
import { ErrorState, EmptyState, LoadingState } from "@/components/layout/PageState";

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

export type DirectoryCreateResult = {
  mediaSaved: boolean;
};

/**
 * DirectoryEntityList is the shared create/edit/archive/image surface for
 * Members, Institutions, and Groups. Creation happens in a side sheet;
 * media is reported as a recoverable partial failure instead of becoming
 * an unhandled promise.
 */
export function DirectoryEntityList<T extends Entity>({
  entities,
  isLoading,
  isError,
  onCreate,
  onUpdate,
  onArchive,
  onSetImage,
  onRetry,
  entityLabel,
  createLabel,
  addLabel,
  emptyLabel,
  supportsIcon = false,
}: {
  entities: T[] | undefined;
  isLoading: boolean;
  isError: boolean;
  onCreate: (payload: DirectoryCreatePayload) => Promise<DirectoryCreateResult>;
  onUpdate: (id: string, name: string) => Promise<unknown>;
  onArchive: (id: string, archived: boolean) => Promise<unknown>;
  onSetImage: (id: string, pendingImage: string) => Promise<unknown>;
  onRetry: () => void | Promise<unknown>;
  entityLabel: string;
  createLabel: string;
  addLabel: string;
  emptyLabel: string;
  supportsIcon?: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [iconKey, setIconKey] = useState("");
  const [pendingImage, setPendingImage] = useState<string | undefined>(undefined);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editingName, setEditingName] = useState("");
  const [imageTargetId, setImageTargetId] = useState<string | null>(null);
  const [rowPendingImage, setRowPendingImage] = useState<string | undefined>(undefined);
  const [submitting, setSubmitting] = useState(false);
  const [pendingActionId, setPendingActionId] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<string | undefined>();
  const [actionError, setActionError] = useState<string | undefined>();
  const [imageError, setImageError] = useState<{ id: string; message: string } | undefined>();

  const submitCreate = async (event: FormEvent) => {
    event.preventDefault();
    const trimmedName = name.trim();
    setSubmitError(undefined);
    if (!trimmedName) {
      setSubmitError(t("directory.nameRequired"));
      return;
    }
    setSubmitting(true);
    try {
      const result = await onCreate({ name: trimmedName, iconKey: iconKey || undefined, pendingImage });
      setName("");
      setIconKey("");
      setPendingImage(undefined);
      setOpen(false);
      if (result.mediaSaved) {
        toast.success(t("directory.entityCreated", { name: trimmedName }));
      } else {
        const message = t("directory.entityCreatedMediaError", { name: trimmedName });
        setActionError(message);
        toast.error(message);
      }
    } catch (error) {
      setSubmitError(displayError(error, t("directory.createError")));
    } finally {
      setSubmitting(false);
    }
  };

  const saveRename = async (entity: T) => {
    const trimmedName = editingName.trim();
    if (!trimmedName) {
      setActionError(t("directory.nameRequired"));
      return;
    }
    setActionError(undefined);
    setPendingActionId(entity.id);
    try {
      await onUpdate(entity.id, trimmedName);
      setEditingId(null);
      toast.success(t("common.saved"));
    } catch (error) {
      setActionError(displayError(error, t("directory.updateError")));
    } finally {
      setPendingActionId(null);
    }
  };

  const saveImage = async (entity: T) => {
    if (!rowPendingImage) {
      return;
    }
    setImageError(undefined);
    setPendingActionId(entity.id);
    try {
      await onSetImage(entity.id, rowPendingImage);
      setImageTargetId(null);
      setRowPendingImage(undefined);
      toast.success(t("common.saved"));
    } catch (error) {
      setImageError({ id: entity.id, message: displayError(error, t("directory.imageSaveError", { entity: entityLabel })) });
    } finally {
      setPendingActionId(null);
    }
  };

  const archive = async (entity: T, archived: boolean) => {
    setActionError(undefined);
    setPendingActionId(entity.id);
    try {
      await onArchive(entity.id, archived);
      toast.success(archived ? t("common.archived") : t("common.active"));
    } catch (error) {
      setActionError(displayError(error, t("directory.updateError")));
    } finally {
      setPendingActionId(null);
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <div>
        <Sheet open={open} onOpenChange={setOpen}>
          <SheetTrigger className={buttonVariants({})}>
            <Plus className="size-4" aria-hidden="true" /> {addLabel}
          </SheetTrigger>
          <SheetContent>
            <SheetHeader>
              <SheetTitle>{addLabel}</SheetTitle>
            </SheetHeader>
            <form onSubmit={submitCreate} className="flex flex-col gap-4" aria-label={createLabel}>
              <Input
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder={createLabel}
                aria-label={createLabel}
                disabled={submitting}
              />
              {supportsIcon && <IconPicker id={`${createLabel}-icon`} value={iconKey} onChange={setIconKey} />}
              <ImagePicker label={t("common.media")} value={pendingImage} onChange={setPendingImage} />
              {submitError && (
                <p role="alert" className="text-sm text-destructive">
                  {submitError}
                </p>
              )}
              <Button type="submit" disabled={submitting}>
                {submitting ? t("common.pending") : t("common.add")}
              </Button>
            </form>
          </SheetContent>
        </Sheet>
      </div>

      {isLoading && <LoadingState label={t("ui.state.loadingPage")} />}
      {isError && (
        <ErrorState
          title={t("directory.loadError")}
          description={t("ui.state.errorDescription")}
          onRetry={onRetry}
          retryLabel={t("common.retryAction")}
        />
      )}
      {actionError && (
        <p role="alert" className="text-sm text-warning-foreground">
          {actionError}
        </p>
      )}

      {!isLoading && !isError && entities && entities.length === 0 && <EmptyState title={t("directory.emptyTitle")} description={emptyLabel} />}

      {!isLoading && !isError && entities && entities.length > 0 && (
        <ul className="flex flex-col gap-2">
          {entities.map((entity) => {
            const archived = Boolean(entity.archivedAt);
            const isEditing = editingId === entity.id;
            const isPending = pendingActionId === entity.id;
            return (
              <li key={entity.id} className="flex flex-col gap-2 rounded-md border border-border px-3 py-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="flex min-w-0 flex-1 items-center gap-2 text-sm text-foreground">
                    {isEditing ? (
                      <Input
                        value={editingName}
                        onChange={(event) => setEditingName(event.target.value)}
                        aria-label={t("accounts.name")}
                        disabled={isPending}
                        className="max-w-xs"
                      />
                    ) : (
                      entity.name
                    )}
                    {archived && <Badge variant="secondary">{t("common.archived")}</Badge>}
                  </span>
                  <div className="flex flex-wrap items-center gap-2">
                    {isEditing ? (
                      <Button type="button" size="sm" onClick={() => void saveRename(entity)} disabled={isPending}>
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
                          setActionError(undefined);
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
                        setImageError(undefined);
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
                          <AlertDialogAction onClick={() => void archive(entity, !archived)} disabled={isPending}>
                            {archived ? t("common.active") : t("common.archive")}
                          </AlertDialogAction>
                        </AlertDialogFooter>
                      </AlertDialogContent>
                    </AlertDialog>
                  </div>
                </div>
                {imageTargetId === entity.id && (
                  <div className="flex flex-col gap-2 rounded-md bg-muted/40 p-3 sm:flex-row sm:items-end sm:justify-between">
                    <ImagePicker
                      label={t("common.media")}
                      value={rowPendingImage}
                      existingAssetId={entity.logoAssetId ?? entity.avatarAssetId}
                      onChange={setRowPendingImage}
                    />
                    <Button type="button" size="sm" disabled={!rowPendingImage || isPending} onClick={() => void saveImage(entity)}>
                      {isPending ? t("common.pending") : t("common.save")}
                    </Button>
                  </div>
                )}
                {imageError?.id === entity.id && (
                  <p role="alert" className="text-sm text-destructive">
                    {imageError.message} {t("directory.imageRetry")}
                  </p>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
