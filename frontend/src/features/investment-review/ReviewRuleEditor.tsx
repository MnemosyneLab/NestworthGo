import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { useInstruments } from "@/queries/investments";
import { useSettings } from "@/queries/settings";
import { useReviewV2Commands } from "@/queries/investmentReviewV2";
import { Service } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity";
import { InstrumentType } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import type { ReviewRule, ReviewRuleLeaf, ReviewRuleSpec, ReviewPredicateNode } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import { callService } from "@/lib/wails";

type Condition = ReviewRuleLeaf;
const choices = Object.values(InstrumentType).filter(value => value !== InstrumentType.$zero);
const defaultCondition = (): Condition => ({ metric: "amount", operator: ">", value: "1000", currency: "CNY", fxMode: "native" });
const leaves = (node: ReviewPredicateNode): Condition[] => (node.all ?? node.any ?? []).flatMap(child => child.leaf ? [child.leaf] : []);
const hasNestedGroup = (node: ReviewPredicateNode) => (node.all ?? node.any ?? []).some(child => !child.leaf);
function parsePredicate(source: string): ReviewPredicateNode | null {
  try {
    const value = JSON.parse(source) as ReviewPredicateNode;
    if (!value || typeof value !== "object" || Boolean(value.all) === Boolean(value.any) || !(value.all ?? value.any)?.length) return null;
    return value;
  } catch { return null; }
}
function predicate(conditions: Condition[], join: "all" | "any"): ReviewPredicateNode {
  const nodes = conditions.map(leaf => ({ leaf }));
  return join === "all" ? { all: nodes } : { any: nodes };
}
function cleanCondition(value: Condition): Condition {
  if (value.metric === "type") return { metric: "type", operator: "in", types: value.types?.length ? value.types : [InstrumentType.InstrumentBankInvestmentProduct] };
  if (value.metric === "price_change" || value.metric === "annualized") return { metric: value.metric, operator: value.operator || "<", value: value.value ?? "", lookbackDays: value.lookbackDays ?? 30 };
  return { metric: value.metric, operator: value.operator || ">", value: value.value ?? "", currency: value.currency ?? "", fxMode: value.fxMode ?? "native" };
}
function makeSpec(timezone: string, targetKind: string, target: string[], conditions: Condition[], join: "all" | "any", policy: string, count: number, windowDays: number, minHits: number): ReviewRuleSpec {
  return {
    specVersion: 1, metricPolicyVersion: "daily_metrics_v1", evaluator: "daily_predicate_v1",
    target: targetKind === "instruments" ? { kind: "instruments", instrumentIds: target } : { kind: "all_current" },
    predicate: predicate(conditions.map(cleanCondition), join),
    schedule: { period: "calendar_day", timezone, cutoffVersion: "civil_day_end_v1" },
    triggerPolicy: policy === "consecutive" ? { kind: policy, count } : policy === "rolling" ? { kind: policy, windowDays, minHits } : { kind: "single" },
    dataPolicy: { missing: "unknown", calendar: "all_civil_days", quoteCarry: "forbid_for_daily_signal", baselineMaxGapDays: 7, fxReferenceMaxAgeDays: 7, fxTimestampMaxAgeHours: 12 },
  };
}

