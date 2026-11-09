import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, ChevronsUpDown, MessageSquare, Pencil, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { cn } from "@/lib/utils";

/** `title` is the display label (may be a fallback ordinal); `savedTitle` is the stored one, empty when untitled. */
export type SwitcherConversation = { id: string; title: string; savedTitle: string; updated: string };
const MAX_TITLE_CODE_POINTS = 64;

/** Display-only relative time; the ledger timezone is irrelevant for chat recency. */
export function relativeUpdated(value: string, language: string, now = Date.now()) {
  const time = Date.parse(value);
  if (!Number.isFinite(time)) return "";
  const seconds = Math.round((time - now) / 1000);
  const formatter = new Intl.RelativeTimeFormat(language, { numeric: "auto" });
  const minutes = Math.round(seconds / 60);
  if (Math.abs(minutes) < 60) return formatter.format(minutes, "minute");
  const hours = Math.round(minutes / 60);
  if (Math.abs(hours) < 24) return formatter.format(hours, "hour");
  const days = Math.round(hours / 24);
  if (Math.abs(days) < 7) return formatter.format(days, "day");
  return new Intl.DateTimeFormat(language, { dateStyle: "medium" }).format(time);
}

type GroupKey = "groupToday" | "groupYesterday" | "groupEarlier";
export function groupKey(value: string, now = new Date()): GroupKey {
  const time = Date.parse(value);
  if (!Number.isFinite(time)) return "groupEarlier";
  const start = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  if (time >= start) return "groupToday";
  const yesterday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1).getTime();
  if (time >= yesterday) return "groupYesterday";
  return "groupEarlier";
}

/** Conversation switcher with per-row rename and confirmed delete. It owns only
 * presentation state; every mutation goes through the owner's admission guard. */
