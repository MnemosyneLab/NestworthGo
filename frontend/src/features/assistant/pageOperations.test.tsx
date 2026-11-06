import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { act, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { BusinessPageRoot, SecretRegion, SecurityOperationRegion } from "@/lib/businessCapture";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { capturePageContent } from "./pageContent";
import { operatePageControl, type PageOperation } from "./pageOperations";

const view="future-view:";
function operation(name:string,action="activate",extra:Partial<PageOperation>={}):PageOperation {
 const p=capturePageContent(view,"generation");
 return {snapshotId:p.id,targetRef:p.nodes.find(n=>n.name===name && n.targetRef)!.targetRef!,action,...extra};
}
const execute=(op:PageOperation,full=true,blocked=false)=>operatePageControl(view,"generation",op,()=>!full || blocked ? "blocked" : undefined);
it("uses real shared React handlers and exact option identities without exposing internal values",()=>{
 function Form(){const [amount,setAmount]=useState("1.0000");const [choice,setChoice]=useState("a");return <BusinessPageRoot view={view} accessIdentity="synthetic"><Input aria-label="Exact amount" value={amount} onChange={e=>setAmount(e.target.value)}/><NativeSelect aria-label="Account" value={choice} onChange={e=>setChoice(e.target.value)}><option value="a">Synthetic A</option><option value="b">Synthetic B</option></NativeSelect><p>{amount} {choice}</p></BusinessPageRoot>;}
 render(<Form/>);
 let result;
 act(()=>{result=execute(operation("Exact amount","set_value",{value:"123.4500"}));});
 expect(result).toMatchObject({state:"dispatched"});expect(screen.getByText("123.4500 a")).toBeInTheDocument();
 const page=capturePageContent(view,"generation");
 act(()=>{result=execute(operation("Account","select_option",{optionRef:page.nodes.find(n=>n.name==="Synthetic B")!.targetRef}));});
 expect(result).toMatchObject({state:"dispatched"});expect(screen.getByText("123.4500 b")).toBeInTheDocument();
});
it("rejects ReadOnly, unresolved flow and readonly/disabled/busy controls",()=>{
 const click=vi.fn();render(<BusinessPageRoot view={view} accessIdentity="synthetic"><Button onClick={click}>Record</Button><Input aria-label="Locked" readOnly value="3"/><Button disabled>Disabled</Button><section aria-busy="true"><Button>Busy</Button></section><div role="group" aria-disabled="true"><Button>Parent disabled</Button></div></BusinessPageRoot>);
 expect(execute(operation("Record"),false).state).toBe("blocked");expect(execute(operation("Record"),true,true).state).toBe("blocked");
 for(const name of ["Disabled","Busy","Parent disabled"])expect(execute(operation(name)).state).toBe("blocked");
 expect(execute(operation("Locked","set_value",{value:"4"})).state).toBe("blocked");expect(click).not.toHaveBeenCalled();
});
it("rejects a replaced same-label node, live edits, wrong view and workspace generation",()=>{
 const click=vi.fn();const element=(key:string)=><BusinessPageRoot view={view} accessIdentity="synthetic"><Button key={key} onClick={click}>Same</Button><Input aria-label="Draft" defaultValue="1"/></BusinessPageRoot>;
 const r=render(element("one"));const old=operation("Same");r.rerender(element("two"));expect(execute(old)).toEqual({state:"stale",snapshotId:old.snapshotId});
 const op=operation("Same");(screen.getByLabelText("Draft") as HTMLInputElement).value="2";expect(execute(op).state).toBe("stale");
 expect(operatePageControl("other:","generation",operation("Same"),()=>undefined).state).toBe("stale");
 expect(operatePageControl(view,"new-workspace",operation("Same"),()=>undefined).state).toBe("stale");expect(click).not.toHaveBeenCalled();
});
it("keeps assistant-occluded controls readable but never executes them",()=>{
 const click=vi.fn();render(<BusinessPageRoot view={view} accessIdentity="synthetic" occluded><Button onClick={click}>Record</Button></BusinessPageRoot>);
 expect(execute(operation("Record")).state).toBe("blocked");expect(click).not.toHaveBeenCalled();
});
it("excludes secret, hidden, foreign and unsupported controls",()=>{
 render(<><button>Foreign</button><BusinessPageRoot view={view} accessIdentity="synthetic"><SecretRegion><Input aria-label="Secret" defaultValue="canary"/></SecretRegion><div hidden><Button>Hidden</Button></div><input type="file"/><a href="https://example.com">External</a><Button>Public</Button></BusinessPageRoot></>);
 const p=capturePageContent(view,"generation");expect(JSON.stringify(p)).not.toContain("canary");expect(p.nodes.filter(n=>n.targetRef).map(n=>n.name)).toEqual(["Public"]);
 expect(execute({...operation("Public"),targetRef:crypto.randomUUID()}).state).toBe("invalid_target");
});
it("discovers owned popup controls only after opening and invalidates the prior snapshot",async()=>{
 const click=vi.fn();render(<BusinessPageRoot view={view} accessIdentity="synthetic"><Popover><PopoverTrigger>Options</PopoverTrigger><PopoverContent><Button onClick={click}>Apply</Button></PopoverContent></Popover></BusinessPageRoot>);
 const before=operation("Options");await act(async()=>{expect(execute(before).state).toBe("dispatched");});
 await screen.findByRole("button",{name:"Apply"});expect(execute(before).state).toBe("stale");
 act(()=>{expect(execute(operation("Apply")).state).toBe("dispatched");});expect(click).toHaveBeenCalledOnce();
});
it("reports delivery only for asynchronous handlers, preserving pending and error state",async()=>{
 let finish!:(value:void)=>void;
 function Form(){const [state,setState]=useState("ready");return <BusinessPageRoot view={view} accessIdentity="synthetic"><Button disabled={state==="pending"} onClick={async()=>{setState("pending");await new Promise<void>(r=>{finish=r;});setState("failed validation");}}>Submit</Button><p role="status">{state}</p></BusinessPageRoot>;}
 render(<Form/>);act(()=>{expect(execute(operation("Submit")).state).toBe("dispatched");});expect(screen.getByRole("status")).toHaveTextContent("pending");
 expect(execute(operation("Submit")).state).toBe("blocked");await act(async()=>finish());expect(screen.getByRole("status")).toHaveTextContent("failed validation");
});

it("inherits a protected security source through real nested shared portals",async()=>{
 const click=vi.fn();render(<BusinessPageRoot view={view} accessIdentity="synthetic"><SecurityOperationRegion><p>Public access status</p><Popover><PopoverTrigger>Security options</PopoverTrigger><PopoverContent><Popover><PopoverTrigger>Nested security options</PopoverTrigger><PopoverContent><Button onClick={click}>Change access</Button></PopoverContent></Popover></PopoverContent></Popover></SecurityOperationRegion><Button onClick={click}>Ordinary business</Button></BusinessPageRoot>);
 await userEvent.click(screen.getByRole("button",{name:"Security options"}));await userEvent.click(await screen.findByRole("button",{name:"Nested security options"}));await screen.findByRole("button",{name:"Change access"});
 const page=capturePageContent(view,"generation");const node=page.nodes.find(n=>n.name==="Change access")!;expect(node).toBeDefined();expect(node.state).toContain("operation_protected=true");expect(node.actions).toEqual([]);expect(node.targetRef).toBeUndefined();expect(JSON.stringify(page)).toContain("Public access status");expect(click).not.toHaveBeenCalled();
});
it("revokes already-mounted owned popup controls when their business owner becomes occluded",async()=>{
 const click=vi.fn();const content=(occluded:boolean)=><BusinessPageRoot view={view} accessIdentity="synthetic" occluded={occluded}><Popover open><PopoverTrigger>Options</PopoverTrigger><PopoverContent><Button onClick={click}>Popup action</Button></PopoverContent></Popover></BusinessPageRoot>;
 const r=render(content(false));await screen.findByRole("button",{name:"Popup action"});const before=operation("Popup action");r.rerender(content(true));
 expect(execute(before).state).toBe("stale");const fresh=operation("Popup action");expect(execute(fresh).state).toBe("blocked");expect(click).not.toHaveBeenCalled();r.rerender(content(false));act(()=>{expect(execute(operation("Popup action")).state).toBe("dispatched");});expect(click).toHaveBeenCalledOnce();
});
it("reports unverified when a real shared React click handler throws through window.error",()=>{
 const failure=new Error("synthetic React handler failure");const click=vi.fn(()=>{throw failure;});
 // Keep jsdom's test-level uncaught-error reporter from treating the deliberately
 // induced handler failure as a suite error. Production dispatch does not cancel it.
 const observed:unknown[]=[];const onError=(event:ErrorEvent)=>{observed.push(event.error);event.preventDefault();};window.addEventListener("error",onError);
 try {render(<BusinessPageRoot view={view} accessIdentity="synthetic"><Button onClick={click}>Failing action</Button></BusinessPageRoot>);expect(execute(operation("Failing action")).state).toBe("unverified");expect(click).toHaveBeenCalled();expect(observed).toContain(failure);}
 finally {window.removeEventListener("error",onError);}
});
it("revokes a formerly ordinary target and excludes external submit controls for protected forms",()=>{
 const click=vi.fn();const content=(protectedNow:boolean)=><BusinessPageRoot view={view} accessIdentity="synthetic"><div data-assistant-operation={protectedNow ? "protected" : undefined}><form id="security-form" onSubmit={e=>{e.preventDefault();click();}}><Input aria-label="Public setting"/></form></div><Button form="security-form">Save access</Button><Button type="button" onClick={click}>Ordinary business</Button></BusinessPageRoot>;
 const r=render(content(false));const old=operation("Save access");r.rerender(content(true));expect(execute(old)).toEqual({state:"stale",snapshotId:old.snapshotId});const page=capturePageContent(view,"generation");expect(page.nodes.find(n=>n.name==="Save access")?.targetRef).toBeUndefined();expect(click).not.toHaveBeenCalled();act(()=>{expect(execute(operation("Ordinary business")).state).toBe("dispatched");});expect(click).toHaveBeenCalledOnce();
});

it.each([{value:"",valid:true},{value:"a".repeat(2048),valid:true},{value:"a".repeat(2049),valid:false},{value:"😀".repeat(2048),valid:true},{value:"😀".repeat(2049),valid:false},{value:"a\0b",valid:false}])("matches code-point input limits (case %#)",({value,valid})=>{
 const changed=vi.fn();
 function Form(){const [text,setText]=useState("original");return <BusinessPageRoot view={view} accessIdentity="synthetic"><Input aria-label="Unicode value" value={text} onChange={e=>{changed();setText(e.target.value);}}/></BusinessPageRoot>;}
 render(<Form/>);
 act(()=>{expect(execute(operation("Unicode value","set_value",{value})).state).toBe(valid ? "dispatched" : "invalid_target");});
 expect(screen.getByLabelText("Unicode value")).toHaveValue(valid ? value : "original");
 expect(changed).toHaveBeenCalledTimes(valid ? 1 : 0);
});
it("rechecks live authority just before mutation and treats mid-event revocation as uncertain",()=>{
 let allowed=true;
 const changed=vi.fn();
 render(<BusinessPageRoot view={view} accessIdentity="synthetic"><Input aria-label="Live" defaultValue="original" onChange={()=>{changed();allowed=false;}}/></BusinessPageRoot>);
 const op=operation("Live","set_value",{value:"changed"});
 const refusal=vi.fn().mockReturnValueOnce(undefined).mockReturnValue("blocked");
 expect(operatePageControl(view,"generation",op,refusal).state).toBe("blocked");
 expect(screen.getByLabelText("Live")).toHaveValue("original");expect(changed).not.toHaveBeenCalled();
 act(()=>{expect(operatePageControl(view,"generation",op,()=>allowed ? undefined : "blocked").state).toBe("unverified");});
 expect(changed).toHaveBeenCalledOnce();
});
