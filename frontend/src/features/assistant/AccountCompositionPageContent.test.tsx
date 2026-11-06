import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { AccountCompositionPanel } from "@/features/accounts/AccountCompositionPanel";
import { capturePageContent } from "./pageContent";
import { AssistantPageProvider } from "./PageContext";
import { ModelConversation } from "./ModelConversation";

const accountID = "00000000-0000-4000-8000-000000000001";
const viewKey = `accounts:${accountID}`;
// Empty composition keeps all rows, totals and labels identical between modes:
// only the live checked properties may change the semantic generation.
const composition = { currency: "USD", totalExact: "0", complete: true, status: "empty", valuedCount: 0, missingCount: 0, types: [], details: [], detailSlices: [] };
function RealComposition() {
 return <main data-assistant-page="account" data-assistant-view={viewKey}><section data-assistant-module="composition"><AccountCompositionPanel composition={composition} /></section></main>;
}
const api = vi.hoisted(() => ({Status: vi.fn(), PendingPageReads: vi.fn(), DeliverPageContent: vi.fn(), PublishPage: vi.fn(),  ConsumeNavigation: vi.fn(), List: vi.fn(), Inspect: vi.fn(), Messages: vi.fn(), Start: vi.fn(), Cancel: vi.fn(), Create: vi.fn(), Delete: vi.fn()}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service", () => ({ ...api, Usage: async () => (await import("@/test/aiUsage")).emptyUsageReport }));
const priorRun = {id:"prior",state:"succeeded",text:"Synthetic prior answer",prompt:"Earlier",model:"mock",created:"2026-11-05T00:00:00Z"};
beforeEach(() => {
 vi.clearAllMocks();
 api.Status.mockResolvedValue({epoch:"host",revision:1,available:true,config:{enabled:true,model:"mock"}});
 api.PendingPageReads.mockResolvedValue([]); api.PublishPage.mockResolvedValue({id:"lease"});
 api.List.mockResolvedValue([{id:"conversation",runCount:1}]); api.Inspect.mockResolvedValue({id:"conversation",runs:[priorRun],runCount:1,offset:0});
});
it("captures both real composition radios and changes the snapshot using checked state alone", async () => {
 render(<RealComposition />);
 const user=userEvent.setup();
 const before=capturePageContent(viewKey);
 expect(before.nodes.find(n=>n.id==="composition/account-composition-type")).toMatchObject({role:"radio",name:"By type",value:"By type",state:["checked=true"]});
 expect(before.nodes.find(n=>n.id==="composition/account-composition-detail")).toMatchObject({role:"radio",name:"By detail",value:"By detail",state:["checked=false"]});
 await user.click(screen.getByRole("radio",{name:"By detail"}));
 const after=capturePageContent(viewKey);
 expect(after.nodes.find(n=>n.id==="composition/account-composition-type")?.state).toEqual(["checked=false"]);
 expect(after.nodes.find(n=>n.id==="composition/account-composition-detail")?.state).toEqual(["checked=true"]);
 const withoutChecked=(page:typeof before)=>page.nodes.map(node=>({...node,state:node.state.filter(s=>!s.startsWith("checked="))}));
 expect(withoutChecked(after)).toEqual(withoutChecked(before));
 expect(after.id).not.toBe(before.id);
 expect(capturePageContent(viewKey).id).toBe(after.id);
});
it.each(["before-consume","during-consume","unchanged"])("rejects navigation when real account composition changes %s", async phase => {
 let resolve!: (target:{kind:string;accountId:string})=>void;
 let completed: unknown;
 api.ConsumeNavigation.mockImplementation(()=>new Promise(done=>{resolve=done;}));
 api.Start.mockImplementation(async q=>{
  const run={...priorRun,id:"run",requestId:q.requestId,prompt:q.prompt,text:"Requested"};
  completed={id:"conversation",runCount:2,offset:1,runs:[{...run,state:"succeeded"}],navigation:{id:"intent",leaseId:q.pageLeaseId,requestId:q.requestId}};
  api.Inspect.mockResolvedValue(phase==="before-consume" ? {id:"conversation",runCount:2,offset:1,runs:[{...run,state:"running"}]} : completed);
  return {state:"accepted"};
 });
 const navigate=vi.fn(); const client=createTestQueryClient();
 render(<QueryClientProvider client={client}><AssistantPageProvider householdId="synthetic" incarnation="synthetic" surface="desktop" viewKey={viewKey} blocked={()=>false} onNavigate={navigate}><RealComposition /><ModelConversation /></AssistantPageProvider></QueryClientProvider>);
 const user=userEvent.setup(); await screen.findByText("Synthetic prior answer");
 await user.type(screen.getByLabelText("Message"),"Open overview");
 await user.click(screen.getByRole("button",{name:"Send to model"}));
 if(phase!=="before-consume") await waitFor(()=>expect(api.ConsumeNavigation).toHaveBeenCalledTimes(1));
 else await screen.findByText(/Running · reading related data/);
 if(phase!=="unchanged") await user.click(screen.getByRole("radio",{name:"By detail"}));
 if(phase!=="before-consume") await act(async()=>resolve({kind:"overview",accountId:""}));
 else {
  api.Inspect.mockResolvedValue(completed);
  await act(async()=>{await client.invalidateQueries({queryKey:["aiConversation","host"]});});
 }
 if(phase==="unchanged") await waitFor(()=>expect(navigate).toHaveBeenCalledWith({page:"overview"}));
 else { await screen.findByText(/No page change was applied/); expect(navigate).not.toHaveBeenCalled(); }
 if(phase==="before-consume") expect(api.ConsumeNavigation).not.toHaveBeenCalled();
});
