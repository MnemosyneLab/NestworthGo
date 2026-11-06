import { capturePageContent, capturedPageControl, controlOperations } from "./pageContent";

export type PageOperation = { snapshotId:string; targetRef:string; action:string; value?:string; optionRef?:string };
export type PageOperationResult = { state:string; snapshotId:string };

/** Dispatch only to the exact live node observed by semantic capture. This is
 * deliberately synchronous after the IPC claim: never schedule a later click. */
export function operatePageControl(viewKey:string, generation:string, operation:PageOperation, fullAccess:boolean, blocked:boolean):PageOperationResult {
 const before=capturePageContent(viewKey,generation);
 const result=(state:string)=>({state,snapshotId:before.id});
 if(!fullAccess || blocked) return result("blocked");
 if(before.id!==operation.snapshotId) return result("stale");
 const el=capturedPageControl(operation.targetRef);
 const node=before.nodes.find(n=>n.targetRef===operation.targetRef);
 if(!el || !el.isConnected || !node || !controlOperations(el).includes(operation.action)) return result("invalid_target");
 if(node.state.some(s=>["inert=true","disabled=true","readonly=true","busy=true","redacted","value_redacted=true"].includes(s)) ||
   el.closest('[inert],[aria-hidden="true"],[aria-busy="true"],[aria-disabled="true"],[data-assistant-private],[data-assistant-occluded="assistant"]') || el.matches(":disabled")) return result("blocked");
 if(operation.action==="activate") {
  if(operation.value!==undefined || operation.optionRef!==undefined) return result("invalid_target");
  // No synthetic keyboard, arbitrary event names, routes or code are accepted.
  el.click();
 } else if(operation.action==="set_value") {
  if(typeof operation.value!=="string" || operation.value.length>2048 || operation.optionRef!==undefined || !(el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement)) return result("invalid_target");
  const prototype=el instanceof HTMLInputElement ? HTMLInputElement.prototype : HTMLTextAreaElement.prototype;
  Object.getOwnPropertyDescriptor(prototype,"value")!.set!.call(el,operation.value);
  el.dispatchEvent(new Event("input",{bubbles:true}));
  el.dispatchEvent(new Event("change",{bubbles:true}));
 } else if(operation.action==="select_option") {
  const option=operation.optionRef ? capturedPageControl(operation.optionRef) : undefined;
  if(operation.value!==undefined || !(el instanceof HTMLSelectElement) || el.multiple || !(option instanceof HTMLOptionElement) || option.closest("select")!==el || option.disabled || option.closest("optgroup[disabled]") || !before.nodes.some(n=>n.targetRef===operation.optionRef)) return result("invalid_target");
  Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,"selectedIndex")!.set!.call(el,Array.from(el.options).indexOf(option));
  el.dispatchEvent(new Event("change",{bubbles:true}));
 } else return result("invalid_target");
 // Dispatch is not a receipt. Async validation, queries and financial commits
 // remain owned by the existing UI. The next read observes their current state.
 return result("dispatched");
}
