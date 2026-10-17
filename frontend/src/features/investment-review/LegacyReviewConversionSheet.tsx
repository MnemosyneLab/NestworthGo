import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/select";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { useInvestmentReviews } from "@/queries/investmentReview";
import { useReviewLegacyConversionCommands } from "@/queries/investmentReviewV2";
import type { ConvertReviewLegacyRuleV2Input, ReviewLegacyConversionDraft, ReviewLegacyConversionPreview } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import type { ReviewRule } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";

export function LegacyReviewConversionSheet({ rule, onClose, onConverted }: { rule: ReviewRule; onClose: () => void; onConverted: () => void }) {
  const { t } = useTranslation();
  const oldReport = useInvestmentReviews(true);
  const commands = useReviewLegacyConversionCommands();
  const oldPolicy = rule.legacySpec?.configuration.policy;
  const enrolledCount = Object.values(rule.legacySpec?.configuration.enrolled ?? {}).filter(Boolean).length;
  const [targetKind, setTargetKind] = useState("instruments");
  const [kind, setKind] = useState<"single" | "consecutive" | "rolling">("consecutive");
  const [count, setCount] = useState(oldPolicy?.sustainDays ?? 7);
  const [windowDays, setWindowDays] = useState(30);
  const [minHits, setMinHits] = useState(oldPolicy?.sustainDays ?? 7);
  const [publishedPreview, setPublishedPreview] = useState<{ key: string; value: ReviewLegacyConversionPreview } | null>(null);
  const [submitted, setSubmitted] = useState(false);
  const pendingSave = useRef<{ key: string; request: ConvertReviewLegacyRuleV2Input } | null>(null);
  const timezone = oldReport.data?.timezone ?? "";
  const triggerPolicy = kind === "single" ? { kind } : kind === "consecutive" ? { kind, count } : { kind, windowDays, minHits };
  const draft: ReviewLegacyConversionDraft = { legacyRuleId: rule.id, targetKind, timezone, triggerPolicy };
  const draftKey = JSON.stringify(draft);
  const preview = publishedPreview?.key === draftKey ? publishedPreview.value : null;
  const canPreview = !!oldPolicy && !!timezone && !commands.preview.isPending && !submitted;
  const runPreview = async () => {
    pendingSave.current = null;
    setPublishedPreview(null);
    try { setPublishedPreview({ key: draftKey, value: await commands.preview.mutateAsync(draft) }); }
    catch { /* The query error stays visible below. */ }
  };
  const convert = async () => {
    if (!preview || commands.convert.isPending) return;
    if (!pendingSave.current || pendingSave.current.key !== draftKey) {
      pendingSave.current = { key: draftKey, request: { ...draft, expectedRevision: preview.legacyRevision, expectedConfigRevision: preview.configRevision, expectedStateRevision: preview.stateRevision, expectedSourceToken: preview.sourceToken, expectedTemporalToken: preview.temporalToken, lowYieldRuleId: crypto.randomUUID(), lossRuleId: crypto.randomUUID(), mutationId: crypto.randomUUID() } };
    }
    setSubmitted(true);
    try { await commands.convert.mutateAsync(pendingSave.current.request); onConverted(); onClose(); }
    catch (cause) {
      const code = (cause as { wireError?: { code?: string } })?.wireError?.code;
      if (code === "conflict" || code === "validation" || code === "not_found") {
        // These repository refusals occur before commit. A fresh preview can
        // safely issue new IDs; other failures retain the original receipt.
        pendingSave.current = null;
        setPublishedPreview(null);
        setSubmitted(false);
      }
    }
  };
  return <Sheet open onOpenChange={open => { if (!open && !submitted) onClose(); }}><SheetContent size="lg" closeDisabled={submitted}><SheetHeader><SheetTitle>{t("reviewV2.convertLegacy")}</SheetTitle></SheetHeader>
    <div className="space-y-4 pt-4 text-sm">
      <p>{t("reviewV2.legacyConversionWarning", { span: oldPolicy?.sustainDays ?? 0, window: oldPolicy?.windowDays ?? 0 })}</p>
      {oldReport.isError && <p role="alert">{t("reviewV2.loadError")}</p>}
      <label className="block space-y-1"><span>{t("reviewV2.targetMode")}</span><NativeSelect value={targetKind} disabled={submitted} onChange={event => setTargetKind(event.target.value)}><option value="instruments">{t("reviewV2.selectedInstruments")} ({enrolledCount})</option><option value="all_current">{t("reviewV2.allCurrent")}</option></NativeSelect></label>
      {targetKind === "all_current" && <p role="status" className="text-muted-foreground">{t("reviewV2.legacyTargetExpansion")}</p>}
      <p>{t("reviewV2.timezone")}: {timezone || t("reviewV2.unknown")}</p>
      <label className="block space-y-1"><span>{t("reviewV2.trigger")}</span><NativeSelect value={kind} disabled={submitted} onChange={event => setKind(event.target.value as typeof kind)}><option value="single">{t("reviewV2.single")}</option><option value="consecutive">{t("reviewV2.consecutive")}</option><option value="rolling">{t("reviewV2.rolling")}</option></NativeSelect></label>
      {kind === "consecutive" && <label className="block space-y-1"><span>{t("reviewV2.consecutiveDays")}</span><Input type="number" disabled={submitted} min={1} max={365} value={count} onChange={event => setCount(Number(event.target.value))} /></label>}
      {kind === "rolling" && <div className="flex gap-3"><label className="block space-y-1"><span>{t("reviewV2.windowDays")}</span><Input type="number" disabled={submitted} min={1} max={365} value={windowDays} onChange={event => setWindowDays(Number(event.target.value))} /></label><label className="block space-y-1"><span>{t("reviewV2.minHits")}</span><Input type="number" disabled={submitted} min={1} max={365} value={minHits} onChange={event => setMinHits(Number(event.target.value))} /></label></div>}
      <Button variant="outline" disabled={!canPreview} onClick={() => void runPreview()}>{t("reviewV2.preview")}</Button>
      {commands.preview.isError && <p role="alert">{String(commands.preview.error)}</p>}
      {preview && <div className="space-y-2 rounded-lg border p-3" role="status"><p>{t("reviewV2.legacyConversionCounts", { low: preview.lowYieldAlerts, loss: preview.lossAlerts, unknown: preview.unknownOutcomes })}</p><p className="text-muted-foreground">{t("reviewV2.legacyAckWarning")}</p><Button disabled={commands.convert.isPending} onClick={() => void convert()}>{t("reviewV2.convertLegacyAction")}</Button></div>}
      {commands.convert.isError && <p role="alert">{String(commands.convert.error)}</p>}
      {submitted && commands.convert.isError && <p role="status">{t("reviewV2.legacyRetry")}</p>}
    </div>
  </SheetContent></Sheet>;
}
