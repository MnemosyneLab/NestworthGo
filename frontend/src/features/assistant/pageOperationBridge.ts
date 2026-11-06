import { operatePageControl, type PageOperation, type PageOperationResult } from "./pageOperations";

type ClaimedOperation=Omit<PageOperation,"value"|"optionRef"> & { expiresAt?:string|null; value?:string|null; optionRef?:string|null };
export type PageOperationAPI={
 pending:()=>Promise<{id:string}[]|null>;
 claim:(id:string)=>Promise<ClaimedOperation>;
 deliver:(id:string,result:PageOperationResult)=>Promise<unknown>;
};
/** One polling pass. The backend owns consumption, so no local replay queue is
 * kept across errors or remounts. Read host state again after asynchronous claim. */
export async function pollPageOperations(api:PageOperationAPI,current:()=>{viewKey:string;generation:string;fullAccess:boolean;blocked:()=>boolean},stopped:()=>boolean) {
 const operations=await api.pending();
 for(const request of operations ?? []) {
  if(stopped()) return;
  const operation=await api.claim(request.id);
  if(stopped()) return;
  if(!operation.expiresAt || !(Date.parse(operation.expiresAt)>Date.now())) continue;
  const host=current();
  let result:PageOperationResult;
  try {
   result=operatePageControl(host.viewKey,host.generation,{
    snapshotId:operation.snapshotId,targetRef:operation.targetRef,action:operation.action,
    ...(operation.value!=null ? {value:operation.value}:{}),
    ...(operation.optionRef ? {optionRef:operation.optionRef}:{}),
   },host.fullAccess,host.blocked());
  } catch {result={state:"unverified",snapshotId:operation.snapshotId};}
  await api.deliver(request.id,result);
 }
}
