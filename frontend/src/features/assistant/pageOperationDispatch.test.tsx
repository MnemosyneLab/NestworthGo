import { useState } from "react";
import { act, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { BusinessPageRoot } from "@/lib/businessCapture";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/select";
import * as pageContent from "./pageContent";
import { pollPageOperations } from "./pageOperationBridge";

afterEach(()=>{vi.restoreAllMocks();vi.useRealTimers();});
it.each(["activate","set_value","select_option"].flatMap(action=>["expiry","full","blocked","view","generation","stopped"].map(fence=>({action,fence}))))("checks $fence after real capture before $action",async({action,fence})=>{
 const changed=vi.fn();
 function Form(){const [value,setValue]=useState("original");const [choice,setChoice]=useState("a");return <BusinessPageRoot view="synthetic:" accessIdentity="test"><Button onClick={changed}>Action</Button><Input aria-label="Value" value={value} onChange={e=>{changed();setValue(e.target.value);}}/><NativeSelect aria-label="Choice" value={choice} onChange={e=>{changed();setChoice(e.target.value);}}><option value="a">A</option><option value="b">B</option></NativeSelect></BusinessPageRoot>;}
 render(<Form/>);
 vi.useFakeTimers({toFake:["Date"]});vi.setSystemTime(new Date("2030-01-01T00:00:00Z"));
 const page=pageContent.capturePageContent("synthetic:","generation");
 const name=action==="activate" ? "Action" : action==="set_value" ? "Value" : "Choice";
 const operation={snapshotId:page.id,targetRef:page.nodes.find(n=>n.name===name)!.targetRef!,action,expiresAt:"2030-01-01T00:00:01Z",...(action==="set_value" ? {value:"changed"} : {}),...(action==="select_option" ? {optionRef:page.nodes.find(n=>n.name==="B")!.targetRef!} : {})};
 let blocked=false,stopped=false;
 const host={viewKey:"synthetic:",generation:"generation",fullAccess:true,blocked:()=>blocked};
 const capture=pageContent.capturePageContent;
 vi.spyOn(pageContent,"capturePageContent").mockImplementationOnce((...args)=>{
  const result=capture(...args);
  if(fence==="expiry")vi.setSystemTime(new Date("2030-01-01T00:00:02Z"));
  if(fence==="full")host.fullAccess=false;
  if(fence==="blocked")blocked=true;
  if(fence==="view")host.viewKey="other:";
  if(fence==="generation")host.generation="other";
  if(fence==="stopped")stopped=true;
  return result;
 });
 const deliver=vi.fn().mockResolvedValue(undefined);
 await act(()=>pollPageOperations({pending:async()=>[{id:"request"}],claim:async()=>operation,deliver},()=>({...host}),()=>stopped));
 expect(deliver).toHaveBeenCalledWith("request",{state:fence==="expiry" || fence==="stopped" ? "not_dispatched" : "blocked",snapshotId:page.id});
 expect(changed).not.toHaveBeenCalled();expect(screen.getByLabelText("Value")).toHaveValue("original");expect(screen.getByLabelText("Choice")).toHaveValue("a");
});


it("allows a read-only navigation then fresh page interaction, but fences unknown side effects", async()=>{
 const write=vi.fn();
 function App(){const [page,setPage]=useState("overview");const [value,setValue]=useState("before");return <>
  <nav data-assistant-navigation={`${page}:`} data-assistant-identity="synthetic"><Button data-assistant-effect="presentation" onClick={()=>setPage("asset-changes")}>Asset changes</Button><Button onClick={write}>Refresh provider</Button></nav>
  <BusinessPageRoot view={`${page}:`} accessIdentity="synthetic"><section data-assistant-effect="presentation"><Input aria-label="Filter" value={value} onChange={e=>setValue(e.target.value)}/></section><Button onClick={write}>Save money</Button></BusinessPageRoot>
 </>;}
 render(<App/>);
 let viewKey="overview:";
 const host=()=>({viewKey,generation:"generation",fullAccess:false,blocked:()=>false});
 async function dispatch(name:string,action:string,value?:string){const page=pageContent.capturePageContent(viewKey,"generation");const deliver=vi.fn();await act(()=>pollPageOperations({pending:async()=>[{id:"once"}],claim:async()=>({snapshotId:page.id,targetRef:page.nodes.find(n=>n.name===name)!.targetRef!,action,value,expiresAt:"2099-01-01T00:00:00Z"}),deliver},host,()=>false));return deliver.mock.calls[0][1].state;}
 expect(await dispatch("Asset changes","activate")).toBe("dispatched");viewKey="asset-changes:";
 expect(await dispatch("Filter","set_value","last week")).toBe("dispatched");expect(screen.getByLabelText("Filter")).toHaveValue("last week");
 expect(await dispatch("Save money","activate")).toBe("blocked");expect(await dispatch("Refresh provider","activate")).toBe("blocked");expect(write).not.toHaveBeenCalled();
});
