import { useLayoutEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { FieldError } from "@/components/ui/field-error";
import { FilterableSelect } from "@/components/ui/filterable-select";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { useCatalog } from "@/queries/catalog";
import { useInstruments } from "@/queries/investments";
import { useBootstrap } from "@/queries/household";
import { useHistoryOrigin } from "@/queries/history";
import { useReviewV2Commands } from "@/queries/investmentReviewV2";
import { formatCivilDateRange, timeZoneOptions } from "@/lib/time";
import { Service } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity";
import { InstrumentType } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import type { ReviewRule, ReviewRuleLeaf, ReviewRuleSpec, ReviewPredicateNode } from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import { callService } from "@/lib/wails";

type Condition = ReviewRuleLeaf;
type GroupMode = "all" | "any";
type PredicateTreeChange = (update: (node: ReviewPredicateNode) => ReviewPredicateNode) => void;
const choices = Object.values(InstrumentType).filter(value => value !== InstrumentType.$zero);
const defaultCondition = (currency: string): Condition => ({ metric: "amount", operator: ">", value: "1000", ...(currency ? { currency } : {}), fxMode: "native" });
const defaultPredicate = (currency: string): ReviewPredicateNode => ({ all: [{ leaf: defaultCondition(currency) }] });
const moneyThresholdPattern = /^-?\d+(?:\.\d+)?$/;
const percentageThresholdPattern = /^-?(0|[1-9]\d{0,4})(?:\.\d{1,8})?$/;
const currencyCodePattern = /^[A-Z]{3}$/;
const supportedLookbackDays = [7, 30, 90, 180, 365] as const;
const timeZoneSelectOptions = timeZoneOptions().map(timezone => ({ value: timezone, label: timezone }));
const isGroup = (node: ReviewPredicateNode) => node.all !== undefined || node.any !== undefined;
const groupMode = (node: ReviewPredicateNode): GroupMode => node.any !== undefined ? "any" : "all";
const groupChildren = (node: ReviewPredicateNode) => node.all ?? node.any ?? [];
const withChildren = (node: ReviewPredicateNode, children: ReviewPredicateNode[]): ReviewPredicateNode => groupMode(node) === "any" ? { any: children } : { all: children };

function updateAtPath(node: ReviewPredicateNode, path: number[], update: (current: ReviewPredicateNode) => ReviewPredicateNode): ReviewPredicateNode {
  if (path.length === 0) return update(node);
  const children = [...groupChildren(node)];
  const index = path[0];
  if (!children[index]) return node;
  children[index] = updateAtPath(children[index], path.slice(1), update);
  return withChildren(node, children);
}

function countLeaves(node: ReviewPredicateNode): number {
  if (node.leaf) return 1;
  return groupChildren(node).reduce((total, child) => total + countLeaves(child), 0);
}

function isValidTimeZone(value: string): boolean {
  if (!value || value === "system") return false;
  try {
    new Intl.DateTimeFormat("en", { timeZone: value }).format(0);
    return true;
  } catch {
    return false;
  }
}

function cleanCondition(value: Condition, defaultCurrency: string): Condition {
  if (value.metric === "type") return { metric: "type", operator: "in", types: value.types?.length ? value.types : [InstrumentType.InstrumentBankInvestmentProduct] };
  if (value.metric === "instrument") return { metric: "instrument", operator: value.operator === "not_in" ? "not_in" : "in", instrumentIds: value.instrumentIds ?? [] };
  if (value.metric === "price_change" || value.metric === "annualized") return { metric: value.metric, operator: value.operator || "<", value: value.value ?? "", lookbackDays: value.lookbackDays ?? 30 };
  return { metric: value.metric, operator: value.operator || ">", value: value.value ?? "", currency: value.currency ?? (defaultCurrency || undefined), fxMode: value.fxMode ?? "native" };
}

function cleanPredicate(node: ReviewPredicateNode, defaultCurrency: string): ReviewPredicateNode {
  if (node.leaf) return { leaf: cleanCondition(node.leaf, defaultCurrency) };
  const children = groupChildren(node).map(child => cleanPredicate(child, defaultCurrency));
  return groupMode(node) === "any" ? { any: children } : { all: children };
}

function seedMissingCurrencies(node: ReviewPredicateNode, currency: string): ReviewPredicateNode {
  if (node.leaf) {
    if ((node.leaf.metric === "amount" || node.leaf.metric === "cost") && node.leaf.currency === undefined) {
      return { leaf: { ...node.leaf, currency } };
    }
    return node;
  }
  const children = groupChildren(node).map(child => seedMissingCurrencies(child, currency));
  return groupMode(node) === "any" ? { any: children } : { all: children };
}

function isPredicateNode(value: unknown, depth = 0, leaves = { count: 0 }): value is ReviewPredicateNode {
  if (value === null || typeof value !== "object" || Array.isArray(value)) return false;
  const node = value as Record<string, unknown>;
  const shapes = ["all", "any", "leaf"].filter(key => Object.prototype.hasOwnProperty.call(node, key));
  if (shapes.length !== 1 || Object.keys(node).length !== 1) return false;
  if (shapes[0] === "leaf") {
    if (depth === 0 || node.leaf === null || typeof node.leaf !== "object" || Array.isArray(node.leaf)) return false;
    const leaf = node.leaf as Record<string, unknown>;
    leaves.count++;
    return leaves.count <= 20 && typeof leaf.metric === "string" && typeof leaf.operator === "string";
  }
  if (depth >= 2) return false;
  const children = node[shapes[0]];
  return Array.isArray(children) && children.length > 0 && children.length <= 20 && children.every(child => isPredicateNode(child, depth + 1, leaves));
}

function annualizedLeavesAreScoped(node: ReviewPredicateNode, bankOnly = false): boolean {
  if (node.leaf) return node.leaf.metric !== "annualized" || bankOnly;
  const children = groupChildren(node);
  const scoped = groupMode(node) === "all" && children.some(child => child.leaf?.metric === "type" && child.leaf.types?.length === 1 && child.leaf.types[0] === InstrumentType.InstrumentBankInvestmentProduct);
  return children.every(child => annualizedLeavesAreScoped(child, bankOnly || scoped));
}

function currencyIsValid(value: string, currencies: string[], currencyCatalogLoaded: boolean): boolean {
  return currencyCodePattern.test(value) && (!currencyCatalogLoaded || currencies.includes(value));
}

function predicateIsValid(node: ReviewPredicateNode, defaultCurrency: string, currencies: string[], currencyCatalogLoaded: boolean): boolean {
  let leafCount = 0;
  const visit = (current: ReviewPredicateNode, depth: number): boolean => {
    const shapes = Number(current.all !== undefined) + Number(current.any !== undefined) + Number(current.leaf !== undefined);
    if (shapes !== 1) return false;
    if (current.leaf) {
      if (depth === 0) return false;
      leafCount++;
      if (leafCount > 20) return false;
      const leaf = current.leaf;
      if (leaf.metric === "type") return (leaf.types?.length ?? 0) > 0;
      if (leaf.metric === "instrument") return (leaf.instrumentIds?.length ?? 0) > 0;
      if (!leaf.value?.trim()) return false;
      if (leaf.metric === "amount" || leaf.metric === "cost") return moneyThresholdPattern.test(leaf.value.trim()) && currencyIsValid(leaf.currency ?? defaultCurrency, currencies, currencyCatalogLoaded) && Boolean(leaf.fxMode);
      if (leaf.metric === "price_change" || leaf.metric === "annualized") return percentageThresholdPattern.test(leaf.value.trim()) && [7, 30, 90, 180, 365].includes(leaf.lookbackDays ?? 0);
      return false;
    }
    if (depth >= 2) return false;
    const children = groupChildren(current);
    return children.length >= 1 && children.length <= 20 && children.every(child => visit(child, depth + 1));
  };
  return visit(node, 0) && annualizedLeavesAreScoped(node);
}

function makeSpec(timezone: string, targetKind: string, target: string[], predicateNode: ReviewPredicateNode, policy: string, count: number, windowDays: number, minHits: number): ReviewRuleSpec {
  return {
    specVersion: 1, metricPolicyVersion: "daily_metrics_v1", evaluator: "daily_predicate_v1",
    target: targetKind === "instruments" ? { kind: "instruments", instrumentIds: target } : { kind: "all_current" },
    predicate: predicateNode,
    schedule: { period: "calendar_day", timezone, cutoffVersion: "civil_day_end_v1" },
    triggerPolicy: policy === "consecutive" ? { kind: policy, count } : policy === "rolling" ? { kind: policy, windowDays, minHits } : { kind: "single" },
    dataPolicy: { missing: "unknown", calendar: "all_civil_days", quoteCarry: "forbid_for_daily_signal", baselineMaxGapDays: 7, fxReferenceMaxAgeDays: 7, fxTimestampMaxAgeHours: 12 },
  };
}

function ReviewLeafEditor({ leaf, depth, index, idPath, canRemove, instruments, currencies, currencyCatalogLoaded, defaultCurrency, change, remove }: {
  leaf: Condition;
  depth: number;
  index: number;
  idPath: string;
  canRemove: boolean;
  instruments: Array<{ id: string; name: string }>;
  currencies: string[];
  currencyCatalogLoaded: boolean;
  defaultCurrency: string;
  change: (value: Partial<Condition>) => void;
  remove: () => void;
}) {
  const { t } = useTranslation();
  const fieldId = `review-condition-${idPath}-${index}`;
  const metricChanged = (metric: string) => {
    if (metric === "type") change({ metric, operator: "in", types: [InstrumentType.InstrumentBankInvestmentProduct] });
    else if (metric === "instrument") change({ metric, operator: "in", instrumentIds: instruments[0] ? [instruments[0].id] : [] });
    else if (metric === "annualized" || metric === "price_change") change({ metric, operator: "<", value: "0", lookbackDays: 30 });
    else change({ metric, operator: ">", value: "1000", currency: defaultCurrency || undefined, fxMode: "native" });
  };
  const knownInstrumentIds = new Set(instruments.map(item => item.id));
  const missingInstrumentIds = (leaf.instrumentIds ?? []).filter(id => !knownInstrumentIds.has(id));
  const selectionInvalid = leaf.metric === "type" ? !(leaf.types?.length) : leaf.metric === "instrument" ? !(leaf.instrumentIds?.length) : false;
  const valueInvalid = leaf.metric === "amount" || leaf.metric === "cost"
    ? !leaf.value?.trim() || !moneyThresholdPattern.test(leaf.value.trim())
    : leaf.metric === "price_change" || leaf.metric === "annualized"
      ? !leaf.value?.trim() || !percentageThresholdPattern.test(leaf.value.trim())
      : false;
  const currentCurrency = leaf.currency ?? defaultCurrency;
  const currencyOptions = Array.from(new Set([...currencies, ...(defaultCurrency ? [defaultCurrency] : []), ...(currentCurrency ? [currentCurrency] : [])]));
  const currencyInvalid = (leaf.metric === "amount" || leaf.metric === "cost") && !currencyIsValid(currentCurrency, currencies, currencyCatalogLoaded);
  const currentLookbackDays = leaf.lookbackDays ?? 30;
  const lookbackInvalid = (leaf.metric === "price_change" || leaf.metric === "annualized") && !supportedLookbackDays.includes(currentLookbackDays as typeof supportedLookbackDays[number]);
  const selectionErrorId = `${fieldId}-selection-error`;
  const valueErrorId = `${fieldId}-value-error`;
  const currencyErrorId = `${fieldId}-currency-error`;
  const lookbackErrorId = `${fieldId}-lookback-error`;
  return <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-card p-2">
    <NativeSelect aria-label={t("reviewV2.metric")} value={leaf.metric} onChange={event => metricChanged(event.target.value)}>
      <option value="type">{t("reviewV2.type")}</option><option value="instrument">{t("reviewV2.instrumentCondition")}</option>
      <option value="amount">{t("reviewV2.amount")}</option><option value="cost">{t("reviewV2.cost")}</option>
      <option value="price_change">{t("reviewV2.priceChange")}</option><option value="annualized">{t("reviewV2.annualized")}</option>
    </NativeSelect>
    {leaf.metric === "type" ? <><div className="flex max-h-32 flex-wrap gap-2 overflow-auto" role="group" aria-label={t("reviewV2.type")} aria-invalid={selectionInvalid || undefined} aria-describedby={selectionInvalid ? selectionErrorId : undefined}>{choices.map(choice => <label key={choice} className="flex items-center gap-1 text-xs"><input type="checkbox" checked={leaf.types?.includes(choice) ?? false} onChange={event => change({ types: event.target.checked ? [...(leaf.types ?? []), choice] : (leaf.types ?? []).filter(item => item !== choice) })} />{t(`reviewV2.types.${choice}`)}</label>)}</div><FieldError id={selectionErrorId}>{selectionInvalid ? t("reviewV2.conditionSelectionRequired") : undefined}</FieldError></>
      : leaf.metric === "instrument" ? <>
        <NativeSelect aria-label={t("reviewV2.instrumentOperator")} value={leaf.operator} onChange={event => change({ operator: event.target.value })}><option value="in">{t("reviewV2.includesAny")}</option><option value="not_in">{t("reviewV2.excludesAll")}</option></NativeSelect>
        <><div className="flex max-h-32 flex-wrap gap-2 overflow-auto" role="group" aria-label={t("reviewV2.instrumentCondition")} aria-invalid={selectionInvalid || undefined} aria-describedby={selectionInvalid ? selectionErrorId : undefined}>{instruments.map(item => <label key={item.id} className="flex items-center gap-1 text-xs"><input type="checkbox" checked={leaf.instrumentIds?.includes(item.id) ?? false} onChange={event => change({ instrumentIds: event.target.checked ? [...(leaf.instrumentIds ?? []), item.id] : (leaf.instrumentIds ?? []).filter(id => id !== item.id) })} />{item.name}</label>)}{missingInstrumentIds.map(id => <label key={id} className="flex items-center gap-1 text-xs"><input type="checkbox" checked onChange={() => change({ instrumentIds: (leaf.instrumentIds ?? []).filter(item => item !== id) })} />{t("reviewV2.unavailableSelectedInstrument")}</label>)}</div><FieldError id={selectionErrorId}>{selectionInvalid ? t("reviewV2.conditionSelectionRequired") : undefined}</FieldError></>
      </>
      : <>
        <NativeSelect aria-label={t("reviewV2.operator")} value={leaf.operator} onChange={event => change({ operator: event.target.value })}>{[">", ">=", "<", "<="].map(op => <option key={op} value={op}>{op}</option>)}</NativeSelect>
        <div className="flex flex-col gap-1"><Input aria-label={t("reviewV2.value")} className="w-24" inputMode="decimal" value={leaf.value ?? ""} aria-invalid={valueInvalid || undefined} aria-describedby={valueInvalid ? valueErrorId : undefined} onChange={event => change({ value: event.target.value })} /><FieldError id={valueErrorId}>{valueInvalid ? t("reviewV2.valueInvalid") : undefined}</FieldError></div>
        {leaf.metric === "amount" || leaf.metric === "cost" ? <><div className="flex flex-col gap-1"><NativeSelect aria-label={t("reviewV2.currency")} className="w-24" value={currentCurrency} aria-invalid={currencyInvalid || undefined} aria-describedby={currencyInvalid ? currencyErrorId : undefined} onChange={event => change({ currency: event.target.value })}><option value="">{t("accounts.selectCurrency")}</option>{currencyOptions.map(currency => <option key={currency} value={currency}>{currency}</option>)}</NativeSelect><FieldError id={currencyErrorId}>{currencyInvalid ? t("reviewV2.currencyInvalid") : undefined}</FieldError></div><NativeSelect aria-label={t("reviewV2.fxMode")} value={leaf.fxMode ?? "native"} onChange={event => change({ fxMode: event.target.value })}><option value="native">{t("reviewV2.nativeCurrency")}</option><option value="convert_on_day">{t("reviewV2.convertOnDay")}</option></NativeSelect></> : <><span className="text-sm">%</span><div className="flex flex-col gap-1"><NativeSelect aria-label={t("reviewV2.lookbackDays")} className="w-24" value={String(currentLookbackDays)} aria-invalid={lookbackInvalid || undefined} aria-describedby={lookbackInvalid ? lookbackErrorId : undefined} onChange={event => change({ lookbackDays: Number(event.target.value) })}>{!supportedLookbackDays.includes(currentLookbackDays as typeof supportedLookbackDays[number]) && <option value={currentLookbackDays}>{currentLookbackDays} {t("reviewV2.days")}</option>}{supportedLookbackDays.map(days => <option key={days} value={days}>{days} {t("reviewV2.days")}</option>)}</NativeSelect><FieldError id={lookbackErrorId}>{lookbackInvalid ? t("reviewV2.lookbackInvalid") : undefined}</FieldError></div><span className="text-sm">{t("reviewV2.days")}</span></>}
      </>}
    <Button type="button" variant="ghost" disabled={!canRemove} aria-label={t("reviewV2.removeConditionFromGroup", { group: depth + 1, index: index + 1 })} onClick={remove}>{t("reviewV2.remove")}</Button>
  </div>;
}

function PredicateGroupEditor({ node, path, depth, tree, onTreeChange, instruments, currencies, currencyCatalogLoaded, defaultCurrency, invalid = false }: {
  node: ReviewPredicateNode;
  path: number[];
  depth: number;
  tree: ReviewPredicateNode;
  onTreeChange: PredicateTreeChange;
  instruments: Array<{ id: string; name: string }>;
  currencies: string[];
  currencyCatalogLoaded: boolean;
  defaultCurrency: string;
  invalid?: boolean;
}) {
  const { t } = useTranslation();
  const children = groupChildren(node);
  const leafCount = countLeaves(tree);
  const rootChildren = (next: ReviewPredicateNode[], group: ReviewPredicateNode = node) => withChildren(group, next);
  const changeChild = (index: number, update: (child: ReviewPredicateNode) => ReviewPredicateNode) => onTreeChange(current => updateAtPath(current, path, group => {
    const next = [...groupChildren(group)];
    next[index] = update(next[index]);
    return withChildren(group, next);
  }));
  const removeChild = (index: number) => onTreeChange(current => updateAtPath(current, path, group => rootChildren(groupChildren(group).filter((_, childIndex) => childIndex !== index), group)));
  const updateGroup = (update: (group: ReviewPredicateNode) => ReviewPredicateNode) => onTreeChange(current => updateAtPath(current, path, update));
  const addLeaf = () => updateGroup(group => withChildren(group, [...groupChildren(group), { leaf: defaultCondition(defaultCurrency) }]));
  const addConditionGroup = () => updateGroup(group => {
    const child: ReviewPredicateNode = { all: [{ leaf: defaultCondition(defaultCurrency) }] };
    return withChildren(group, [...groupChildren(group), child]);
  });
  const removeGroup = () => {
    if (depth > 0 && path.length > 0) onTreeChange(current => updateAtPath(current, path.slice(0, -1), parent => withChildren(parent, groupChildren(parent).filter((_, index) => index !== path[path.length - 1]))));
  };
  const groupErrorId = "review-predicate-error";
  return <fieldset className={`space-y-3 rounded-lg border border-border p-3 ${depth === 1 ? "ml-3 bg-muted/30" : "bg-card"}`} aria-label={t(depth === 0 ? "reviewV2.rootConditionGroup" : "reviewV2.nestedConditionGroup")} aria-invalid={depth === 0 && invalid || undefined} aria-describedby={depth === 0 && invalid ? groupErrorId : undefined}>
    <div className="flex flex-wrap items-center justify-between gap-2">
      <NativeSelect aria-label={t("reviewV2.groupMode", { level: depth + 1 })} value={groupMode(node)} onChange={event => updateGroup(group => event.target.value === "any" ? { any: groupChildren(group) } : { all: groupChildren(group) })}>
        <option value="all">{t("reviewV2.allConditions")}</option><option value="any">{t("reviewV2.anyCondition")}</option>
      </NativeSelect>
      {depth > 0 && <Button type="button" variant="ghost" onClick={removeGroup}>{t("reviewV2.removeGroup")}</Button>}
    </div>
    <div className="space-y-2">{children.map((child, index) => isGroup(child) ? <PredicateGroupEditor key={`${path.join(".")}.${index}`} node={child} path={[...path, index]} depth={depth + 1} tree={tree} onTreeChange={onTreeChange} instruments={instruments} currencies={currencies} currencyCatalogLoaded={currencyCatalogLoaded} defaultCurrency={defaultCurrency} /> : <ReviewLeafEditor key={`${path.join(".")}.${index}`} leaf={child.leaf ?? defaultCondition(defaultCurrency)} depth={depth} index={index} idPath={path.length ? path.join("-") : "root"} canRemove={children.length > 1} instruments={instruments} currencies={currencies} currencyCatalogLoaded={currencyCatalogLoaded} defaultCurrency={defaultCurrency} change={value => changeChild(index, current => ({ leaf: { ...(current.leaf ?? defaultCondition(defaultCurrency)), ...value } }))} remove={() => removeChild(index)} />)}</div>
    {depth === 0 && <FieldError id={groupErrorId}>{invalid ? t("reviewV2.invalidRule") : undefined}</FieldError>}
    <div className="flex flex-wrap gap-2">
      <Button type="button" variant="outline" aria-label={t("reviewV2.addConditionToGroup", { level: depth + 1 })} disabled={children.length >= 20 || leafCount >= 20} onClick={addLeaf}>{t("reviewV2.addCondition")}</Button>
      {depth === 0 && <Button type="button" variant="outline" onClick={addConditionGroup} disabled={children.length >= 20 || leafCount >= 20}>{t("reviewV2.addConditionGroup")}</Button>}
    </div>
  </fieldset>;
}

export function ReviewRuleEditor({ open, onOpenChange, rule }: { open: boolean; onOpenChange: (open: boolean) => void; rule?: ReviewRule | null }) {
  const { t } = useTranslation();
  const instruments = useInstruments();
  const catalog = useCatalog();
  const bootstrap = useBootstrap();
  const historyOrigin = useHistoryOrigin();
  const commands = useReviewV2Commands();
  const ruleSpec = rule?.spec;
  const defaultCurrency = bootstrap.data?.household?.baseCurrency ?? "";
  const currencies = catalog.data?.currencies ?? [];
  const householdTimezone = historyOrigin.data?.timezone ?? "";
  const sourcePredicateIsValid = !rule || isPredicateNode(ruleSpec?.predicate);
  const [name, setName] = useState(rule?.name ?? "");
  const [enabled, setEnabled] = useState(rule?.enabled ?? true);
  const [severity, setSeverity] = useState(rule?.severity ?? "notice");
  const [timezone, setTimezone] = useState<string | null>(ruleSpec?.schedule?.timezone ?? null);
  const selectedTimezone = timezone ?? householdTimezone;
  const [targetKind, setTargetKind] = useState(ruleSpec?.target?.kind ?? "all_current");
  const [target, setTarget] = useState<string[]>(ruleSpec?.target?.instrumentIds ?? []);
  const [predicateTree, setPredicateTree] = useState<ReviewPredicateNode>(() => isPredicateNode(ruleSpec?.predicate) ? ruleSpec.predicate : defaultPredicate(defaultCurrency));
  const [predicateSourceValid, setPredicateSourceValid] = useState(sourcePredicateIsValid);
  const changePredicateTree = (next: ReviewPredicateNode | ((current: ReviewPredicateNode) => ReviewPredicateNode)) => {
    setPredicateTree(next);
    setPredicateSourceValid(true);
  };
  const [policy, setPolicy] = useState(ruleSpec?.triggerPolicy?.kind ?? "single");
  const [count, setCount] = useState(ruleSpec?.triggerPolicy?.count ?? 3);
  const [windowDays, setWindowDays] = useState(ruleSpec?.triggerPolicy?.windowDays ?? 10);
  const [minHits, setMinHits] = useState(ruleSpec?.triggerPolicy?.minHits ?? 5);
  const [previewInstrument, setPreviewInstrument] = useState(ruleSpec?.target?.instrumentIds?.[0] ?? "");
  const [previewKey, setPreviewKey] = useState("");
  const [preview, setPreview] = useState<Awaited<ReturnType<typeof Service.PreviewReviewRuleV2>> | null>(null);
  const [error, setError] = useState("");
  const [previewError, setPreviewError] = useState("");
  const selectedTimeZoneOptions = useMemo(() => {
    if (!isValidTimeZone(selectedTimezone) || timeZoneSelectOptions.some(option => option.value === selectedTimezone)) return timeZoneSelectOptions;
    return [{ value: selectedTimezone, label: selectedTimezone }, ...timeZoneSelectOptions];
  }, [selectedTimezone]);
  const predicateTreeWithCurrencyDefaults = useMemo(() => defaultCurrency ? seedMissingCurrencies(predicateTree, defaultCurrency) : predicateTree, [predicateTree, defaultCurrency]);
  const cleanedPredicate = useMemo(() => cleanPredicate(predicateTree, defaultCurrency), [predicateTree, defaultCurrency]);
  const spec = useMemo(() => makeSpec(selectedTimezone, targetKind, target, cleanedPredicate, policy, count, windowDays, minHits), [selectedTimezone, targetKind, target, cleanedPredicate, policy, count, windowDays, minHits]);
  const timezoneInitialized = rule ? Boolean(ruleSpec?.schedule?.timezone) : Boolean(householdTimezone);
  const nameInvalid = !name.trim();
  const timezoneInvalid = !timezoneInitialized || !isValidTimeZone(selectedTimezone);
  const targetInvalid = targetKind === "instruments" && !target.length;
  const currencyCatalogLoaded = catalog.data !== undefined;
  const predicateInvalid = !predicateSourceValid || !predicateIsValid(predicateTree, defaultCurrency, currencies, currencyCatalogLoaded);
  const countInvalid = policy === "consecutive" && (!Number.isInteger(count) || count < 1 || count > 365);
  const windowInvalid = policy === "rolling" && (!Number.isInteger(windowDays) || windowDays < 1 || windowDays > 365);
  const minHitsInvalid = policy === "rolling" && (!Number.isInteger(minHits) || minHits < 1 || minHits > windowDays);
  const invalid = nameInvalid || timezoneInvalid || predicateInvalid || targetInvalid || countInvalid || windowInvalid || minHitsInvalid;
  const save = async () => {
    setError("");
    try {
      const request = { name: name.trim(), enabled, severity, spec };
      if (rule) await commands.updateRule.mutateAsync({ ...request, ruleId: rule.id, expectedRevision: rule.rowRevision });
      else await commands.createRule.mutateAsync(request);
      onOpenChange(false);
    } catch (cause) { setError(String(cause)); }
  };
  const inputKey = JSON.stringify([spec, previewInstrument]);
  // Clear visible output during the input render, so A→B→A cannot revive it.
  const [previousInput, setPreviousInput] = useState(inputKey + open);
  if (previousInput !== inputKey + open) {
    setPreviousInput(inputKey + open);
    setPreview(null);
    setPreviewError("");
  }
  const previewGeneration = useRef({ value: 0 });
  useLayoutEffect(() => {
    const generation = previewGeneration.current;
    generation.value++;
    return () => { generation.value++; };
  }, [inputKey, open]);
  const runPreview = async () => {
    const generation = ++previewGeneration.current.value;
    const key = inputKey;
    setPreviewError(""); setPreview(null);
    try {
      const result = await callService(() => Service.PreviewReviewRuleV2({ spec, instrumentId: previewInstrument, latestClosedOn: "", expectedSourceToken: "" }));
      if (previewGeneration.current.value === generation) {
        setPreview(result);
        setPreviewKey(key);
      }
    } catch (cause) {
      if (previewGeneration.current.value === generation) setPreviewError(String(cause));
    }
  };
  return <Sheet open={open} onOpenChange={onOpenChange}><SheetContent size="xl"><SheetHeader><SheetTitle>{t(rule ? "reviewV2.editRule" : "reviewV2.newRule")}</SheetTitle></SheetHeader>
    <div className="space-y-5 pb-5">
      <div className="grid gap-2"><Label htmlFor="review-rule-name">{t("reviewV2.name")}</Label><Input id="review-rule-name" value={name} onChange={event => setName(event.target.value)} maxLength={120} aria-invalid={nameInvalid || undefined} aria-describedby={nameInvalid ? "review-rule-name-error" : undefined} /><FieldError id="review-rule-name-error">{nameInvalid ? t("reviewV2.nameRequired") : undefined}</FieldError></div>
      <div className="flex flex-wrap gap-4"><label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={enabled} onChange={event => setEnabled(event.target.checked)} />{t("reviewV2.enabled")}</label><label className="flex items-center gap-2 text-sm">{t("reviewV2.severity")}<NativeSelect value={severity} onChange={event => setSeverity(event.target.value)}><option value="notice">{t("reviewV2.notice")}</option><option value="warning">{t("reviewV2.warning")}</option></NativeSelect></label></div>
      <fieldset className="space-y-2" aria-invalid={targetInvalid || undefined} aria-describedby={targetInvalid ? "review-rule-target-error" : undefined}><legend className="font-medium">{t("reviewV2.target")}</legend><p className="text-xs text-muted-foreground">{t("reviewV2.positiveHoldings")}</p><NativeSelect aria-label={t("reviewV2.targetMode")} value={targetKind} aria-invalid={targetInvalid || undefined} aria-describedby={targetInvalid ? "review-rule-target-error" : undefined} onChange={event => { const kind = event.target.value; setTargetKind(kind); if (kind === "instruments" && !target.length && instruments.data?.[0]) setTarget([instruments.data[0].id]); }}><option value="all_current">{t("reviewV2.allCurrent")}</option><option value="instruments">{t("reviewV2.selectedInstruments")}</option></NativeSelect>
      {targetKind === "instruments" && <div className="max-h-36 overflow-auto rounded-lg border p-2">{(instruments.data ?? []).map(item => <label key={item.id} className="flex gap-2 p-1 text-sm"><input type="checkbox" checked={target.includes(item.id)} onChange={event => setTarget(ids => event.target.checked ? [...ids, item.id] : ids.filter(id => id !== item.id))} />{item.name}</label>)}</div>}<FieldError id="review-rule-target-error">{targetInvalid ? t("reviewV2.targetRequired") : undefined}</FieldError></fieldset>
      <fieldset className="space-y-3"><legend className="font-medium">{t("reviewV2.dailyConditions")}</legend><p className="text-xs text-muted-foreground">{t("reviewV2.predicateBuilderHelp")}</p>{catalog.isError && <p role="alert" className="text-sm text-destructive">{t("reviewV2.currencyCatalogError")} <Button type="button" variant="link" size="sm" className="h-auto p-0" onClick={() => void catalog.refetch()}>{t("common.retry")}</Button></p>}<PredicateGroupEditor node={predicateTreeWithCurrencyDefaults} path={[]} depth={0} tree={predicateTreeWithCurrencyDefaults} onTreeChange={changePredicateTree} instruments={instruments.data ?? []} currencies={currencies} currencyCatalogLoaded={currencyCatalogLoaded} defaultCurrency={defaultCurrency} invalid={predicateInvalid} /></fieldset>
      <fieldset className="space-y-3"><legend className="font-medium">{t("reviewV2.trigger")}</legend><NativeSelect aria-label={t("reviewV2.trigger")} value={policy} onChange={event => setPolicy(event.target.value)}><option value="single">{t("reviewV2.single")}</option><option value="consecutive">{t("reviewV2.consecutive")}</option><option value="rolling">{t("reviewV2.rolling")}</option></NativeSelect>
      {policy === "consecutive" && <div className="flex flex-col gap-1"><label className="flex items-center gap-2 text-sm">{t("reviewV2.consecutiveDays")}<Input className="w-20" type="number" min={1} max={365} value={count} aria-invalid={countInvalid || undefined} aria-describedby={countInvalid ? "review-rule-count-error" : undefined} onChange={event => setCount(Number(event.target.value))} /></label><FieldError id="review-rule-count-error">{countInvalid ? t("reviewV2.triggerValueInvalid") : undefined}</FieldError></div>}
      {policy === "rolling" && <div className="flex flex-wrap gap-3"><div className="flex flex-col gap-1"><label className="flex items-center gap-2 text-sm">{t("reviewV2.windowDays")}<Input className="w-20" type="number" min={1} max={365} value={windowDays} aria-invalid={windowInvalid || undefined} aria-describedby={windowInvalid ? "review-rule-window-error" : undefined} onChange={event => setWindowDays(Number(event.target.value))} /></label><FieldError id="review-rule-window-error">{windowInvalid ? t("reviewV2.triggerValueInvalid") : undefined}</FieldError></div><div className="flex flex-col gap-1"><label className="flex items-center gap-2 text-sm">{t("reviewV2.minHits")}<Input className="w-20" type="number" min={1} max={windowDays} value={minHits} aria-invalid={minHitsInvalid || undefined} aria-describedby={minHitsInvalid ? "review-rule-hits-error" : undefined} onChange={event => setMinHits(Number(event.target.value))} /></label><FieldError id="review-rule-hits-error">{minHitsInvalid ? t("reviewV2.triggerValueInvalid") : undefined}</FieldError></div></div>}</fieldset>
      <div className="space-y-2"><Label htmlFor="review-rule-timezone">{t("reviewV2.timezone")}</Label><FilterableSelect id="review-rule-timezone" className="w-64" aria-label={t("reviewV2.timezone")} value={selectedTimezone} options={selectedTimeZoneOptions} noOptionsLabel={t("common.noMatches")} aria-invalid={timezoneInvalid || undefined} aria-describedby={timezoneInvalid ? "review-rule-timezone-help review-rule-timezone-error" : "review-rule-timezone-help"} onValueChange={setTimezone} /><p id="review-rule-timezone-help" className="text-xs text-muted-foreground">{t("reviewV2.dailyHelp", { timezone: selectedTimezone || "…" })}</p><FieldError id="review-rule-timezone-error">{timezoneInvalid ? t("reviewV2.timezoneInvalid") : undefined}</FieldError></div>
      <div className="space-y-2 rounded-lg border p-3"><p className="font-medium">{t("reviewV2.preview")}</p><NativeSelect aria-label={t("reviewV2.previewInstrument")} value={previewInstrument} onChange={event => setPreviewInstrument(event.target.value)}><option value="">{t("reviewV2.selectInstrument")}</option>{(instruments.data ?? []).filter(item => !target.length || target.includes(item.id)).map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</NativeSelect><Button type="button" variant="outline" disabled={invalid || !previewInstrument} onClick={() => void runPreview()}>{t("reviewV2.preview")}</Button>{preview && previewKey === inputKey && <p className="text-sm">{t("reviewV2.previewResult", { hits: preview.window.hitCount, misses: preview.window.falseCount, unknowns: preview.window.unknownCount, range: formatCivilDateRange(preview.window.windowStart, preview.window.windowEnd) })}</p>}</div>
      {previewError && <FieldError>{previewError}</FieldError>}{error && <FieldError>{error}</FieldError>}{invalid && <FieldError>{t("reviewV2.invalidRule")}</FieldError>}
      <div className="flex justify-end gap-2"><Button type="button" variant="outline" onClick={() => onOpenChange(false)}>{t("common.cancel")}</Button><Button disabled={invalid || commands.createRule.isPending || commands.updateRule.isPending} onClick={() => void save()}>{t("common.save")}</Button></div>
    </div>
  </SheetContent></Sheet>;
}
