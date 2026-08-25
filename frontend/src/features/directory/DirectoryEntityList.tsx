import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Plus } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
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
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface Entity {
  id: string;
  name: string;
  archivedAt?: string | null;
}

/**
 * DirectoryEntityList is the one shared implementation behind the
 * Members/Institutions/Groups tabs (implementation plan Phase 5):
 * DirectoryService already treats all three as one CRUD/archive shape
 * (technical design Sec6), so the frontend does too rather than
 * tripling near-identical list/create/archive UI.
 */
export function DirectoryEntityList<T extends Entity>({
  entities,
  isLoading,
  isError,
  onCreate,
  onArchive,
  createLabel,
  emptyLabel,
}: {
  entities: T[] | undefined;
  isLoading: boolean;
  isError: boolean;
  onCreate: (name: string) => void;
  onArchive: (id: string, archived: boolean) => void;
  createLabel: string;
  emptyLabel: string;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");

  const submitCreate = (event: React.FormEvent) => {
    event.preventDefault();
    if (!name.trim()) {
      return;
    }
    onCreate(name.trim());
    setName("");
  };

  return (
    <div className="flex flex-col gap-4">
      <form onSubmit={submitCreate} className="flex items-center gap-2" aria-label={createLabel}>
        <Input value={name} onChange={(event) => setName(event.target.value)} placeholder={createLabel} aria-label={t("accounts.name")} />
        <Button type="submit">
          <Plus className="size-4" /> {t("common.add")}
        </Button>
      </form>

      {isLoading && <p className="text-sm text-muted-foreground">{t("common.comingSoon")}</p>}
      {isError && <p role="alert" className="text-sm text-destructive">{t("accounts.loadError")}</p>}
      {entities && entities.length === 0 && <p className="text-sm text-muted-foreground">{emptyLabel}</p>}

      {entities && entities.length > 0 && (
        <ul className="flex flex-col gap-2">
          {entities.map((entity) => {
            const archived = Boolean(entity.archivedAt);
            return (
              <li key={entity.id} className="flex items-center justify-between rounded-md border border-border px-3 py-2">
                <span className="flex items-center gap-2 text-sm text-foreground">
                  {entity.name}
                  {archived && <Badge variant="secondary">{t("common.archived")}</Badge>}
                </span>
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
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