export function ReviewRuleEditor({ open, onOpenChange, rule }: { open: boolean; onOpenChange: (open: boolean) => void; rule?: ReviewRule | null }) {
  const { t } = useTranslation();
  const instruments = useInstruments();
  const settings = useSettings();
  const commands = useReviewV2Commands();
  const [name, setName] = useState(rule?.name ?? "");
  const [enabled, setEnabled] = useState(rule?.enabled ?? true);
  const [severity, setSeverity] = useState(rule?.severity ?? "notice");
  const [timezone, setTimezone] = useState<string | null>(rule?.spec.schedule.timezone ?? null);
  const selectedTimezone = timezone ?? settings.data?.timezone ?? "Etc/UTC";
  const [targetKind, setTargetKind] = useState(rule?.spec.target.kind ?? "all_current");
  const [target, setTarget] = useState<string[]>(rule?.spec.target.instrumentIds ?? []);
  const [join, setJoin] = useState<"all" | "any">(rule?.spec.predicate.any ? "any" : "all");
  const [conditions, setConditions] = useState<Condition[]>(rule ? leaves(rule.spec.predicate) : [defaultCondition()]);
  const [advanced, setAdvanced] = useState(hasNestedGroup(rule?.spec.predicate ?? { all: [] }));
  const [predicateJSON, setPredicateJSON] = useState(JSON.stringify(rule?.spec.predicate ?? predicate([defaultCondition()], "all"), null, 2));
  const [policy, setPolicy] = useState(rule?.spec.triggerPolicy.kind ?? "single");
  const [count, setCount] = useState(rule?.spec.triggerPolicy.count ?? 3);
  const [windowDays, setWindowDays] = useState(rule?.spec.triggerPolicy.windowDays ?? 10);
  const [minHits, setMinHits] = useState(rule?.spec.triggerPolicy.minHits ?? 5);
  const [previewInstrument, setPreviewInstrument] = useState(rule?.spec.target.instrumentIds?.[0] ?? "");
  const [previewKey, setPreviewKey] = useState("");
  const [preview, setPreview] = useState<Awaited<ReturnType<typeof Service.PreviewReviewRuleV2>> | null>(null);
  const [error, setError] = useState("");
  const parsedPredicate = useMemo(() => parsePredicate(predicateJSON), [predicateJSON]);
  const spec = useMemo(() => {
    const base = makeSpec(selectedTimezone, targetKind, target, conditions, join, policy, count, windowDays, minHits);
    return advanced && parsedPredicate ? { ...base, predicate: parsedPredicate } : base;
  }, [selectedTimezone, targetKind, target, conditions, join, policy, count, windowDays, minHits, advanced, parsedPredicate]);
  const invalid = !name.trim() || !selectedTimezone.trim() || (advanced ? !parsedPredicate : conditions.some(c => c.metric === "type" ? !c.types?.length : !c.value?.trim())) || (targetKind === "instruments" && !target.length) || (!advanced && !conditions.length) || (policy === "consecutive" && (count < 1 || count > 365)) || (policy === "rolling" && (windowDays < 1 || windowDays > 365 || minHits < 1 || minHits > windowDays)) || (!advanced && conditions.some(c => c.metric === "annualized") && (join !== "all" || !conditions.some(c => c.metric === "type" && c.types?.length === 1 && c.types[0] === "bank_investment_product")));
  const save = async () => {
    setError("");
    try {
      const request = { name: name.trim(), enabled, severity, spec };
      if (rule) await commands.updateRule.mutateAsync({ ...request, ruleId: rule.id, expectedRevision: rule.rowRevision });
      else await commands.createRule.mutateAsync(request);
      onOpenChange(false);
    } catch (cause) { setError(String(cause)); }
  };
  const runPreview = async () => {
    setError(""); setPreview(null);
    try { const result = await callService(() => Service.PreviewReviewRuleV2({ spec, instrumentId: previewInstrument, latestClosedOn: "", expectedSourceToken: "" })); setPreview(result); setPreviewKey(JSON.stringify(spec)); }
    catch (cause) { setError(String(cause)); }
  };
  const update = (index: number, value: Partial<Condition>) => setConditions(items => items.map((item, i) => i === index ? { ...item, ...value } : item));
  return <Sheet open={open} onOpenChange={onOpenChange}><SheetContent size="xl"><SheetHeader><SheetTitle>{t(rule ? "reviewV2.editRule" : "reviewV2.newRule")}</SheetTitle></SheetHeader>
    <div className="space-y-5 pb-5">
      <div className="grid gap-2"><Label htmlFor="review-rule-name">{t("reviewV2.name")}</Label><Input id="review-rule-name" value={name} onChange={event => setName(event.target.value)} maxLength={120} /></div>
      <div className="flex flex-wrap gap-4"><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={enabled} onChange={event => setEnabled(event.target.checked)} />{t("reviewV2.enabled")}</label><label className="flex items-center gap-2 text-sm">{t("reviewV2.severity")}<NativeSelect value={severity} onChange={event => setSeverity(event.target.value)}><option value="notice">{t("reviewV2.notice")}</option><option value="warning">{t("reviewV2.warning")}</option></NativeSelect></label></div>
      <fieldset className="space-y-2"><legend className="font-medium">{t("reviewV2.target")}</legend><p className="text-xs text-muted-foreground">{t("reviewV2.positiveHoldings")}</p><NativeSelect aria-label={t("reviewV2.targetMode")} value={targetKind} onChange={event => { const kind = event.target.value; setTargetKind(kind); if (kind === "instruments" && !target.length && instruments.data?.[0]) setTarget([instruments.data[0].id]); }}><option value="all_current">{t("reviewV2.allCurrent")}</option><option value="instruments">{t("reviewV2.selectedInstruments")}</option></NativeSelect>
      {targetKind === "instruments" && <div className="max-h-36 overflow-auto rounded-lg border p-2">{(instruments.data ?? []).map(item => <label key={item.id} className="flex gap-2 p-1 text-sm"><input type="checkbox" checked={target.includes(item.id)} onChange={event => setTarget(ids => event.target.checked ? [...ids, item.id] : ids.filter(id => id !== item.id))} />{item.name}</label>)}</div>}</fieldset>
      <fieldset className="space-y-3"><legend className="font-medium">{t("reviewV2.dailyConditions")}</legend><Button type="button" variant="outline" onClick={() => {
        if (advanced) {
          if (!parsedPredicate || hasNestedGroup(parsedPredicate)) return;
          setJoin(parsedPredicate.any ? "any" : "all"); setConditions(leaves(parsedPredicate)); setAdvanced(false);
        } else {
          setPredicateJSON(JSON.stringify(predicate(conditions.map(cleanCondition), join), null, 2)); setAdvanced(true);
        }
      }} disabled={advanced && (!parsedPredicate || hasNestedGroup(parsedPredicate))}>{t(advanced ? "reviewV2.simpleConditions" : "reviewV2.advancedConditions")}</Button>
      {advanced ? <label className="block space-y-2 text-sm">{t("reviewV2.predicateJSON")}<textarea className="min-h-48 w-full rounded-md border bg-background p-2 font-mono text-xs" spellCheck={false} value={predicateJSON} onChange={event => setPredicateJSON(event.target.value)} /></label> : <><NativeSelect aria-label={t("reviewV2.matchMode")} value={join} onChange={event => setJoin(event.target.value as "all" | "any")}><option value="all">{t("reviewV2.allConditions")}</option><option value="any">{t("reviewV2.anyCondition")}</option></NativeSelect>
        {conditions.map((condition, index) => <div key={index} className="flex flex-wrap items-center gap-2 rounded-lg border p-2">
          <NativeSelect aria-label={t("reviewV2.metric")} value={condition.metric} onChange={event => { const metric = event.target.value; update(index, metric === "type" ? { metric, operator: "in", types: [InstrumentType.InstrumentBankInvestmentProduct] } : metric === "annualized" || metric === "price_change" ? { metric, operator: "<", value: "0", lookbackDays: 30 } : { metric, operator: ">", value: "1000", currency: "CNY", fxMode: "native" }); }}><option value="type">{t("reviewV2.type")}</option><option value="amount">{t("reviewV2.amount")}</option><option value="cost">{t("reviewV2.cost")}</option><option value="price_change">{t("reviewV2.priceChange")}</option><option value="annualized">{t("reviewV2.annualized")}</option></NativeSelect>
          {condition.metric === "type" ? <div className="flex max-h-32 flex-wrap gap-2 overflow-auto" aria-label={t("reviewV2.type")}>{choices.map(choice => <label key={choice} className="flex items-center gap-1 text-xs"><input type="checkbox" checked={condition.types?.includes(choice) ?? false} onChange={event => update(index, { types: event.target.checked ? [...(condition.types ?? []), choice] : (condition.types ?? []).filter(item => item !== choice) })} />{t(`reviewV2.types.${choice}`)}</label>)}</div> : <><NativeSelect aria-label={t("reviewV2.operator")} value={condition.operator} onChange={event => update(index, { operator: event.target.value })}>{[">", ">=", "<", "<="].map(op => <option key={op} value={op}>{op}</option>)}</NativeSelect><Input aria-label={t("reviewV2.value")} className="w-24" inputMode="decimal" value={condition.value ?? ""} onChange={event => update(index, { value: event.target.value })} />{condition.metric === "amount" || condition.metric === "cost" ? <><Input aria-label={t("reviewV2.currency")} className="w-20" maxLength={3} value={condition.currency ?? "CNY"} onChange={event => update(index, { currency: event.target.value.toUpperCase() })} /><NativeSelect aria-label={t("reviewV2.fxMode")} value={condition.fxMode ?? "native"} onChange={event => update(index, { fxMode: event.target.value })}><option value="native">{t("reviewV2.nativeCurrency")}</option><option value="convert_on_day">{t("reviewV2.convertOnDay")}</option></NativeSelect></> : <><span className="text-sm">%</span><Input aria-label={t("reviewV2.lookbackDays")} className="w-20" type="number" min={1} max={365} value={condition.lookbackDays ?? 30} onChange={event => update(index, { lookbackDays: Number(event.target.value) })} /><span className="text-sm">{t("reviewV2.days")}</span></>}</>}
          <Button type="button" variant="ghost" disabled={conditions.length === 1} onClick={() => setConditions(items => items.filter((_, i) => i !== index))}>{t("reviewV2.remove")}</Button>
        </div>)}<Button type="button" variant="outline" disabled={conditions.length >= 20} onClick={() => setConditions(items => [...items, defaultCondition()])}>{t("reviewV2.addCondition")}</Button></>}
      </fieldset>
      <fieldset className="space-y-3"><legend className="font-medium">{t("reviewV2.trigger")}</legend><NativeSelect aria-label={t("reviewV2.trigger")} value={policy} onChange={event => setPolicy(event.target.value)}><option value="single">{t("reviewV2.single")}</option><option value="consecutive">{t("reviewV2.consecutive")}</option><option value="rolling">{t("reviewV2.rolling")}</option></NativeSelect>
      {policy === "consecutive" && <label className="flex items-center gap-2 text-sm">{t("reviewV2.consecutiveDays")}<Input className="w-20" type="number" min={1} max={365} value={count} onChange={event => setCount(Number(event.target.value))} /></label>}
      {policy === "rolling" && <div className="flex flex-wrap gap-3"><label className="flex items-center gap-2 text-sm">{t("reviewV2.windowDays")}<Input className="w-20" type="number" min={1} max={365} value={windowDays} onChange={event => setWindowDays(Number(event.target.value))} /></label><label className="flex items-center gap-2 text-sm">{t("reviewV2.minHits")}<Input className="w-20" type="number" min={1} max={windowDays} value={minHits} onChange={event => setMinHits(Number(event.target.value))} /></label></div>}</fieldset>
      <div className="space-y-2"><label className="flex items-center gap-2 text-sm">{t("reviewV2.timezone")}<Input className="w-48" value={selectedTimezone} onChange={event => setTimezone(event.target.value)} /></label><p className="text-xs text-muted-foreground">{t("reviewV2.dailyHelp", { timezone: selectedTimezone })}</p></div>
      <div className="rounded-lg border p-3 space-y-2"><p className="font-medium">{t("reviewV2.preview")}</p><NativeSelect aria-label={t("reviewV2.previewInstrument")} value={previewInstrument} onChange={event => setPreviewInstrument(event.target.value)}><option value="">{t("reviewV2.selectInstrument")}</option>{(instruments.data ?? []).filter(item => !target.length || target.includes(item.id)).map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</NativeSelect><Button type="button" variant="outline" disabled={invalid || !previewInstrument} onClick={() => void runPreview()}>{t("reviewV2.preview")}</Button>{preview && previewKey === JSON.stringify(spec) && <p className="text-sm">{t("reviewV2.previewResult", { hits: preview.window.hitCount, misses: preview.window.falseCount, unknowns: preview.window.unknownCount, from: preview.window.windowStart, to: preview.window.windowEnd })}</p>}</div>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}{invalid && <p className="text-sm text-destructive">{t("reviewV2.invalidRule")}</p>}
      <div className="flex justify-end gap-2"><Button variant="outline" onClick={() => onOpenChange(false)}>{t("common.cancel")}</Button><Button disabled={invalid || commands.createRule.isPending || commands.updateRule.isPending} onClick={() => void save()}>{t("common.save")}</Button></div>
    </div>
  </SheetContent></Sheet>;
}
