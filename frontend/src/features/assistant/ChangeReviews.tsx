import { useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as API from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service";
import { Button } from "@/components/ui/button";
import { callService } from "@/lib/wails";
import { displayEnum } from "@/lib/display";

/** This panel renders only host-frozen data. Chat and model tool output cannot
 * trigger confirmation. Reconnect/polling only inspects the original identity. */
export function ChangeReviews({ epoch, id, enabled, busy, runKey = "" }: {epoch:string;id:string;enabled:boolean;busy:boolean;runKey?:string}) {
 const {t}=useTranslation();
 const client=useQueryClient();
 const guard=useRef(false);
 const [pending,setPending]=useState(false);
 const [uncertain,setUncertain]=useState(false);
 const reviews=useQuery({queryKey:["aiReviews",epoch,id,runKey],queryFn:()=>callService(()=>API.Reviews(epoch,id)),enabled:!!epoch&&!!id,networkMode:"always",retry:false,refetchInterval:query=>busy || query.state.data?.some(review=>!["succeeded","cancelled"].includes(review.state)) ? 1500 : false});
 async function act(review:NonNullable<typeof reviews.data>[number],action:string) {
  if(guard.current)return;
  guard.current=true;setPending(true);setUncertain(false);
  try {await callService(()=>API.ReviewAction(epoch,id,review.id,review.digest,action));}
  catch {setUncertain(true);}
  finally {
   // Reconcile even after a lost reply. Never repeat the write automatically.
   await reviews.refetch();
   await client.invalidateQueries({predicate:q=>!String(q.queryKey[0]).startsWith("ai")});
   guard.current=false;setPending(false);
  }
 }
 const items=reviews.data??[];
 if(!items.length&&!reviews.isError)return null;
 return <section aria-label={t("aiChat.writeReview")} className="max-h-72 shrink-0 overflow-y-auto border-t border-border p-3 text-sm">
  <h3 className="font-medium">{t("aiChat.writeReview")}</h3>
  <p className="py-1 text-xs text-muted-foreground">{t("aiChat.writeReviewHelp")}</p>
  {(uncertain||reviews.isError)&&<p role="alert">{t("aiChat.writeUnknown")}</p>}
  {items.map(review=><article key={review.id} className="my-2 space-y-2 rounded border border-border p-3">
   <p className="font-medium">{displayEnum(t,"history.kind",review.kind)} · {review.account}</p>
   <dl className="grid grid-cols-1 gap-1 break-words">
    <div><dt className="inline">{t("aiChat.writeAmount")}: </dt><dd className="inline num">{review.amount} {review.currency}</dd></div>
    <div><dt className="inline">{t("aiChat.writeResult")}: </dt><dd className="inline num">{review.resultingAmount} {review.currency}</dd></div>
    <div><dt className="inline">{t("aiChat.writeTime")}: </dt><dd className="inline">{review.effectiveAt}</dd></div>
    <div><dt className="inline">{t("history.reasonLabel")}: </dt><dd className="inline">{displayEnum(t,"history.reason",review.reason)}</dd></div>
    {review.note&&<div><dt className="inline">{t("aiChat.writeNote")}: </dt><dd className="inline whitespace-pre-wrap">{review.note}</dd></div>}
   </dl>
   <p role="status">{t(`aiChat.writeState.${review.state}`)}</p>
   <div className="flex flex-wrap gap-2">
    {review.state==="awaiting_confirmation"&&<Button type="button" disabled={!enabled||busy||pending||reviews.isError||uncertain} onClick={()=>void act(review,"confirm")}>{t("aiChat.writeConfirm")}</Button>}
    {review.state==="needs_preview"&&<Button type="button" disabled={!enabled||busy||pending||reviews.isError} onClick={()=>void act(review,"repreview")}>{t("aiChat.writeRepreview")}</Button>}
    {["awaiting_confirmation","needs_preview"].includes(review.state)&&<Button type="button" variant="outline" disabled={busy||pending||reviews.isError} onClick={()=>void act(review,"cancel")}>{t("aiChat.writeCancel")}</Button>}
   </div>
  </article>)}
  <Button type="button" variant="outline" disabled={pending} onClick={()=>void reviews.refetch().then(result=>{if(!result.isError)setUncertain(false);})}>{t("aiChat.writeInspect")}</Button>
 </section>;
}
