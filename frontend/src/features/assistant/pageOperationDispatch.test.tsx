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
