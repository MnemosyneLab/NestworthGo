import { BusinessPageRoot } from "@/lib/businessCapture";
import type { ReactNode } from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, vi, type Mock } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "./queryClient";
import { AssistantPageProvider } from "@/features/assistant/PageContext";
import { ModelConversation } from "@/features/assistant/ModelConversation";
import { PageChromeProvider } from "@/components/layout/PageChrome";

type APIs = Record<"Status" | "PendingPageReads" | "PublishPage" | "List" | "Inspect" | "Start" | "ConsumeNavigation", Mock>;
/** Exercise the real page with deferred desktop IPC, without a provider or ledger. */
export async function verifyPageNavigation({api, page, viewKey, phase, ready, change}: {
 api: APIs; page: ReactNode; viewKey: string;
 phase: "before-consume" | "during-consume" | "unchanged";
 ready: () => Promise<unknown>; change: () => Promise<unknown>;
}) {
 const prior={id:"prior",state:"succeeded",text:"Synthetic prior answer",prompt:"Earlier",model:"mock",created:"2026-11-05T00:00:00Z"};
 api.Status.mockResolvedValue({epoch:"host",revision:1,available:true,config:{enabled:true,model:"mock"}});
 api.PendingPageReads.mockResolvedValue([]); api.PublishPage.mockResolvedValue({id:"lease"});
 api.List.mockResolvedValue([{id:"conversation",runCount:1}]); api.Inspect.mockResolvedValue({id:"conversation",runs:[prior],runCount:1,offset:0});
 let resolve!: (target:{kind:string;accountId:string})=>void; let completed: unknown;
 api.ConsumeNavigation.mockClear();
 api.ConsumeNavigation.mockImplementation(()=>new Promise(done=>{resolve=done;}));
 api.Start.mockImplementation(async q=>{
  const run={...prior,id:"run",requestId:q.requestId,prompt:q.prompt,text:"Requested"};
  completed={id:"conversation",runCount:2,offset:1,runs:[{...run,state:"succeeded"}],navigation:{id:"intent",leaseId:q.pageLeaseId,requestId:q.requestId}};
  api.Inspect.mockResolvedValue(phase==="before-consume" ? {id:"conversation",runCount:2,offset:1,runs:[{...run,state:"running"}]} : completed);
  return {state:"accepted"};
 });
 const navigate=vi.fn(); const client=createTestQueryClient(); const toolbar=document.createElement("div"); document.body.append(toolbar);
 const user=userEvent.setup();
 const rendered=render(<QueryClientProvider client={client}><PageChromeProvider activePageId={viewKey.split(":")[0]} target={toolbar}><AssistantPageProvider householdId="synthetic" incarnation="synthetic" surface="desktop" viewKey={viewKey} blocked={()=>false} onNavigate={navigate}><BusinessPageRoot view={viewKey} accessIdentity="synthetic">{page}</BusinessPageRoot><ModelConversation /></AssistantPageProvider></PageChromeProvider></QueryClientProvider>);
 try {
  await ready(); await screen.findByText("Synthetic prior answer");
  await user.type(screen.getByLabelText("Message"),"Open overview");
  await user.click(screen.getByRole("button",{name:"Send to model"}));
  if(phase!=="before-consume") await waitFor(()=>expect(api.ConsumeNavigation).toHaveBeenCalledTimes(1));
  else await screen.findByText(/Running · reading related data/);
  if(phase!=="unchanged") await change();
  if(phase!=="before-consume") await act(async()=>resolve({kind:"overview",accountId:""}));
  else {api.Inspect.mockResolvedValue(completed); await act(async()=>{await client.invalidateQueries({queryKey:["aiConversation","host"]});});}
  if(phase==="unchanged") await waitFor(()=>expect(navigate).toHaveBeenCalledWith({page:"overview"}));
  else {await screen.findByText(/No page change was applied/); expect(navigate).not.toHaveBeenCalled();}
  if(phase==="before-consume") expect(api.ConsumeNavigation).not.toHaveBeenCalled();
 } finally { rendered.unmount(); toolbar.remove(); }
}
