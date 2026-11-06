import { useState } from "react";
import { act, render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { BusinessPageRoot, SecretRegion } from "@/lib/businessCapture";
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
const execute=(op:PageOperation,full=true,blocked=false)=>operatePageControl(view,"generation",op,full,blocked);
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
 const r=render(element("one"));const old=operation("Same");r.rerender(element("two"));expect(execute(old).state).toBe("stale");
 const op=operation("Same");(screen.getByLabelText("Draft") as HTMLInputElement).value="2";expect(execute(op).state).toBe("stale");
 expect(operatePageControl("other:","generation",operation("Same"),true,false).state).toBe("stale");
 expect(operatePageControl(view,"new-workspace",operation("Same"),true,false).state).toBe("stale");expect(click).not.toHaveBeenCalled();
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
