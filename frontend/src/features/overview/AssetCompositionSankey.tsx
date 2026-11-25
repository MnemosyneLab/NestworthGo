import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { EChart, type EChartsOption } from "@/components/charts/EChart";
import { chartTheme, cssColor, joinTooltipLines, useThemeVersion } from "@/components/charts/chartTheme";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState } from "@/components/layout/PageState";
import { formatAuditMoney } from "@/lib/money";
import { displayEnum } from "@/lib/display";
import type { TFunction } from "i18next";
import type { AssetCompositionDTO, AssetNodeDTO, AssetPathDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio/models";

const MIN_CHART_WIDTH = 620;

export function AssetCompositionSankey({ composition, assets, currency, isActive = true, onOpenAccount }: {
  composition: AssetCompositionDTO;
  assets: string;
  currency: string;
  isActive?: boolean;
  onOpenAccount?: (id: string) => void;
}) {
  const { t } = useTranslation();
  const contentRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  useEffect(() => {
    const content = contentRef.current;
    if (!content) return;
    const observer = new ResizeObserver(([entry]) => {
      const width = Math.floor(entry.contentRect.width);
      if (width > 0) setWidth(width);
    });
    observer.observe(content);
    return () => observer.disconnect();
  }, []);

  const { positive, negative, missingCount, zeroCount } = composition;
  const wide = width >= MIN_CHART_WIDTH;
  const paths = composition.paths ?? [];
  const labels = assetCompositionLabels(paths, t);
  const hasPositive = paths.length > 0;
  const hasNegative = negative !== "0";
  const valued = hasPositive || hasNegative || zeroCount > 0;
  const partial = missingCount > 0;
  const rootLabel = t(hasNegative ? "assetSankey.positive" : partial ? "assetSankey.known" : "assetSankey.total");

  return (
    <Card className="min-w-0">
      <CardHeader>
        <CardTitle>{t("assetSankey.title")}</CardTitle>
        <CardDescription>{t("assetSankey.description")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div ref={contentRef} className="flex min-w-0 flex-col gap-4">
          <div className="space-y-2">
            {(valued || !partial) && (
              <dl className="flex flex-wrap gap-x-6 gap-y-2 text-sm">
                {hasNegative && <>
                  <AmountSummary label={t("assetSankey.positive")} value={positive} currency={currency} />
                  <AmountSummary label={t("assetSankey.negative")} value={negative} currency={currency} />
                </>}
                <AmountSummary label={t(partial ? "assetSankey.known" : "assetSankey.total")} value={assets} currency={currency} />
              </dl>
            )}
            {partial && <p className="text-sm text-warning-foreground">{t("assetSankey.missing", { count: missingCount })}</p>}
            {zeroCount > 0 && <p className="text-sm text-muted-foreground">{t("assetSankey.zero", { count: zeroCount })}</p>}
          </div>
          {hasPositive ? <>
            {wide && isActive && <AssetSankeyChart nodes={composition.nodes ?? []} labels={labels} rootLabel={rootLabel} width={width} currency={currency} onOpenAccount={onOpenAccount} />}
            <details open={!wide}>
              <summary className="w-fit cursor-pointer rounded-lg text-sm text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50">
                {t("assetSankey.details")}
              </summary>
              <table aria-label={t("assetSankey.details")} className="mt-2 w-full table-fixed text-left text-sm">
                <thead className="border-b border-border text-muted-foreground">
                  <tr>
                    <th scope="col" className="py-2 font-medium">{t("assetSankey.allocation")}</th>
                    <th scope="col" className="w-[42%] py-2 pl-3 text-right font-medium">{t("assetSankey.amount")}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border">
                  {paths.map(path => {
                    const account = labels.account(path.accountId);
                    return (
                      <tr key={`${path.assetClass}/${path.accountId}`}>
                        <th scope="row" className="py-2 align-top font-normal [overflow-wrap:anywhere]">
                          {onOpenAccount ? (
                            <Button type="button" variant="link" className="h-auto max-w-full justify-start whitespace-normal p-0 text-left" onClick={() => onOpenAccount(path.accountId)}>{account}</Button>
                          ) : <span className="font-medium">{account}</span>}
                          <div className="text-xs text-muted-foreground">{labels.assetClass(path.assetClass)} · {labels.institution(path.institutionId)}</div>
                        </th>
                        <td className="num py-2 pl-3 text-right align-top [overflow-wrap:anywhere]">{formatAuditMoney(path.amount, currency)}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </details>
          </> : <EmptyState size="compact" title={t(partial && !valued ? "assetSankey.unavailable" : "assetSankey.empty")} />}
        </div>
      </CardContent>
    </Card>
  );
}

function AmountSummary({ label, value, currency }: { label: string; value: string; currency: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="num font-semibold [overflow-wrap:anywhere]">{formatAuditMoney(value, currency)}</dd>
    </div>
  );
}

function distinctLabels(entries: Map<string, string>, t: TFunction): Map<string, string> {
  const groups = new Map<string, string[]>();
  const used = new Set(entries.values());
  const result = new Map(entries);
  for (const [id, name] of entries) {
    const group = groups.get(name) ?? [];
    group.push(id);
    groups.set(name, group);
  }
  for (const [name, ids] of groups) {
    if (ids.length === 1) continue;
    let ordinal = 0;
    for (const id of ids.sort()) {
      let label: string;
      do {
        label = t("assetSankey.duplicateName", { name, ordinal: ++ordinal });
      } while (used.has(label));
      used.add(label);
      result.set(id, label);
    }
  }
  return result;
}

function assetCompositionLabels(paths: AssetPathDTO[], t: TFunction) {
  const accounts = distinctLabels(new Map(paths.map(path => [path.accountId, path.accountName || t("overview.unknownAccount")])), t);
  const institutions = distinctLabels(new Map(paths.map(path => [path.institutionId, path.institutionId
    ? path.institutionName || t("common.unknownValue") : t("overview.unassignedInstitution")])), t);
  return {
    account: (id: string) => accounts.get(id) ?? t("overview.unknownAccount"),
    institution: (id: string) => institutions.get(id) ?? t("common.unknownValue"),
    assetClass: (key: string) => key ? displayEnum(t, "enum", key) : t("common.unknownValue"),
  };
}

function AssetSankeyChart({ nodes, labels, rootLabel, width, currency, onOpenAccount }: {
  nodes: AssetNodeDTO[];
  labels: ReturnType<typeof assetCompositionLabels>;
  rootLabel: string;
  width: number;
  currency: string;
  onOpenAccount?: (id: string) => void;
}) {
  const { t } = useTranslation();
  useThemeVersion();
  const theme = chartTheme();
  const labelSpace = Math.min(218, Math.max(148, Math.floor(width * 0.2)));
  const nodeLabel = (node: AssetNodeDTO) => {
    switch (node.kind) {
      case "assetClass": return labels.assetClass(node.key);
      case "institution": return labels.institution(node.key);
      case "account": return labels.account(node.key);
      case "otherInstitutions": return t("assetSankey.otherInstitutions");
      case "otherAccounts": return t("assetSankey.otherAccounts");
      default: return rootLabel;
    }
  };
  const labelsByID = new Map(nodes.map(node => [node.id, nodeLabel(node)]));
  const nodesByID = new Map(nodes.map(node => [node.id, node]));
  const nodeColors = new Map<string, string>();
  let classIndex = 0;
  for (const node of nodes) {
    const color = node.kind === "assetClass" ? theme.palette[classIndex++ % theme.palette.length]
      : nodeColors.get(node.parentId) ?? theme.primary;
    nodeColors.set(node.id, color);
  }
  const option: EChartsOption = {
    tooltip: {
      trigger: "item",
      formatter: params => {
        if (Array.isArray(params)) return "";
        const data = params.data as { assetNodeId?: string } | undefined;
        const node = data?.assetNodeId ? nodesByID.get(data.assetNodeId) : undefined;
        if (!node) return "";
        const label = labelsByID.get(node.id);
        return joinTooltipLines([
          params.dataType === "edge" ? [labelsByID.get(node.parentId), label].join(" › ") : label,
          formatAuditMoney(node.amount, currency),
        ]);
      },
    },
    series: {
      type: "sankey",
      left: 0, right: labelSpace, top: 8, bottom: 8,
      nodeWidth: 14, nodeGap: 18, draggable: false,
      emphasis: { focus: "trajectory" },
      label: {
        color: theme.foreground,
        width: labelSpace - 22,
        overflow: "truncate",
        textBorderColor: cssColor("--color-card", "#ffffff"),
        textBorderWidth: 3,
        formatter: ({ name }) => labelsByID.get(name) ?? "",
      },
      data: nodes.map(node => ({
        name: node.id,
        assetNodeId: node.id,
        itemStyle: { color: nodeColors.get(node.id), borderWidth: 0, borderRadius: 2 },
        label: { show: Boolean(node.parentId) },
      })),
      links: nodes.filter(node => node.parentId).map(node => ({
        source: node.parentId,
        target: node.id,
        value: Number(node.amount),
        assetNodeId: node.id,
        lineStyle: { color: nodeColors.get(node.id) },
      })),
      lineStyle: { opacity: 0.28, curveness: 0.44 },
    },
  };
  const terminalCount = nodes.filter(node => node.kind === "account" || node.kind === "otherAccounts").length;
  return (
    <div className="flex flex-col gap-1">
      <div aria-hidden="true" className="grid gap-y-1 text-xs font-medium text-muted-foreground" style={{ gridTemplateColumns: `repeat(3, minmax(0, 1fr)) ${labelSpace + 14}px` }}>
        <span>{rootLabel}</span>
        <span>{t("assetSankey.assetClass")}</span>
        <span>{t("assetSankey.institution")}</span>
        <span>{t("assetSankey.account")}</span>
      </div>
      <EChart
        option={option}
        height={Math.max(268, terminalCount * 26 + 100)}
        ariaLabel={t("assetSankey.title")}
        summary={t("assetSankey.description")}
        onSelectName={onOpenAccount ? name => {
          const node = name ? nodesByID.get(name) : undefined;
          if (node?.kind === "account") onOpenAccount(node.key);
        } : undefined}
      />
      {nodes.some(node => node.kind === "otherAccounts") && <p className="text-xs text-muted-foreground">{t("assetSankey.grouped")}</p>}
    </div>
  );
}
