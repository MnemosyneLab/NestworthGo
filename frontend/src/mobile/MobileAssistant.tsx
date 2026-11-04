import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { AssistantConversation } from "@/features/assistant/AssistantConversation";
import { api, type Envelope } from "./api";

export function MobileAssistant({ blocked, disconnected }: { blocked: boolean; disconnected: boolean }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const status = useQuery({ queryKey: ["mobile", "assistant", "status"],
    queryFn: () => api<Envelope<unknown>>("/assistant/status"),
    enabled: open && !disconnected, retry: false, refetchOnWindowFocus: false });
  return <>
    <Button variant="outline" size="icon" className="mobile-icon-button" aria-label={t("assistant.open")} aria-expanded={open} aria-controls="mobile-assistant" disabled={blocked} onClick={() => setOpen(true)}><Sparkles aria-hidden /></Button>
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetContent id="mobile-assistant" className="flex h-full max-w-full flex-col overflow-hidden rounded-none p-0">
        <SheetHeader className="px-4 pt-4"><SheetTitle>{t("assistant.title")}</SheetTitle></SheetHeader>
        <AssistantConversation surface="mobile" status={status.data?.data} loading={status.isPending} disconnected={disconnected || status.isError} retry={() => void status.refetch()} />
      </SheetContent>
    </Sheet>
  </>;
}
