import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Sheet, SheetContent, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { PageIntro } from "@/components/layout/PageHeader";
import { PageChrome } from "@/components/layout/PageChrome";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { displayEnum, displayError } from "@/lib/display";
import { useMarketDataHealth } from "@/queries/marketdata";
import { useMarketDataSyncActions } from "@/features/marketdata/MarketDataSyncBar";
import type { HealthIssueDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata/models";

function visibleIssues(issues: HealthIssueDTO[] | null | undefined): HealthIssueDTO[] {
  return (issues ?? []).filter((issue) => !issue.collapsed);
}

function groupIssues(issues: HealthIssueDTO[]): { kind: string; items: HealthIssueDTO[] }[] {
  const groups = new Map<string, HealthIssueDTO[]>();
  for (const issue of issues) {
    const items = groups.get(issue.kind) ?? [];
    items.push(issue);
    groups.set(issue.kind, items);
  }
  return [...groups.entries()].map(([kind, items]) => ({ kind, items }));
}

function issueRange(t: (key: string, options?: Record<string, unknown>) => string, issue: HealthIssueDTO): string | undefined {
  if (!issue.rangeStart) {
    return undefined;
  }
  if (!issue.rangeEnd || issue.rangeEnd === issue.rangeStart) {
    return issue.rangeStart;
  }
  return t("dataHealth.range", { start: issue.rangeStart, end: issue.rangeEnd });
}

export function DataHealthPage({
  onOpenSettings,
  onOpenMarketData,
  onOpenInvestments,
}: {
  onOpenSettings?: () => void;
  onOpenMarketData?: () => void;
  onOpenInvestments?: () => void;
} = {}) {
  const { t } = useTranslation();
  const health = useMarketDataHealth();
  const sync = useMarketDataSyncActions();
  const [previewOpen, setPreviewOpen] = useState(false);
  const pageChrome = <PageChrome pageId="data-health" title={t("nav.dataHealth")} />;

  const issues = useMemo(() => visibleIssues(health.data?.issues), [health.data?.issues]);
  const executable = issues.filter((issue) => issue.executable);
  const prerequisites = issues.filter((issue) => !issue.executable && issue.action !== "none" && issue.action !== "repair");
  const reported = issues.filter((issue) => !issue.executable && (issue.action === "none" || issue.action === "repair"));

  const runAction = (issue: HealthIssueDTO) => {
    if (issue.action === "provider_settings") {
      onOpenSettings?.();
      return;
    }
    if (issue.action === "instrument_editor") {
      onOpenMarketData?.();
      return;
    }
    if (issue.action === "manual_entry") {
      if (issue.kind === "missing_manual_price") {
        onOpenInvestments?.();
        return;
      }
      onOpenMarketData?.();
    }
  };

  const openPreview = () => {
    sync.preview.mutate(
      { scope: "repair_all" },
      {
        onSuccess: () => setPreviewOpen(true),
        onError: (error) => toast.error(displayError(error, t("dataHealth.loadError"))),
      },
    );
  };

  const confirmRepair = () => {
    setPreviewOpen(false);
    sync.startScope({ scope: "repair_all" });
  };

  if (health.isLoading) {
    return (
      <>
        {pageChrome}
        <LoadingState label={t("ui.state.loadingPage")} />
      </>
    );
  }
  if (health.isError || !health.data) {
    return (
      <>
        {pageChrome}
        <ErrorState
          title={t("dataHealth.loadError")}
          description={t("ui.state.errorDescription")}
          onRetry={() => health.refetch()}
          retryLabel={t("common.retryAction")}
        />
      </>
    );
  }

  const report = health.data;
  const running = sync.running;

  return (
    <div className="flex flex-col gap-6" data-testid="data-health-page">
      {pageChrome}
      <PageIntro description={t("dataHealth.description")} />

      {report.healthy ? (
        <EmptyState title={t("dataHealth.healthyTitle")} description={t("dataHealth.healthyDescription", { date: report.coverageThrough || report.lastFinalizedMarketDate })} />
      ) : (
        <Card>
          <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-3">
            <div className="flex flex-col gap-2">
              <CardTitle>{report.incompleteSince ? t("dataHealth.incompleteSince", { date: report.incompleteSince }) : t("nav.dataHealth")}</CardTitle>
              <p className="text-sm text-muted-foreground">
                {t("dataHealth.summaryGaps", { count: report.issueCount })}
                {report.snapshotDays > 0 ? ` · ${t("dataHealth.summarySnapshots", { count: report.snapshotDays })}` : ""}
                {report.prerequisiteCount > 0 ? ` · ${t("dataHealth.summaryPrerequisites", { count: report.prerequisiteCount })}` : ""}
              </p>
            </div>
            <Button type="button" onClick={openPreview} disabled={running || sync.preview.isPending || sync.start.isPending} data-testid="repair-all">
              {sync.preview.isPending ? t("common.pending") : t("dataHealth.repairAll")}
            </Button>
          </CardHeader>
          <CardContent className="flex flex-col gap-3 text-sm">
            {report.snapshotDays === 0 && issues.some((issue) => issue.kind.startsWith("snapshot") && issue.collapsed) ? (
              <p className="text-muted-foreground">{t("dataHealth.collapsedSnapshots")}</p>
            ) : null}
            {running && sync.current.data ? (
              <p role="status" data-testid="data-health-sync-progress">
                {t("marketData.syncingMarketData")} {displayEnum(t, "marketData.phase", sync.current.data.phase)}{" "}
                {t("marketData.syncProgress", { completed: sync.current.data.completedTargets, total: sync.current.data.targetCount })}
              </p>
            ) : null}
          </CardContent>
        </Card>
      )}

      <IssueSection title={t("dataHealth.executableSection")} issues={executable} t={t} onAction={runAction} />
      <IssueSection title={t("dataHealth.prerequisiteSection")} issues={prerequisites} t={t} onAction={runAction} />
      <IssueSection title={t("dataHealth.otherSection")} issues={reported} t={t} onAction={runAction} />

      <Sheet open={previewOpen} onOpenChange={setPreviewOpen}>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>{t("dataHealth.repairTitle")}</SheetTitle>
            <p className="text-sm text-muted-foreground">{t("dataHealth.repairDescription")}</p>
          </SheetHeader>
          {sync.preview.data && (
            <ul className="flex flex-col gap-2 text-sm" data-testid="repair-preview">
              <li>{t("marketData.previewRequests", { count: sync.preview.data.estimatedRequestCount })}</li>
              <li>{t("marketData.previewInstruments", { count: sync.preview.data.instrumentTargets })}</li>
              <li>{t("marketData.previewFx", { count: sync.preview.data.fxTargets })}</li>
              <li>{t("marketData.previewLatest", { instruments: sync.preview.data.latestInstrumentCount, fx: sync.preview.data.latestFxCount })}</li>
              <li>{t("marketData.previewSnapshots", { count: sync.preview.data.snapshotWorkEstimate })}</li>
            </ul>
          )}
          {(sync.preview.data?.unresolved ?? []).length > 0 && (
            <div className="flex flex-col gap-2">
              <p className="text-sm font-medium">{t("marketData.previewUnresolved")}</p>
              <ul className="flex flex-col gap-1 text-sm text-muted-foreground">
                {(sync.preview.data?.unresolved ?? []).map((item) => (
                  <li key={`${item.targetKey}-${item.code}`}>{t("marketData.blocker", { target: item.targetKey, reason: item.reason || item.code })}</li>
                ))}
              </ul>
            </div>
          )}
          <SheetFooter>
            <Button type="button" variant="outline" onClick={() => setPreviewOpen(false)}>
              {t("common.cancel")}
            </Button>
            <Button type="button" onClick={confirmRepair} disabled={sync.start.isPending} data-testid="confirm-repair">
              {t("dataHealth.confirmRepair")}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </div>
  );
}

function IssueSection({
  title,
  issues,
  t,
  onAction,
}: {
  title: string;
  issues: HealthIssueDTO[];
  t: (key: string, options?: Record<string, unknown>) => string;
  onAction: (issue: HealthIssueDTO) => void;
}) {
  if (issues.length === 0) {
    return null;
  }
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-lg font-semibold">{title}</h2>
      {groupIssues(issues).map((group) => (
        <Card key={group.kind}>
          <CardHeader>
            <CardTitle className="flex items-center justify-between gap-2">
              <span>{displayEnum(t, "dataHealth.kind", group.kind)}</span>
              <Badge variant="secondary">{group.items.length}</Badge>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="flex flex-col gap-3">
              {group.items.map((issue) => (
                <li key={issue.id} className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between" data-testid={`health-issue-${issue.kind}`}>
                  <div className="flex min-w-0 flex-col gap-1">
                    <p className="font-medium text-foreground">{issue.label || issue.targetKey}</p>
                    <p className="text-sm text-muted-foreground">
                      {[issueRange(t, issue), issue.provider, displayEnum(t, "dataHealth.action", issue.action)].filter(Boolean).join(" · ")}
                    </p>
                  </div>
                  {issue.action === "provider_settings" && (
                    <Button type="button" size="sm" variant="outline" onClick={() => onAction(issue)}>
                      {t("dataHealth.openSettings")}
                    </Button>
                  )}
                  {issue.action === "instrument_editor" && (
                    <Button type="button" size="sm" variant="outline" onClick={() => onAction(issue)}>
                      {t("dataHealth.openInstrument")}
                    </Button>
                  )}
                  {issue.action === "manual_entry" && (
                    <Button type="button" size="sm" variant="outline" onClick={() => onAction(issue)}>
                      {t("dataHealth.addManual")}
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      ))}
    </section>
  );
}
