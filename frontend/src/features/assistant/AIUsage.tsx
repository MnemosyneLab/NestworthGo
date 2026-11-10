import { useState } from "react";
import { ChevronDown, RefreshCw } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import * as API from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service";
import { callService } from "@/lib/wails";
import { NativeSelect } from "@/components/ui/select";
import { DatePickerControl } from "@/components/ui/date-picker-control";
import { Button } from "@/components/ui/button";
import { compareCanonical, formatAmount, formatCount } from "@/lib/money";
import { Label } from "@/components/ui/label";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { Notice } from "@/components/ui/notice";
import { EmptyState } from "@/components/layout/PageState";
import { cn } from "@/lib/utils";

type Total = Awaited<ReturnType<typeof API.Usage>>["total"];
// Fixed K/M/B units in every language; only presentation is rounded.
const compactTokens = new Intl.NumberFormat("en", { notation: "compact", maximumFractionDigits: 1 });
function count(value: number | string, missing: number, calls: number) {
  return <UsageValue value={value} missing={missing} calls={calls} />;
}
function UsageValue({ value, missing, calls, money = false }: { value: number | string; missing: number; calls: number; money?: boolean }) {
  const { t } = useTranslation();
  if (!calls || missing >= calls) return <span className="text-muted-foreground">{t("aiUsage.unknown")}</span>;
  const exact = money ? `${value} USD` : typeof value === "number" ? formatCount(value) : formatAmount(value);
  const display = money ? usageCost(String(value)) : typeof value === "number" ? compactTokens.format(value) : formatAmount(value);
  return <span className="inline-flex max-w-full flex-wrap items-baseline justify-end gap-x-2 gap-y-1">
    <span title={exact} aria-label={exact} className="num break-words">{display}</span>
    {missing > 0 && <span className="text-xs font-normal text-muted-foreground" title={t("aiUsage.missingRequests", { count: missing })}>{t("aiUsage.partial")}</span>}
  </span>;
}
function usageCost(value: string) {
  return compareCanonical(value, "0") > 0 && compareCanonical(value, "0.01") < 0
    ? `< ${formatAmount("0.01", "USD")}` : formatAmount(value, "USD");
}
export function UsageNumbers({ total }: { total: Total }) {
  const { t } = useTranslation();
  return <dl className="grid grid-cols-1 gap-x-4 gap-y-2 text-xs sm:grid-cols-2">
    {([
      ["input", total.input, total.missingInput], ["output", total.output, total.missingOutput],
      ["cached", total.cachedInput, total.missingCached], ["cacheWrite", total.cacheWrite, total.missingWrite],
      ["reasoning", total.reasoning, total.missingReasoning],
    ] as const).map(([key, value, missing]) => <div className="flex justify-between gap-2" key={key}><dt>{t(`aiUsage.${key}`)}</dt><dd>{count(value, missing, total.calls)}</dd></div>)}
    <div className="flex justify-between gap-2"><dt>{t("aiUsage.estimatedCost")}</dt><dd><UsageValue value={total.estimatedUsd} missing={total.unpricedCalls} calls={total.calls} money /></dd></div>
    {total.pricedCalls > 0 && <div className="col-span-full text-muted-foreground">{t("aiUsage.cacheCosts", { read: usageCost(total.cachedUsd), write: usageCost(total.cacheWriteUsd) })}</div>}
    {total.unpricedCalls > 0 && <div className="col-span-full text-muted-foreground">{t("aiUsage.unpriced", { count: total.unpricedCalls })}</div>}
    <div className="flex justify-between gap-2"><dt>{t("aiUsage.calls")}</dt><dd>{total.calls}</dd></div>
    {total.audioCalls > 0 && <div className="flex justify-between gap-2"><dt>{t("aiUsage.audioSeconds")}</dt><dd>{count(total.audioSeconds, total.missingAudio, total.audioCalls)}</dd></div>}
    {(total.failed > 0 || total.pending > 0) && <p className="col-span-full text-muted-foreground">{t("aiUsage.outcomes", { failed: total.failed, pending: total.pending })}</p>}
  </dl>;
}
const timezone = () => Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
export function ConversationUsage({ epoch, id, runID, stamp }: { epoch: string; id: string; runID: string; stamp: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const total = useQuery({ queryKey: ["aiUsage", epoch, id, stamp], enabled: !!epoch && !!id, networkMode: "always", retry: false,
    queryFn: () => callService(() => API.Usage(epoch, { timezone: timezone(), from: "", to: "", conversationId: id, runId: "", model: "", groupBy: "model" })), refetchInterval: open ? 10000 : false });
  const current = useQuery({ queryKey: ["aiUsageRun", epoch, id, runID, stamp], enabled: !!epoch && !!id && !!runID, networkMode: "always", retry: false,
    queryFn: () => callService(() => API.Usage(epoch, { timezone: timezone(), from: "", to: "", conversationId: id, runId: runID, model: "", groupBy: "model" })), refetchInterval: open ? 10000 : false });
  return <details className="group shrink-0 border-t border-border bg-muted/30 px-3 py-1.5 text-xs text-muted-foreground" open={open} onToggle={e => setOpen(e.currentTarget.open)}>
    <summary className="flex min-h-8 cursor-pointer list-none items-center gap-2 rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden"><span className="min-w-0 flex-1"><span className="@max-xs:sr-only">{t("aiUsage.title")} · </span>{t("aiUsage.current")} {current.data?.available ? count(current.data.total.tokens, current.data.total.missingTokens, current.data.total.calls) : "—"} · {t("aiUsage.total")} {total.data?.available ? count(total.data.total.tokens, total.data.total.missingTokens, total.data.total.calls) : "—"}</span><ChevronDown aria-hidden="true" className="size-3.5 shrink-0 transition-transform group-open:rotate-180" /></summary>
    <div className="mt-2 max-h-48 space-y-3 overflow-auto text-foreground">
      {total.isPending ? <p>{t("aiUsage.loading")}</p> : total.isError || !total.data?.available ? <p role="status">{t("aiUsage.unavailable")}</p> : <>
        {current.data?.available && <div><p className="mb-1 font-medium">{t("aiUsage.current")}</p><UsageNumbers total={current.data.total} /></div>}
        <div><p className="mb-1 font-medium">{t("aiUsage.total")}</p><UsageNumbers total={total.data.total} /></div>
        {(total.data.groups ?? []).map(g => <div key={g.key}><p className="mb-1 font-medium">{g.label}</p><UsageNumbers total={g.total} /></div>)}
        {total.data.incomplete && <p role="status">{t("aiUsage.incomplete")}</p>}
      </>}
      <p className="text-muted-foreground">{t("aiUsage.coverage")}</p>
      <p className="text-muted-foreground">{t("aiUsage.costBasis")}</p>
    </div>
  </details>;
}
const estimated = (total: Total) => <UsageValue value={total.estimatedUsd} missing={total.unpricedCalls} calls={total.calls} money />;

/** Shared values preserve unknown and partial totals across all usage views. */
function UsageSummary({ total }: { total: Total }) {
  const { t } = useTranslation();
  const tiles = [
    { key: "estimatedCost", label: t("aiUsage.estimatedCost"), value: estimated(total) },
    { key: "tokens", label: t("aiUsage.tokens"), value: count(total.tokens, total.missingTokens, total.calls) },
    { key: "calls", label: t("aiUsage.calls"), value: String(total.calls) },
  ];
  const rows = [
    { key: "input", value: total.input, missing: total.missingInput },
    { key: "output", value: total.output, missing: total.missingOutput },
    { key: "cached", value: total.cachedInput, missing: total.missingCached, subset: true },
    { key: "cacheWrite", value: total.cacheWrite, missing: total.missingWrite, subset: true },
    { key: "reasoning", value: total.reasoning, missing: total.missingReasoning, subset: true },
  ];
  return <div className="space-y-3">
    <dl className="grid grid-cols-1 gap-3 sm:grid-cols-3">
      {tiles.map(tile => <div key={tile.key} className="min-w-0 rounded-xl border border-border bg-card p-4">
        <dt className="text-xs text-muted-foreground">{tile.label}</dt>
        <dd className="num mt-1 break-words text-xl font-semibold">{tile.value}</dd>
      </div>)}
    </dl>
    <div className="space-y-1.5">
      <h4 className="text-xs font-medium text-muted-foreground">{t("aiUsage.breakdown")}</h4>
      <dl className="divide-y divide-border rounded-xl border border-border bg-muted/30 text-sm">
        {rows.map(row => <div key={row.key} className={cn("flex items-center justify-between gap-3 px-4 py-2", row.subset && "pl-8")}>
          <dt className={row.subset ? "text-muted-foreground" : undefined}>{t(`aiUsage.${row.key}`)}</dt>
          <dd className="num font-medium">{count(row.value, row.missing, total.calls)}</dd>
        </div>)}
        {total.audioCalls > 0 && <div className="flex items-center justify-between gap-3 px-4 py-2"><dt>{t("aiUsage.audioSeconds")}</dt><dd className="num font-medium">{count(total.audioSeconds, total.missingAudio, total.audioCalls)}</dd></div>}
      </dl>
    </div>
    {(total.pricedCalls > 0 || total.unpricedCalls > 0 || total.failed > 0 || total.pending > 0) && <div className="space-y-0.5 text-xs text-muted-foreground">
      {total.pricedCalls > 0 && <p>{t("aiUsage.cacheCosts", { read: usageCost(total.cachedUsd), write: usageCost(total.cacheWriteUsd) })}</p>}
      {total.unpricedCalls > 0 && <p>{t("aiUsage.unpriced", { count: total.unpricedCalls })}</p>}
      {(total.failed > 0 || total.pending > 0) && <p>{t("aiUsage.outcomes", { failed: total.failed, pending: total.pending })}</p>}
    </div>}
  </div>;
}

export function AIUsageSettings({ epoch }: { epoch: string }) {
  const { t } = useTranslation();
  const [from, setFrom] = useState(""); const [to, setTo] = useState("");
  const [groupBy, setGroupBy] = useState("day"); const [model, setModel] = useState("");
  const [conversation, setConversation] = useState("");
  const [zone, setZone] = useState(timezone());
  const localZone = timezone();
  const report = useQuery({ queryKey: ["aiUsageSettings", epoch, from, to, groupBy, model, zone, conversation], enabled: !!epoch, retry: false, networkMode: "always",
    queryFn: () => callService(() => API.Usage(epoch, { from, to, groupBy, model, timezone: zone, conversationId: conversation, runId: "" })) });
  const conversations = report.data?.conversations ?? [];
  const filtered = !!(from || to || model || conversation);
  const fieldLabel = "mb-1.5 block text-xs text-muted-foreground";
  return <section className="mt-8 space-y-4 border-t border-border pt-6" aria-label={t("aiUsage.title")}>
    <div className="flex items-center justify-between gap-3">
      <h3 className="font-semibold">{t("aiUsage.title")}</h3>
      <Button type="button" variant="outline" size="sm" disabled={report.isFetching} onClick={() => void report.refetch()}>
        <RefreshCw aria-hidden="true" className={cn("size-3.5", report.isFetching && "animate-spin motion-reduce:animate-none")} />{t("aiUsage.refresh")}
      </Button>
    </div>
    <div className="space-y-3 rounded-xl bg-muted/40 p-3 sm:p-4">
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <div className="min-w-0"><Label htmlFor="usage-from" className={fieldLabel}>{t("aiUsage.from")}</Label><DatePickerControl id="usage-from" value={from} onChange={setFrom} /></div>
        <div className="min-w-0"><Label htmlFor="usage-to" className={fieldLabel}>{t("aiUsage.to")}</Label><DatePickerControl id="usage-to" value={to} onChange={setTo} /></div>
        <div className="min-w-0"><Label htmlFor="usage-conversation" className={fieldLabel}>{t("aiUsage.conversation")}</Label><NativeSelect id="usage-conversation" className="w-full" value={conversation} onChange={e => setConversation(e.target.value)}><option value="">{t("aiUsage.allConversations")}</option>{conversations.map((c, i) => <option key={c.key} value={c.key}>{c.deleted ? `${t("aiUsage.deleted")} ${i + 1}` : c.label}</option>)}</NativeSelect></div>
        <div className="min-w-0"><Label htmlFor="usage-model" className={fieldLabel}>{t("aiUsage.model")}</Label><NativeSelect id="usage-model" className="w-full" value={model} onChange={e => setModel(e.target.value)}><option value="">{t("aiUsage.allModels")}</option>{(report.data?.models ?? []).map(m => <option key={m}>{m}</option>)}</NativeSelect></div>
      </div>
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="flex min-w-0 flex-wrap items-end gap-3">
          <div><span className={fieldLabel} aria-hidden="true">{t("aiUsage.group")}</span>
            <SegmentedControl label={t("aiUsage.group")} value={groupBy} onChange={setGroupBy} className="bg-card"
              options={["day", "conversation", "model", "kind"].map(v => ({ value: v, label: t(`aiUsage.${v}`) }))} /></div>
          <div><Label htmlFor="usage-zone" className={fieldLabel}>{t("aiUsage.timezone")}</Label><NativeSelect id="usage-zone" value={zone} onChange={e => setZone(e.target.value)}>{[...new Set([localZone, "UTC"])].map(z => <option key={z}>{z}</option>)}</NativeSelect></div>
        </div>
        <Button type="button" variant="ghost" size="sm" disabled={!filtered} onClick={() => {setFrom("");setTo("");setModel("");setConversation("");}}>{t("aiUsage.clear")}</Button>
      </div>
    </div>
    {report.isPending ? <p role="status" className="text-sm text-muted-foreground">{t("aiUsage.loading")}</p> : report.isError || !report.data?.available ? <Notice tone="warning" role="alert">{t("aiUsage.unavailable")}</Notice> : <>
      {!report.data.total.calls ? <div className="rounded-xl border border-dashed border-border px-4 py-3"><EmptyState size="compact" title={t("aiUsage.empty")} /></div> : null}
      <UsageSummary total={report.data.total} />
      {report.data.incomplete && <Notice tone="warning" role="status">{t("aiUsage.incomplete")}</Notice>}
      {!!(report.data.groups ?? []).length && <ul className="max-h-96 divide-y divide-border overflow-auto rounded-xl border border-border">{(report.data.groups ?? []).map(g => <li key={g.key}><details className="group">
        <summary className="flex min-h-12 cursor-pointer list-none items-center gap-3 rounded-xl px-4 py-2 text-sm hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 [&::-webkit-details-marker]:hidden">
          <ChevronDown aria-hidden="true" className="size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-180" />
          <span className="min-w-0 flex-1 truncate font-medium">{groupBy === "kind" ? t(`aiUsage.${g.label === "title" ? "titleKind" : g.label}`) : g.deleted ? `${t("aiUsage.deleted")} ${conversations.findIndex(c => c.key === g.key) + 1}` : g.label || t("aiUsage.unassigned")}</span>
          <span className="num shrink-0 text-xs text-muted-foreground">{t("aiUsage.requestCount", { count: g.total.calls })}</span>
        </summary>
        <div className="border-t border-border bg-muted/30 px-4 py-3"><UsageNumbers total={g.total} /></div>
      </details></li>)}</ul>}
      {!!report.data.prices?.length && <details className="text-xs text-muted-foreground"><summary className="w-fit cursor-pointer rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50">{t("aiUsage.prices")}</summary><ul className="space-y-1 pt-2">{report.data.prices.map((p, i) => <li key={i}>{p.model} · {p.serviceTier || "ASR"} · {p.version}<br />{p.perMinute ? t("aiUsage.minuteRate", { rate: p.perMinute }) : t("aiUsage.tokenRates", { input: p.input, cached: p.cached, short: p.write30m, long: p.write12h, output: p.output })} · <a href={p.source} target="_blank" rel="noreferrer" className="underline">{p.verifiedAt}</a></li>)}</ul></details>}
    </>}
    <div className="space-y-1 border-t border-border pt-3 text-xs text-muted-foreground">
      <p>{t("aiUsage.settingsCoverage")}</p>
      <p>{t("aiUsage.costBasis")}</p>
    </div>
  </section>;
}
