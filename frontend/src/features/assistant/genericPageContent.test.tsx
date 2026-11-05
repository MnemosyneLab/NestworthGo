import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { PageChrome, PageChromeProvider } from "@/components/layout/PageChrome";
import { SheetTrigger } from "@/components/ui/sheet";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { BusinessPageRoot, SecretRegion } from "@/lib/businessCapture";
import { SecretField } from "@/features/settings/SecretField";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { ActionMenu } from "@/components/ui/action-menu";
import { AlertDialog, AlertDialogContent, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { capturePageContent } from "./pageContent";

it("defaults to an unregistered business page with ordinary financial fields and automatic modules", async()=>{
 render(<><aside>assistant transcript outside</aside><BusinessPageRoot view="future-business:object-one" accessIdentity="household-a"><section aria-label="Future balances"><h2>Exact draft balances</h2><label htmlFor="new-financial-field">Amount</label><input id="new-financial-field" defaultValue="123.4500"/><label htmlFor="new-note">Ledger note</label><textarea id="new-note" defaultValue="Ordinary financial note"/><label htmlFor="choice">Account</label><select id="choice"><option value="internal-account-uuid">Synthetic brokerage</option></select><button disabled>Record financial change</button></section><Card><CardHeader><CardTitle>Future cash module</CardTitle></CardHeader><CardContent>Ordinary forecast</CardContent></Card></BusinessPageRoot></>);
 const before=capturePageContent("future-business:object-one"); const raw=JSON.stringify(before);
 expect(before.nodes.some(n=>n.role==="region" && n.name==="Future cash module" && n.module!=="page")).toBe(true); expect(before.page).toBe("future-business"); expect(before.nodes.some(n=>n.role==="region" && n.name==="Future balances" && n.module!=="page")).toBe(true);
 expect(before.nodes.find(n=>n.name==="Amount" && n.role==="textbox")).toMatchObject({role:"textbox",value:"123.4500"});
 expect(before.nodes.find(n=>n.name==="Ledger note" && n.role==="textbox")).toMatchObject({role:"textbox",value:"Ordinary financial note"});
 expect(raw).toContain("Synthetic brokerage"); expect(raw).not.toContain("internal-account-uuid"); expect(raw).not.toContain("assistant transcript outside"); expect(raw).not.toContain("<main");
 expect(before.nodes.find(n=>n.name==="Record financial change")?.state).toContain("disabled=true");
 await userEvent.type(screen.getByLabelText("Amount"),"1"); expect(capturePageContent("future-business:object-one").id).not.toBe(before.id);
});

it("fences view, object and workspace identity while keeping the assistant-occluded business surface readable",()=>{
 const content=(view:string,accessIdentity:string,occluded=false)=><BusinessPageRoot view={view} accessIdentity={accessIdentity} occluded={occluded}><p>Same financial facts</p><div hidden>retained inactive ledger</div></BusinessPageRoot>;
 const rendered=render(content("new-view:object-one","household-a")); const first=capturePageContent("new-view:object-one");
 expect(capturePageContent("new-view:object-two").page).toBe("unsupported");
 const current=capturePageContent("new-view:object-one"); rendered.rerender(content("new-view:object-one","household-b"));
 const switched=capturePageContent("new-view:object-one"); expect(switched.id).not.toBe(current.id); expect(switched.id).not.toBe(first.id);
 rendered.rerender(content("new-view:object-two","household-b")); expect(capturePageContent("new-view:object-one").page).toBe("unsupported");
 rendered.rerender(content("new-view:object-two","household-b",true)); const occluded=capturePageContent("new-view:object-two");
 expect(occluded.page).toBe("new-view"); expect(JSON.stringify(occluded)).toContain("Same financial facts"); expect(JSON.stringify(occluded)).not.toContain("retained inactive ledger"); expect(occluded.limitations.join(" ")).toContain("occluded");
});

it("keeps shared secrets private after reveal and blocks indirect accessible-name references", async()=>{
 render(<BusinessPageRoot view="settings:" accessIdentity="synthetic"><SecretField id="provider-key" label="Provider key" hasValue={false} value="synthetic-secret-canary" onChange={vi.fn()}/><SecretRegion><span id="private-label">private-reference-canary</span><textarea readOnly value={'{"token":"synthetic-mcp-canary"}'}/></SecretRegion><button aria-labelledby="private-label">Public action</button><textarea readOnly aria-label="Financial memo" defaultValue="Read ordinary financial notes"/></BusinessPageRoot>);
 const user=userEvent.setup(); const check=()=>{const raw=JSON.stringify(capturePageContent("settings:")); for(const value of ["synthetic-secret-canary","private-reference-canary","synthetic-mcp-canary"]) expect(raw).not.toContain(value); expect(raw).toContain("Read ordinary financial notes");};
 check(); await user.click(screen.getByRole("button",{name:"Show new input"})); expect(screen.getByLabelText("Provider key")).toHaveAttribute("type","text"); check();
 await user.click(screen.getByRole("button",{name:"Hide new input"})); check();
});

it("reads real owned nested portals", async()=>{
 render(<BusinessPageRoot view="future-view:" accessIdentity="synthetic"><Sheet><SheetTrigger>Open business draft</SheetTrigger><SheetContent><SheetHeader><SheetTitle>Business draft</SheetTitle></SheetHeader><label htmlFor="draft-amount">Draft amount</label><input id="draft-amount" defaultValue="98.7600"/><Popover><PopoverTrigger>Business explanation</PopoverTrigger><PopoverContent><p>Nested financial explanation</p><textarea aria-label="Draft note" defaultValue="Readable business note"/></PopoverContent></Popover></SheetContent></Sheet></BusinessPageRoot>);
 await userEvent.click(screen.getByRole("button",{name:"Open business draft"}));
 await screen.findByText("Business draft");
 expect(JSON.stringify(capturePageContent("future-view:"))).toContain("98.7600");
 await userEvent.click(screen.getByRole("button",{name:"Business explanation"}));
 await screen.findByText("Nested financial explanation");
 await waitFor(()=>expect(JSON.stringify(capturePageContent("future-view:"))).toContain("Readable business note"));
 expect(capturePageContent("future-view:").nodes.some(n=>n.role==="dialog")).toBe(true);
});

it("rejects hidden-page, private and assistant portal sources", async()=>{
 render(<><Sheet open><SheetContent><SheetTitle>Assistant private portal</SheetTitle><p>outside-owner-canary</p></SheetContent></Sheet><BusinessPageRoot view="future-view:" accessIdentity="synthetic"><div hidden><Sheet open><SheetContent><SheetTitle>Inactive page portal</SheetTitle><p>hidden-route-canary</p></SheetContent></Sheet></div><SecretRegion><Popover open><PopoverTrigger>Private control</PopoverTrigger><PopoverContent>private-popup-canary</PopoverContent></Popover></SecretRegion></BusinessPageRoot></>);
 await screen.findByText("private-popup-canary");
 const raw=JSON.stringify(capturePageContent("future-view:"));
 for(const value of ["outside-owner-canary","hidden-route-canary","private-popup-canary"]) expect(raw).not.toContain(value);
});

it("captures shared menu and confirmation semantics without running their actions", async()=>{
 const action=vi.fn(); const view=render(<BusinessPageRoot view="future-view:" accessIdentity="synthetic"><ActionMenu label="Business actions" items={[{id:"record",label:"Record synthetic amount",onSelect:action}]}/></BusinessPageRoot>);
 await userEvent.click(screen.getByRole("button",{name:"Business actions"}));
 await screen.findByRole("menuitem",{name:"Record synthetic amount"});
 const menu=capturePageContent("future-view:");expect(menu.nodes.some(n=>n.role==="menuitem" && n.name==="Record synthetic amount")).toBe(true);
 expect(action).not.toHaveBeenCalled();
 view.rerender(<BusinessPageRoot view="future-view:" accessIdentity="synthetic"><AlertDialog open><AlertDialogContent><AlertDialogTitle>Review synthetic change</AlertDialogTitle><p>No change has been committed</p></AlertDialogContent></AlertDialog></BusinessPageRoot>);
 await screen.findByText("Review synthetic change"); const confirmation=capturePageContent("future-view:");
 expect(confirmation.nodes.some(n=>n.role==="alertdialog")).toBe(true);expect(confirmation.id).not.toBe(menu.id);expect(action).not.toHaveBeenCalled();
});

it("keeps existing large-page budgets and rejects excessive structural depth",()=>{
 const view=render(<BusinessPageRoot view="future-view:" accessIdentity="synthetic"><p>{"x".repeat(512*1024+1)}</p></BusinessPageRoot>);
 expect(()=>capturePageContent("future-view:")).toThrow("page_content_budget");
 view.rerender(<BusinessPageRoot view="future-view:" accessIdentity="synthetic">{Array.from({length:9},(_,i)=><p key={i}>{"x".repeat(400000)}</p>)}</BusinessPageRoot>);
 expect(()=>capturePageContent("future-view:")).toThrow("page_content_budget");
 view.rerender(<BusinessPageRoot view="future-view:" accessIdentity="synthetic">{null}</BusinessPageRoot>);
 const root=document.querySelector('[data-assistant-root="business"]')!;
 root.replaceChildren(); let parent=root; for(let i=0;i<130;i++){const child=document.createElement("div");parent.append(child);parent=child;}
 expect(()=>capturePageContent("future-view:")).toThrow("page_content_budget");
});


it("follows actual page toolbar origins into its shared sheet without reading the shell",async()=>{
 const target=document.createElement("div");target.textContent="outside-shell-canary";document.body.append(target);
 const view=render(<PageChromeProvider activePageId="future-view" target={target}><BusinessPageRoot view="future-view:" accessIdentity="synthetic"><PageChrome pageId="future-view" title="Future business title" actions={<Sheet><SheetTrigger>Open business draft</SheetTrigger><SheetContent><SheetTitle>Owned toolbar draft</SheetTitle><input aria-label="Public draft value" defaultValue="123.4500"/></SheetContent></Sheet>}/></BusinessPageRoot></PageChromeProvider>);
 try {expect(JSON.stringify(capturePageContent("future-view:"))).toContain("Future business title");await userEvent.click(screen.getByRole("button",{name:"Open business draft"}));await screen.findByText("Owned toolbar draft");
 const page=capturePageContent("future-view:");const raw=JSON.stringify(page);expect(raw).toContain("123.4500");expect(raw).not.toContain("outside-shell-canary");
 }finally{view.unmount();target.remove();}
});


it.each(["中".repeat(666)+"ab","a".repeat(2048),"a".repeat(2049),"中".repeat(682)+"ab","中".repeat(1000),"🌏".repeat(513)])("bounds ordinary field values by UTF-8 bytes without losing the rest of the page (%#)",value=>{
 render(<BusinessPageRoot view="future-view:" accessIdentity="synthetic"><label htmlFor="ordinary-long-note">Financial note</label><textarea id="ordinary-long-note" defaultValue={value}/><p>Other financial content remains readable</p></BusinessPageRoot>);
 const page=capturePageContent("future-view:");const field=page.nodes.find(n=>n.name==="Financial note" && n.role==="textbox")!;
 expect(new TextEncoder().encode(field.value).length).toBeLessThanOrEqual(2048);
 expect(value.startsWith(field.value)).toBe(true);expect(field.value).not.toContain("\uFFFD");
 const truncated=new TextEncoder().encode(value).length>2048;
 expect(field.state.includes("value_truncated=true")).toBe(truncated);
 expect(JSON.stringify(page)).toContain("Other financial content remains readable");
 if(truncated)expect(page.limitations.join(" ")).toContain("2048 UTF-8 bytes");else expect(field.value).toBe(value);
});

it("revokes old snapshots when only a truncated financial note's tail changes",()=>{
 const initial="中".repeat(1000)+"old tail";
 render(<BusinessPageRoot view="future-view:" accessIdentity="synthetic"><textarea aria-label="Financial note" defaultValue={initial}/></BusinessPageRoot>);
 const before=capturePageContent("future-view:");expect(capturePageContent("future-view:").id).toBe(before.id);
 (screen.getByLabelText("Financial note") as HTMLTextAreaElement).value="中".repeat(1000)+"new tail";
 const after=capturePageContent("future-view:");expect(after.nodes).toEqual(before.nodes);expect(after.id).not.toBe(before.id);expect(capturePageContent("future-view:").id).toBe(after.id);
});


it.each([false,true])("inherits private selection labels through option/optgroup (%s)",group=>{
 render(<BusinessPageRoot view="future-view:" accessIdentity="synthetic"><select aria-label="Public selector" defaultValue="secret-id">{group ? <optgroup data-assistant-private="credential" label="private-group-canary"><option value="secret-id" label="private-option-canary"/></optgroup> : <option data-assistant-private="credential" value="secret-id" label="private-option-canary"/>}<option value="public-id" label="Ordinary public choice"/></select><p>Other public financial values</p></BusinessPageRoot>);
 const page=capturePageContent("future-view:");const raw=JSON.stringify(page);expect(raw).not.toContain("private-option-canary");expect(raw).not.toContain("private-group-canary");expect(raw).not.toContain("secret-id");expect(raw).toContain("Ordinary public choice");
 const select=page.nodes.find(n=>n.role==="combobox")!;expect(select.value).toBe("");expect(select.state).toContain("value_redacted=true");
});

it.each(["hidden","display","aria"])("rejects an owned portal inside a %s ancestor",kind=>{
 const view=render(<BusinessPageRoot view="future-view:" accessIdentity="synthetic"><p>Visible source</p></BusinessPageRoot>);
 const root=document.querySelector<HTMLElement>('[data-assistant-root="business"]')!;const owner=root.dataset.assistantOwner!;
 const origin=document.createElement("span");origin.dataset.assistantOrigin="synthetic-origin";origin.dataset.assistantOwner=owner;origin.setAttribute("aria-hidden","true");root.append(origin);
 const wrapper=document.createElement("div");const portal=document.createElement("div");portal.dataset.assistantPortal="synthetic-origin";portal.dataset.assistantOwner=owner;portal.setAttribute("role","dialog");portal.textContent="hidden-destination-canary";wrapper.append(portal);document.body.append(wrapper);
 try {const initial=capturePageContent("future-view:");expect(JSON.stringify(initial)).toContain("hidden-destination-canary");
 if(kind==="hidden")wrapper.hidden=true;else if(kind==="display")wrapper.style.display="none";else wrapper.setAttribute("aria-hidden","true");
 const after=capturePageContent("future-view:");expect(JSON.stringify(after)).not.toContain("hidden-destination-canary");expect(after.id).not.toBe(initial.id);
 }finally{wrapper.remove();view.unmount();}
});
