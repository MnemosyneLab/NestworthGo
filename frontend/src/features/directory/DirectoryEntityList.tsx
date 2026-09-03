import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Plus } from "lucide-react";
import { toast } from "sonner";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog";
import { cn } from "@/lib/utils";
import { displayEnum, displayError } from "@/lib/display";
import { IconPicker } from "@/components/forms/IconPicker";
import { EntityIcon, type EntityIconKind } from "@/components/icons/EntityIcon";
import { ErrorState, EmptyState, LoadingState } from "@/components/layout/PageState";
import { DEFAULT_ICONS, INSTITUTION_TYPE_ICONS } from "@/lib/defaultIcons";

interface Entity { id: string; name: string; archivedAt?: string | null; iconKey: string; institutionType?: string; }
export type DirectoryCreatePayload = { name: string; iconKey: string; institutionType?: string };

export function DirectoryEntityList<T extends Entity>({ entities, isLoading, isError, onCreate, onUpdate, onArchive, onSetIcon, onRetry, addLabel, emptyLabel, kind, institutionTypes = [] }: {
  entities: T[] | undefined; isLoading: boolean; isError: boolean; onCreate: (payload: DirectoryCreatePayload) => Promise<unknown>;
  onUpdate: (id: string, name: string) => Promise<unknown>; onArchive: (id: string, archived: boolean) => Promise<unknown>;
  onSetIcon: (id: string, iconKey: string) => Promise<unknown>; onRetry: () => void | Promise<unknown>;
  addLabel: string; emptyLabel: string; kind: EntityIconKind; institutionTypes?: string[];
}) {
  const { t } = useTranslation();
  const fallback = DEFAULT_ICONS[kind];
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [institutionType, setInstitutionType] = useState(institutionTypes[0] ?? "");
  const [iconKey, setIconKey] = useState<string>(fallback);
  const [iconCustomized, setIconCustomized] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editingName, setEditingName] = useState("");
  const [editingIcon, setEditingIcon] = useState<string>(fallback);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string>();
  const selectedInstitutionType = institutionType || institutionTypes[0] || "";
  const createIconKey = iconCustomized || kind !== "institution"
    ? iconKey
    : INSTITUTION_TYPE_ICONS[selectedInstitutionType] ?? fallback;

  const submitCreate = async (event: FormEvent) => {
    event.preventDefault(); const trimmed = name.trim();
    if (!trimmed) { setError(t("directory.nameRequired")); return; }
    if (kind === "institution" && !selectedInstitutionType) { setError(t("directory.institutionTypeRequired")); return; }
    setSubmitting(true); setError(undefined);
    try {
      await onCreate({ name: trimmed, iconKey: createIconKey, institutionType: selectedInstitutionType || undefined });
      setName(""); setInstitutionType(""); setIconKey(fallback); setIconCustomized(false); setOpen(false);
      toast.success(t("directory.entityCreated", { name: trimmed }));
    } catch (cause) { setError(displayError(cause, t("directory.createError"))); }
    finally { setSubmitting(false); }
  };

  const save = async (entity: T) => {
    const trimmed = editingName.trim(); if (!trimmed) { setError(t("directory.nameRequired")); return; }
    setSubmitting(true); setError(undefined);
    try {
      await onUpdate(entity.id, trimmed);
      if (editingIcon !== entity.iconKey) await onSetIcon(entity.id, editingIcon);
      setEditingId(null); toast.success(t("common.saved"));
    } catch (cause) { setError(displayError(cause, t("directory.updateError"))); }
    finally { setSubmitting(false); }
  };

  return <div className="flex flex-col gap-4">
    <Sheet open={open} onOpenChange={setOpen}><SheetTrigger className={cn(buttonVariants({}), "w-fit self-start gap-2")}><Plus className="size-4" aria-hidden="true" /> {addLabel}</SheetTrigger>
      <SheetContent><SheetHeader><SheetTitle>{addLabel}</SheetTitle></SheetHeader><form onSubmit={submitCreate} className="flex flex-col gap-4" aria-label={addLabel}>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${kind}-create-name`}>{t(`directory.nameLabel.${kind}`)}</Label>
          <Input id={`${kind}-create-name`} value={name} onChange={(event) => setName(event.target.value)} placeholder={t(`directory.namePlaceholder.${kind}`)} disabled={submitting} />
        </div>
        {kind === "institution" && <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${kind}-create-type`}>{t("directory.institutionType")}</Label>
          <NativeSelect id={`${kind}-create-type`} value={selectedInstitutionType} aria-label={t("directory.institutionType")} onChange={(event) => { const next = event.target.value; setInstitutionType(next); if (!iconCustomized) setIconKey(INSTITUTION_TYPE_ICONS[next] ?? DEFAULT_ICONS.institution); }}>
          {institutionTypes.map((type) => <option key={type} value={type}>{displayEnum(t, "institutionType", type)}</option>)}</NativeSelect>
        </div>}
        <IconPicker id={`${kind}-create-icon`} value={createIconKey} kind={kind} onChange={(key) => { setIconKey(key); setIconCustomized(true); }} />
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <Button type="submit" disabled={submitting}>{submitting ? t("common.pending") : t("common.add")}</Button>
      </form></SheetContent>
    </Sheet>
    {isLoading && <LoadingState label={t("ui.state.loadingPage")} />}
    {isError && <ErrorState title={t("directory.loadError")} description={t("ui.state.errorDescription")} onRetry={onRetry} retryLabel={t("common.retryAction")} />}
    {!isLoading && !isError && entities?.length === 0 && <EmptyState title={t("directory.emptyTitle")} description={emptyLabel} />}
    {!isLoading && !isError && entities && entities.length > 0 && <ul className="flex flex-col gap-2">{entities.map((entity) => {
      const editing = editingId === entity.id; const archived = Boolean(entity.archivedAt);
      return <li key={entity.id} className="rounded-md border border-border px-3 py-3"><div className="flex flex-wrap items-center justify-between gap-3">
        <span className="flex min-w-0 items-center gap-2 text-sm"><EntityIcon iconKey={entity.iconKey} kind={kind} className="size-5 text-primary" />
          {editing ? <Input value={editingName} onChange={(event) => setEditingName(event.target.value)} className="max-w-xs" /> : <><span>{entity.name}</span>{entity.institutionType && <Badge variant="outline">{displayEnum(t, "institutionType", entity.institutionType)}</Badge>}</>}
          {archived && <Badge variant="secondary">{t("common.archived")}</Badge>}</span>
        <span className="flex gap-2">{editing ? <Button size="sm" onClick={() => void save(entity)} disabled={submitting}>{t("common.save")}</Button> : <Button variant="outline" size="sm" onClick={() => { setEditingId(entity.id); setEditingName(entity.name); setEditingIcon(entity.iconKey); }}>{t("common.edit")}</Button>}
          <AlertDialog><AlertDialogTrigger className={cn(buttonVariants({ variant: "outline", size: "sm" }))}>{archived ? t("common.active") : t("common.archive")}</AlertDialogTrigger><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{archived ? t("common.active") : t("common.archive")}</AlertDialogTitle><AlertDialogDescription>{entity.name}</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel><AlertDialogAction onClick={() => void onArchive(entity.id, !archived)}>{archived ? t("common.active") : t("common.archive")}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
        </span></div>{editing && <div className="mt-3"><IconPicker id={`${kind}-${entity.id}-icon`} value={editingIcon} kind={kind} onChange={setEditingIcon} /></div>}</li>;
    })}</ul>}
    {error && !open && <p role="alert" className="text-sm text-destructive">{error}</p>}
  </div>;
}
