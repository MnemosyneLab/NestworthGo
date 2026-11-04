import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Maximize2, Minimize2, Sparkles, X } from "lucide-react";
import { Service } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/assistant";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { hasUnresolvedSheetConfirmation } from "@/components/ui/sheet-confirmation";
import { AssistantConversation } from "./AssistantConversation";

export function useDesktopAssistant() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const wide = useMediaQuery("(min-width: 1280px)");
  const triggerRef = useRef<HTMLButtonElement>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  useEffect(() => { if (open && wide) headingRef.current?.focus(); }, [open, wide]);
  const status = useQuery({ queryKey: ["assistant", "status"], queryFn: () => Service.Status(),
    enabled: open, retry: false, refetchOnWindowFocus: false });
  const conversation = <AssistantConversation surface="desktop" status={status.data} loading={status.isPending}
    disconnected={status.isError} retry={() => void status.refetch()} />;
  const trigger = <Button ref={triggerRef} variant={open ? "soft" : "outline"} aria-label={t("assistant.open")} aria-expanded={open} aria-controls="desktop-assistant"
    onClick={() => { if (!hasUnresolvedSheetConfirmation()) setOpen(value => !value); }}><Sparkles aria-hidden className="size-4" /><span className="hidden sm:inline">{t("assistant.title")}</span></Button>;
  const panel = wide ? <aside id="desktop-assistant" hidden={!open} aria-label={t("assistant.title")}
    className={`${open ? "flex" : "hidden"} min-h-0 min-w-0 shrink-0 flex-col border-l bg-card ${expanded ? "flex-1" : "w-[400px]"}`}>
    <header className="flex min-h-14 shrink-0 items-center gap-2 border-b px-4">
      <Sparkles aria-hidden className="size-5 text-primary" /><h2 ref={headingRef} tabIndex={-1} className="min-w-0 flex-1 font-semibold">{t("assistant.title")}</h2>
      <Button variant="ghost" size="icon" onClick={() => setExpanded(value => !value)} aria-label={t(expanded ? "assistant.compact" : "assistant.expand")}>{expanded ? <Minimize2 aria-hidden className="size-4" /> : <Maximize2 aria-hidden className="size-4" />}</Button>
      <Button variant="ghost" size="icon" onClick={() => { setOpen(false); triggerRef.current?.focus(); }} aria-label={t("assistant.collapse")}><X aria-hidden className="size-4" /></Button>
    </header>
    {conversation}
  </aside> : <Sheet open={open} onOpenChange={setOpen}>
    <SheetContent id="desktop-assistant" finalFocus={triggerRef} size="lg" className="flex flex-col overflow-hidden p-0">
      <SheetHeader className="px-4 pt-4"><SheetTitle>{t("assistant.title")}</SheetTitle></SheetHeader>
      {conversation}
    </SheetContent>
  </Sheet>;
  return { trigger, panel, expanded: wide && expanded && open };
}
