import { useEffect, useRef, useState } from "react";
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
 const reviews=useQuery({queryKey:["aiReviews",epoch,id,runKey],queryFn:()=>callService(()=>API.Reviews(epoch,id)),enabled:!!epoch&&!!id,networkMode:"always",retry:false,refetchInterval:query=>busy || query.state.data?.some(review=>!["succeeded","cancelled","failed"].includes(review.state)) ? 1500 : false});
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
 const completedFull = (reviews.data??[]).filter(r=>r.source==="ai_full"&&r.state==="succeeded").map(r=>r.id).join(",");
 useEffect(()=>{if(completedFull)void client.invalidateQueries({predicate:q=>!String(q.queryKey[0]).startsWith("ai")});},[client,completedFull]);
 const items=reviews.data??[];
 if(!items.length&&!reviews.isError)return null;
 return <section aria-label={t("aiChat.writeReview")} className="max-h-72 shrink-0 overflow-y-auto border-t border-border p-3 text-sm">
  <h3 className="font-medium">{t("aiChat.writeReview")}</h3>
  {(uncertain||reviews.isError)&&<p role="alert">{t("aiChat.writeUnknown")}</p>}
  {items.map(review=><article key={review.id} className="my-2 space-y-2 rounded border border-border p-3">
   {review.source==="ai_full"&&<p className="font-medium">{t("aiChat.fullActive")}</p>}
   <p className="py-1 text-xs text-muted-foreground">{t(review.source==="ai_full" ? "aiChat.fullHelp" : "aiChat.writeReviewHelp")}</p>
   {review.kind!=="capability"&&<p className="font-medium">{displayEnum(t,"history.kind",review.kind)} · {review.account}</p>}
   {review.summary ? <p className="whitespace-pre-wrap num">{review.kind === "capability" ? t(`aiChat.source.${review.summary}`, {defaultValue: review.summary.replaceAll("_", " ")}) : review.summary}</p> : <dl className="grid grid-cols-1 gap-1 break-words">
    {review.accountType ? <>
     <div><dt className="inline">{t("aiChat.writeInstitution")}: </dt><dd className="inline"><span className="rounded border border-border px-1">{t(review.accountInstitution ? "aiChat.writeInstitutionAssigned" : "aiChat.writeInstitutionUnassigned")}</span>{review.accountInstitution && <span className="ml-2">{review.accountInstitution}</span>}</dd></div>
     <div><dt className="inline">{t("aiChat.writeOwners")}: </dt><dd><ul>{review.accountOwners?.map((owner,index)=><li key={index}>{owner}</li>)}</ul></dd></div>
     <div><dt className="inline">{t("aiChat.writeAccountType")}: </dt><dd className="inline">{displayEnum(t,"enum",review.accountType)} · {review.accountCurrency}</dd></div>
    </> : <p>{t("aiChat.writeIdentityMissing")}</p>}
    <div><dt className="inline">{t("aiChat.writeAmount")}: </dt><dd className="inline num">{review.amount} {review.currency}</dd></div>
    <div><dt className="inline">{t("aiChat.writeResult")}: </dt><dd className="inline num">{review.resultingAmount} {review.currency}</dd></div>
    <div><dt className="inline">{t("aiChat.writeTime")}: </dt><dd className="inline">{review.effectiveAt}</dd></div>
    <div><dt className="inline">{t("history.reasonLabel")}: </dt><dd className="inline">{displayEnum(t,"history.reason",review.reason)}</dd></div>
    {review.note&&<div><dt className="inline">{t("aiChat.writeNote")}: </dt><dd className="inline whitespace-pre-wrap">{review.note}</dd></div>}
   </dl>}
   <p role="status">{t(review.source==="ai_full" && review.state==="awaiting_confirmation" ? "aiChat.fullAwaiting" : review.source==="ai_full" && review.state==="needs_preview" ? "aiChat.fullNeedsPreview" : review.source==="ai_full" && review.state==="succeeded" ? "aiChat.fullSucceeded" : `aiChat.writeState.${review.state}`)}</p>
   <div className="flex flex-wrap gap-2">
    {review.source!=="ai_full"&&review.state==="awaiting_confirmation"&&<Button type="button" disabled={!enabled||busy||pending||reviews.isError||uncertain||!review.accountType} onClick={()=>void act(review,"confirm")}>{t("aiChat.writeConfirm")}</Button>}
    {review.source!=="ai_full"&&review.state==="needs_preview"&&<Button type="button" disabled={!enabled||busy||pending||reviews.isError} onClick={()=>void act(review,"repreview")}>{t("aiChat.writeRepreview")}</Button>}
    {["awaiting_confirmation","needs_preview"].includes(review.state)&&<Button type="button" variant="outline" disabled={busy||pending||reviews.isError} onClick={()=>void act(review,"cancel")}>{t("aiChat.writeCancel")}</Button>}
   </div>
  </article>)}
  <Button type="button" variant="outline" disabled={pending} onClick={()=>void reviews.refetch().then(result=>{if(!result.isError)setUncertain(false);})}>{t("aiChat.writeInspect")}</Button>
 </section>;
}