export function ConversationSwitcher({ conversations, currentId, disabled, mutationDisabled, onSelect, onCreate, createDisabled, onRename, onDelete }: {
  conversations: SwitcherConversation[]; currentId: string;
  /** Selecting is blocked while an action is in flight. */
  disabled: boolean;
  /** Rename and delete are also blocked by a running model turn or stale state. */
  mutationDisabled: boolean;
  onSelect: (id: string) => void;
  onCreate: () => void; createDisabled: boolean;
  /** Resolves true only after the rename was stored. */
  onRename: (id: string, title: string) => Promise<boolean>;
  onDelete: (id: string) => void;
}) {
  const { t, i18n } = useTranslation();
  const [open, setOpen] = useState(false);
  const [rename, setRename] = useState<{ id: string; title: string } | null>(null);
  const [pendingDelete, setPendingDelete] = useState<SwitcherConversation | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const current = conversations.find(item => item.id === currentId);
  const currentTitle = current?.title ?? t("aiChat.history");
  const groups = (["groupToday", "groupYesterday", "groupEarlier"] as const).map(key => ({
    key,
    items: conversations.filter(item => groupKey(item.updated) === key),
  })).filter(group => group.items.length);
  const touchTarget = "min-h-11 min-w-11 xl:min-h-8 xl:min-w-8";
  return <div className="flex shrink-0 items-center gap-2 border-b border-border px-3 py-2">
    <Popover open={open} onOpenChange={next => { setOpen(next); if (!next) setRename(null); }}>
      <PopoverTrigger ref={triggerRef} disabled={disabled || !conversations.length} aria-label={current ? `${t("aiChat.history")}: ${currentTitle}` : t("aiChat.history")}
        className="flex min-h-11 min-w-0 flex-1 items-center gap-2 rounded-lg border border-border bg-card px-3 text-left text-sm shadow-xs transition-[border-color,box-shadow] hover:border-foreground/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 disabled:cursor-not-allowed disabled:opacity-50 xl:min-h-9">
        <MessageSquare aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1">
          <span className="block truncate font-medium">{currentTitle}</span>
          {current?.updated && <span className="block truncate text-xs text-muted-foreground">{relativeUpdated(current.updated, i18n.language)}</span>}
        </span>
        <ChevronsUpDown aria-hidden="true" className="size-3.5 shrink-0 text-muted-foreground" />
      </PopoverTrigger>
      <PopoverContent aria-label={t("aiChat.history")} className="w-[min(22rem,calc(100vw-2rem))] max-h-[min(28rem,70dvh)] overflow-y-auto p-1">
        {groups.map(group => <section key={group.key} aria-label={t(`aiChat.${group.key}`)}>
          <h3 className="px-3 pb-1 pt-2 text-xs font-medium text-muted-foreground">{t(`aiChat.${group.key}`)}</h3>
          <ul className="space-y-0.5">
            {group.items.map(item => {
              const active = item.id === currentId;
              const editing = rename?.id === item.id;
              return <li key={item.id} className={cn("group flex items-center gap-1 rounded-lg px-1", active ? "bg-primary/10" : "hover:bg-muted")}>
                {editing ? <form className="flex min-w-0 flex-1 flex-col gap-2 p-2" onSubmit={event => {
                  event.preventDefault();
                  const draft = rename;
                  if (!draft.title.trim() || Array.from(draft.title.trim()).length > MAX_TITLE_CODE_POINTS) return;
                  void onRename(draft.id, draft.title).then(stored => { if (stored) setRename(null); });
                }}>
                  <Label htmlFor="ai-conversation-title">{t("aiChat.titleLabel")}</Label>
                  {/* Native maxlength counts UTF-16 units; 128 allows 64 supplementary code points. */}
                  <Input id="ai-conversation-title" autoFocus value={rename.title} maxLength={128} disabled={disabled}
                    onChange={event => setRename({ id: item.id, title: Array.from(event.target.value).slice(0, MAX_TITLE_CODE_POINTS).join("") })}
                    onKeyDown={event => {
                      if (event.key === "Escape") { event.stopPropagation(); setRename(null); }
                      if (event.key === "Enter" && event.nativeEvent.isComposing) event.preventDefault();
                    }} />
                  <div className="flex gap-2">
                    <Button type="submit" size="sm" disabled={disabled || mutationDisabled || !rename.title.trim()}>{t("aiChat.saveTitle")}</Button>
                    <Button type="button" size="sm" variant="ghost" disabled={disabled} onClick={() => setRename(null)}>{t("aiChat.cancelTitle")}</Button>
                  </div>
                </form> : <>
                  <button type="button" aria-current={active ? "true" : undefined} disabled={disabled}
                    className="flex min-h-11 min-w-0 flex-1 items-center gap-2 rounded-lg px-2 text-left text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 disabled:opacity-50 xl:min-h-10"
                    onClick={() => { setOpen(false); if (!active) onSelect(item.id); }}>
                    <Check aria-hidden="true" className={cn("size-4 shrink-0 text-primary", !active && "invisible")} />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate">{item.title}</span>
                      {item.updated && <span className="block truncate text-xs text-muted-foreground">{relativeUpdated(item.updated, i18n.language)}</span>}
                    </span>
                  </button>
                  <Button type="button" size="icon" variant="ghost" className={cn(touchTarget, "xl:opacity-0 xl:group-focus-within:opacity-100 xl:group-hover:opacity-100")}
                    aria-label={`${t("aiChat.rename")}: ${item.title}`} title={t("aiChat.rename")} disabled={disabled || mutationDisabled}
                    onClick={() => setRename({ id: item.id, title: item.savedTitle })}><Pencil aria-hidden="true" className="size-4" /></Button>
                  <Button type="button" size="icon" variant="ghost" className={cn(touchTarget, "xl:opacity-0 xl:group-focus-within:opacity-100 xl:group-hover:opacity-100")}
                    aria-label={`${t("aiChat.delete")}: ${item.title}`} title={t("aiChat.delete")} disabled={disabled || mutationDisabled}
                    onClick={() => { setOpen(false); setPendingDelete(item); }}><Trash2 aria-hidden="true" className="size-4" /></Button>
                </>}
              </li>;
            })}
          </ul>
        </section>)}
      </PopoverContent>
    </Popover>
    <Button type="button" variant="outline" size="icon" className={touchTarget} aria-label={t("aiChat.new")} title={t("aiChat.new")}
      disabled={createDisabled} onClick={onCreate}><Plus aria-hidden="true" className="size-4" /></Button>
    <AlertDialog open={!!pendingDelete} onOpenChange={next => { if (!next) setPendingDelete(null); }}>
      <AlertDialogContent finalFocus={triggerRef}>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("aiChat.deleteTitle")}</AlertDialogTitle>
          <AlertDialogDescription>{t("aiChat.deleteDescription", { title: pendingDelete?.title ?? "" })}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("aiChat.deleteCancel")}</AlertDialogCancel>
          <AlertDialogAction disabled={disabled || mutationDisabled} onClick={() => { if (pendingDelete && !disabled && !mutationDisabled) onDelete(pendingDelete.id); }}>{t("aiChat.deleteConfirm")}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>;
}
