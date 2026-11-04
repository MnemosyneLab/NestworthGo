import { lazy, Suspense, useCallback, useEffect, useLayoutEffect, useState, useRef, type ReactNode } from "react";
import { AppShell, DEFAULT_PAGE_ID } from "@/app/AppShell";
import { OnboardingPage } from "@/features/onboarding/OnboardingPage";
import { OverviewPage } from "@/features/overview/OverviewPage";
import { AccountsPage } from "@/features/accounts/AccountsPage";
import { BlockedStartupPage, StartupLoadingPage } from "@/features/startup/BlockedStartupPage";
import { LoadingState } from "@/components/layout/PageState";
import { useTranslation } from "react-i18next";
import { useBootstrap } from "@/queries/household";
import { useStartup } from "@/queries/app";
import { useSettings } from "@/queries/settings";
import { useUiStore, type Appearance, type Accent } from "@/stores/ui";
import { setLanguage } from "@/i18n";
import { reconciliationFocusFromTarget, resolveLegacyTarget, targetForPage, type HistoryNavigationFilters, type NavigationTarget, type PageId, type LiquidityFocus, type HistoricalNavigationContext, type InstrumentDetailContext, type PortfolioView, type ReconciliationNavigationFocus, type TasksTab } from "@/app/navigation";
import { readCachedTasksAttention, resolveTasksTab } from "@/features/tasks/useTasksCounts";
import { useQueryClient } from "@tanstack/react-query";
import { useAnalysisStore } from "@/stores/analysis";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { hasUnresolvedSheetConfirmation } from "@/components/ui/sheet-confirmation";
import { NavigationContext } from "@/app/NavigationContext";
import type { HealthFocus, AccountListFocus } from "@/app/navigation";
import { destinationKey, revisitTarget, appendVisit, recordRecent, TASK_SESSION_LIMIT, type RevisitableDestination } from "@/app/navigationHistory";
import type { HistoricalCaptureContext } from "@/queries/historicalCapture";
import { toast } from "sonner";
import { AgentWorkspaceObserver } from "@/queries/agent";
import { MarketDataSyncWorkspaceObserver } from "@/queries/marketdata";

const HistoricalOverviewPage = lazy(() => import("@/features/historical-overview/HistoricalOverviewPage").then(module => ({ default: module.HistoricalOverviewPage })));
const PortfolioPage = lazy(() => import("@/features/portfolio/PortfolioPage").then((module) => ({ default: module.PortfolioPage })));
const InstrumentDetailPage = lazy(() => import("@/features/investments/InstrumentDetailPage").then((module) => ({ default: module.InstrumentDetailPage })));
const DirectoryPage = lazy(() => import("@/features/directory/DirectoryPage").then((module) => ({ default: module.DirectoryPage })));
const MarketDataPage = lazy(() => import("@/features/marketdata/MarketDataPage").then((module) => ({ default: module.MarketDataPage })));
const DataHealthPage = lazy(() => import("@/features/data-health/DataHealthPage").then((module) => ({ default: module.DataHealthPage })));
const SettingsPage = lazy(() => import("@/features/settings/SettingsPage").then((module) => ({ default: module.SettingsPage })));
const HistoryPage = lazy(() => import("@/features/history/HistoryPage").then((module) => ({ default: module.HistoryPage })));
const ReturnAnalysisPage = lazy(() => import("@/features/insights/ReturnAnalysisPage").then((module) => ({ default: module.ReturnAnalysisPage })));
const AssetChangesPage = lazy(() => import("@/features/insights/AssetChangesPage").then((module) => ({ default: module.AssetChangesPage })));
const TasksPage = lazy(() => import("@/features/tasks/TasksPage").then(module => ({ default: module.TasksPage })));
const AvailableFundsPage = lazy(() => import("@/features/liquidity/AvailableFundsPage").then((module) => ({ default: module.AvailableFundsPage })));

function WorkspaceLazy({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  return <Suspense fallback={<LoadingState label={t("ui.state.loadingPage")} />}>{children}</Suspense>;
}

/**
 * App gates on AppService.Startup() before any other bound service so a
 * failed database open renders BlockedStartupPage instead of a raw Wails
 * "service not found" rejection. Onboarding runs until a Household exists.
 *
 * Object-navigation source pages remain mounted until the user returns. Overview stays a static import
 * so the default landing page has no Suspense flash. Other routes load
 * through dynamic imports; server state stays in TanStack Query.
 */
function Workspace() {
  const { t } = useTranslation();
  const [activePageId, setActivePageId] = useState<PageId>(DEFAULT_PAGE_ID);
  const [selectedAccountId, setSelectedAccountId] = useState<string | null>(null);
  const [portfolioView, setPortfolioView] = useState<PortfolioView>("holdings");
  const [tasksTab, setTasksTab] = useState<TasksTab | null>(null);
  const [accountCreateRequestId, setAccountCreateRequestId] = useState(0);
  const nextAccountCreateRequestId = useRef(0);
  const [returnToHoldingsAfterCreate, setReturnToHoldingsAfterCreate] = useState(false);
  const [holdingDraftRequest, setHoldingDraftRequest] = useState<{ requestId: number; accountId: string }>();
  const nextHoldingDraftRequest = useRef(0);
  const [instrumentDetail, setInstrumentDetail] = useState<InstrumentDetailContext>();
  const [reconciliationRequestId, setReconciliationRequestId] = useState(0);
  const nextReconciliationRequestId = useRef(0);
  const [retainedReconciliation, setRetainedReconciliation] = useState(() => new Map<number, ReconciliationNavigationFocus>());
  const [reconciliationFocus, setReconciliationFocus] = useState<ReconciliationNavigationFocus>({});
  const [historyFilters, setHistoryFilters] = useState<HistoryNavigationFilters | undefined>();
  const [historyActivityId, setHistoryActivityId] = useState<string>();
  const [historyRequestId, setHistoryRequestId] = useState(0);
  const nextHistoryRequestId = useRef(0);
  const handleHistoryFiltersChange = useCallback((filters: HistoryNavigationFilters) => setHistoryFilters(filters), []);
  const [accountFilter, setAccountFilter] = useState<AccountListFocus>();
  const [updateValue, setUpdateValue] = useState(false);
  const [healthFocus, setHealthFocus] = useState<HealthFocus>();
  const [marketFocus, setMarketFocus] = useState<HealthFocus>();
  const [marketFocusRequestId, setMarketFocusRequestId] = useState(0);
  const [marketSearchQuery, setMarketSearchQuery] = useState("");
  const nextMarketFocusRequestId = useRef(0);
  const [visitedMaintenance, setVisitedMaintenance] = useState({ market: false, health: false });
  const [historicalDate, setHistoricalDate] = useState<string>();
  const [historicalContext, setHistoricalContext] = useState<HistoricalNavigationContext>();
  const [historicalCapture, setHistoricalCapture] = useState<HistoricalCaptureContext>();
  const handleHistoricalContextChange = useCallback((date: string, context: HistoricalNavigationContext, capture: HistoricalCaptureContext) => {
    setHistoricalDate(date);
    setHistoricalCapture(previous => JSON.stringify(previous) === JSON.stringify(capture) ? previous : capture);
    setHistoricalContext(previous => JSON.stringify(previous) === JSON.stringify(context) ? previous : context);
  }, []);
  const [historicalRequest, setHistoricalRequest] = useState(0);
  const nextHistoricalRequest = useRef(0);
  const [liquidityProductId, setLiquidityProductId] = useState<string | undefined>();
  const [liquidityAccountId, setLiquidityAccountId] = useState<string | undefined>();
  const [liquidityFocus, setLiquidityFocus] = useState<LiquidityFocus>();
  const [liquidityRequestId, setLiquidityRequestId] = useState(0);
  const nextLiquidityRequestId = useRef(0);
  type Snapshot = { historicalCapture?: HistoricalCaptureContext; historicalDate?: string; historicalContext?: HistoricalNavigationContext; historicalRequest: number; page: PageId; accountId: string | null; filter?: AccountListFocus; health?: HealthFocus; market?: HealthFocus; marketFocusRequestId: number; marketSearchQuery: string; portfolioView: PortfolioView; tasksTab: TasksTab | null; reconciliationRequestId: number; reconciliationFocus: ReconciliationNavigationFocus; instrumentDetail?: InstrumentDetailContext; scroll: number; focus: HTMLElement | null; analysis: ReturnType<typeof useAnalysisStore.getState>; history?: HistoryNavigationFilters; historyActivityId?: string; historyRequestId?: number; liquidityProduct?: string; liquidityAccount?: string; liquidityFocus?: LiquidityFocus; liquidityRequest?: number };
  type Entry = { id: number; contextualBack?: boolean; target: RevisitableDestination; snapshot?: Snapshot };
  const [entries, setEntries] = useState<Entry[]>([{ id: 0, target: { page: DEFAULT_PAGE_ID } }]);
  const [cursor, setCursor] = useState(0);
  const [recent, setRecent] = useState<Entry[]>([]);
  const [retainedTaskVisits, setRetainedTaskVisits] = useState(() => new Map<number, Entry>());
  const nextEntryId = useRef(0);
  const activeEntryId = entries[cursor]?.id;
  const trail = entries.slice(0, cursor).flatMap(entry => entry.snapshot ? [entry.snapshot] : []);
  const allSnapshots = [...entries, ...recent].flatMap(entry => entry.snapshot ? [entry.snapshot] : []);
  const restorePosition = useRef<{ scroll: number; focus: HTMLElement | null; focusId?: string; page?: PageId; entryId?: number } | null>(null);
  useLayoutEffect(() => {
    const position = restorePosition.current;
    // External stores may commit the cursor before the destination page. Keep
    // the request until both match, including across cancelled focus frames.
    if (position && ((position.page && position.page !== activePageId) || (position.entryId !== undefined && position.entryId !== activeEntryId))) return;
    const main = document.getElementById("main-content");
    if (main) main.scrollTop = position?.scroll ?? 0;
    const frame = requestAnimationFrame(() => {
      if (restorePosition.current !== position) return;
      restorePosition.current = null;
      if (document.querySelector('[role="menu"]')) return;
      const connected = position?.focus?.isConnected && !position.focus.closest('[hidden], [inert], [aria-hidden="true"]') ? position.focus : null;
      const byId = document.getElementById(position?.focusId ?? position?.focus?.id ?? "");
      const focus = connected ?? (byId && !byId.closest('[hidden], [inert], [aria-hidden="true"]') ? byId : null);
      const fallback = document.getElementById(activePageId === "tasks" ? "tasks-heading" : activePageId === "overview" ? "overview-upcoming-actions-heading" : "main-content");
      (focus ?? fallback)?.focus({ preventScroll: true });
    });
    // Scroll is already restored before paint. New input only cancels the
    // deferred focus, so it cannot steal focus from a newly opened menu.
    const cancelRestore = () => {
      cancelAnimationFrame(frame);
      if (restorePosition.current === position) restorePosition.current = null;
    };
    document.addEventListener("pointerdown", cancelRestore, { capture: true, once: true });
    document.addEventListener("keydown", cancelRestore, { capture: true, once: true });
    document.addEventListener("focusin", cancelRestore, { capture: true, once: true });
    return () => {
      cancelAnimationFrame(frame);
      document.removeEventListener("pointerdown", cancelRestore, true);
      document.removeEventListener("keydown", cancelRestore, true);
      document.removeEventListener("focusin", cancelRestore, true);
    };
  }, [activePageId, cursor, activeEntryId]);
  const startup = useStartup();
  const settings = useSettings({ enabled: startup.data?.available === true });
  const setAccent = useUiStore((state) => state.setAccent);
  const setAppearance = useUiStore((state) => state.setAppearance);
  const queryClient = useQueryClient();
  const setReturnView = useAnalysisStore((state) => state.setReturnView);
  const setAssetView = useAnalysisStore((state) => state.setAssetView);

  useEffect(() => {
    if (!settings.data) {
      return;
    }
    setAppearance(settings.data.appearance as Appearance);
    setAccent(settings.data.accent as Accent);
    setLanguage(settings.data.language);
  }, [settings.data, setAppearance, setAccent]);

  const workspaceReady = startup.data?.available === true && Boolean(settings.data);
  const bootstrap = useBootstrap({ enabled: workspaceReady });

  const confirmationUnresolved = hasUnresolvedSheetConfirmation();
  if (startup.isLoading) {
    return <StartupLoadingPage label={t("startup.loading")} />;
  }
  if (!confirmationUnresolved && (startup.isError || (startup.data && !startup.data.available))) {
    return <BlockedStartupPage startup={startup.data} failure={startup.error} />;
  }
  if (settings.isError && !confirmationUnresolved) {
    return <BlockedStartupPage failure={settings.error} />;
  }
  if (settings.isLoading || !settings.data) {
    return <StartupLoadingPage label={t("ui.state.loadingWorkspace")} />;
  }
  if (bootstrap.isLoading) {
    return <StartupLoadingPage label={t("ui.state.loadingWorkspace")} />;
  }
  if (bootstrap.isError && !confirmationUnresolved) {
    return <BlockedStartupPage failure={bootstrap.error} />;
  }

  if (bootstrap.data && !bootstrap.data.household && !confirmationUnresolved) {
    return <OnboardingPage onCompleted={() => setActivePageId("accounts")} />;
  }

  const openAccount = (accountId: string) => open({ page: "accounts", accountId });
  const applyTarget = (target: NavigationTarget | PageId) => {
    if (hasUnresolvedSheetConfirmation()) return;
    const navigation = resolveLegacyTarget(typeof target === "string" ? targetForPage(target) : target);
    const pageId = navigation.page;
    setAccountCreateRequestId(0);
    setUpdateValue(false);
    if (navigation.page === "tasks") {
      const cached = readCachedTasksAttention(queryClient);
      const nextTab = resolveTasksTab({ explicit: navigation.tab, previous: tasksTab, attention: cached.attention, actionable: cached.actionable });
      setTasksTab(nextTab);
      setReconciliationFocus(reconciliationFocusFromTarget(navigation));
      const requestId = ++nextReconciliationRequestId.current;
      setRetainedReconciliation(previous => new Map(previous).set(requestId, reconciliationFocusFromTarget(navigation)));
      setReconciliationRequestId(requestId);
    }
    if (navigation.page === "historical-overview") { setHistoricalDate(navigation.date); setHistoricalContext(navigation.historical); setHistoricalCapture(undefined); setHistoricalRequest(++nextHistoricalRequest.current); }
    // Top-level Accounts navigation always returns to the list. Opening a
    // specific account uses openAccount, which sets selectedAccountId first.
    if (pageId === "accounts") {
      if (navigation.page === "accounts" && navigation.create) setAccountCreateRequestId(++nextAccountCreateRequestId.current);
      setSelectedAccountId(navigation.page === "accounts" ? navigation.accountId ?? null : null);
      setAccountFilter(navigation.page === "accounts" ? navigation.filter : undefined);
      setUpdateValue(navigation.page === "accounts" && navigation.updateValue === true);
    }
    if (navigation.page === "market-data") {
      setMarketFocus(navigation.focus);
      const nextRequestId = ++nextMarketFocusRequestId.current;
      const nextSearch = navigation.search ?? (navigation.focus?.instrumentId ? "" : marketSearchQuery);
      setMarketSearchQuery(nextSearch);
      setMarketFocusRequestId(nextRequestId);
      setVisitedMaintenance(previous => ({ ...previous, market: true }));
    }
    if (navigation.page === "data-health") { setHealthFocus(navigation.focus); setVisitedMaintenance(previous => ({ ...previous, health: true })); }
    if (navigation.page === "portfolio") setPortfolioView(navigation.view ?? "holdings");
    if (navigation.page === "instrument-detail") setInstrumentDetail({ instrumentId: navigation.instrumentId,
      accountId: navigation.accountId, section: navigation.section, from: navigation.from, to: navigation.to, healthFocus: navigation.healthFocus });
    if (navigation.page === "available-funds") {
      setLiquidityProductId(navigation.productId);
      setLiquidityAccountId(navigation.accountId);
      setLiquidityFocus(navigation.focus);
      setLiquidityRequestId(++nextLiquidityRequestId.current);
    }
    if (navigation.page === "history") {
      setHistoryFilters(navigation.filters);
      setHistoryActivityId(navigation.activityId);
      setHistoryRequestId(++nextHistoryRequestId.current);
    }
    if (navigation.page === "return-analysis") {
      if (navigation.analysis) useAnalysisStore.getState().replaceFilters(navigation.analysis);
      setReturnView({ tab: navigation.tab, cursor: navigation.cursor });
    } else if (navigation.page === "asset-changes") {
      if (navigation.analysis) useAnalysisStore.getState().replaceFilters(navigation.analysis);
      setAssetView({ tab: navigation.tab, cursor: navigation.cursor });
    }
    setActivePageId(pageId);
  };

  const capture = (): Snapshot => ({ historicalCapture, historicalDate, historicalContext, historicalRequest, page: activePageId, accountId: selectedAccountId, filter: accountFilter, portfolioView, tasksTab, reconciliationRequestId, reconciliationFocus, instrumentDetail,
      health: healthFocus, market: marketFocus, scroll: document.getElementById("main-content")?.scrollTop ?? 0,
      marketFocusRequestId, marketSearchQuery, focus: document.activeElement instanceof HTMLElement ? document.activeElement : null, analysis: useAnalysisStore.getState(), history: historyFilters, historyRequestId, liquidityProduct: liquidityProductId, liquidityAccount: liquidityAccountId, liquidityFocus, liquidityRequest: liquidityRequestId });
  const targetFromSnapshot = (snapshot: Snapshot): NavigationTarget => {
    switch (snapshot.page) {
      case "accounts": return { page: "accounts", accountId: snapshot.accountId ?? undefined, filter: snapshot.filter };
      case "tasks": return { page: "tasks", tab: snapshot.tasksTab ?? "upcoming", ...snapshot.reconciliationFocus };
      case "portfolio": return { page: "portfolio", view: snapshot.portfolioView };
      case "history": return { page: "history", filters: snapshot.history };
      case "instrument-detail": return { page: "instrument-detail", ...snapshot.instrumentDetail! };
      case "historical-overview": return { page: "historical-overview", date: snapshot.historicalDate, historical: snapshot.historicalContext };
      case "market-data": return { page: "market-data", focus: snapshot.market, search: snapshot.marketSearchQuery };
      case "data-health": return { page: "data-health", focus: snapshot.health };
      case "available-funds": return { page: "available-funds", accountId: snapshot.liquidityAccount, productId: snapshot.liquidityProduct, focus: snapshot.liquidityFocus };
      case "return-analysis": case "asset-changes": {
        const state = snapshot.analysis;
        const analysis = { scope: state.scope, scopeId: state.scopeId, valuation: state.valuation, includeCash: state.includeCash, from: state.from, to: state.to, moreFilters: state.moreFilters, returnType: state.contributionReturnType };
        return snapshot.page === "return-analysis" ? { page: snapshot.page, tab: state.returnTab, cursor: state.returnCursor, analysis } : { page: snapshot.page, tab: state.assetTab, cursor: state.assetCursor, analysis };
      }
      default: return targetForPage(snapshot.page);
    }
  };
  const saveCurrent = () => {
    const snapshot = capture();
    const entry = { ...entries[cursor], target: revisitTarget(targetFromSnapshot(snapshot)), snapshot };
    if (snapshot.page === "tasks" && retainedReconciliation.has(snapshot.reconciliationRequestId)) setRetainedTaskVisits(previous => new Map(previous).set(snapshot.reconciliationRequestId, entry));
    return entry;
  };
  const visit = (target: NavigationTarget, snapshot?: Snapshot, contextualBack = true) => {
    if (hasUnresolvedSheetConfirmation()) return;
    const source = saveCurrent();
    const resolved = structuredClone(resolveLegacyTarget(target));
    if (resolved.page === "tasks" && !resolved.tab) {
      const cached = readCachedTasksAttention(queryClient);
      resolved.tab = resolveTasksTab({ previous: tasksTab, attention: cached.attention, actionable: cached.actionable });
    }
    // Conservatively retain Tasks workspaces: their local editable drafts have no discard API.
    // Refuse a 21st session rather than silently evicting a user's unfinished reconciliation.
    if (!snapshot && resolved.page === "tasks" && retainedReconciliation.size >= TASK_SESSION_LIMIT) {
      toast.info(t("navigationHistory.sessionLimit"));
      return;
    }
    const destination = { id: ++nextEntryId.current, contextualBack, target: revisitTarget(resolved), snapshot };
    if (destinationKey(source.target) === destinationKey(destination.target) && !("create" in target && target.create) && !("updateValue" in target && target.updateValue) && resolved.page !== "tasks") return;
    if (snapshot?.page === "historical-overview") {
      // Fork the page's editable UI state while its capture handle keeps the selected
      // immutable query result alive. Back/Forward retain their own page hosts.
      destination.snapshot = { ...snapshot, historicalRequest: ++nextHistoricalRequest.current };
    }
    const next = appendVisit(entries, cursor, source, destination);
    setEntries(next);
    setCursor(next.length - 1);
    setRecent(previous => recordRecent(previous.filter(entry => entry.id !== source.id), source));
    setReturnToHoldingsAfterCreate(target.page === "accounts" && target.create === true && activePageId === "portfolio");
    if (destination.snapshot) restore(destination.snapshot, destination.id);
    else {
      restorePosition.current = { scroll: 0, focus: null, focusId: restorePosition.current?.focusId, page: resolved.page, entryId: destination.id };
      applyTarget(resolved);
    }
  };
  const open = (target: NavigationTarget) => visit(target);
  const handleNavigate = (target: NavigationTarget | PageId) => visit(typeof target === "string" ? targetForPage(target) : target, undefined, false);
  const openHealth = (focus: HealthFocus) => {
    if (focus.accountId && !focus.instrumentId && !focus.currencyA) open({ page: "accounts", accountId: focus.accountId, updateValue: true });
    else if (focus.action === "provider_settings") open({ page: "settings" });
    else if (focus.instrumentId || (focus.currencyA && focus.currencyB)) open({ page: "market-data", focus });
    else open({ page: "data-health", focus });
  };
  const updateMarketSearchQuery = (query: string) => {
    setMarketSearchQuery(query);
  };
  const restore = (previous: Snapshot, entryId: number) => {
    setReturnToHoldingsAfterCreate(false);
    setAccountCreateRequestId(0);
    restorePosition.current = { ...previous, page: previous.page, entryId };
    setHistoricalCapture(previous.historicalCapture);
    setHistoricalDate(previous.historicalDate); setHistoricalContext(previous.historicalContext); setHistoricalRequest(previous.historicalRequest);
    useAnalysisStore.setState(previous.analysis); setHistoryFilters(previous.history); setHistoryActivityId(previous.historyActivityId); setHistoryRequestId(previous.historyRequestId ?? 0);
    setSelectedAccountId(previous.accountId); setAccountFilter(previous.filter); setUpdateValue(false);
    setHealthFocus(previous.health); setMarketFocus(previous.market);
    setMarketFocusRequestId(previous.marketFocusRequestId);
    setMarketSearchQuery(previous.marketSearchQuery);
    setPortfolioView(previous.portfolioView); setTasksTab(previous.tasksTab); setReconciliationFocus(previous.reconciliationFocus); setReconciliationRequestId(previous.reconciliationRequestId); setInstrumentDetail(previous.instrumentDetail);
    setLiquidityProductId(undefined); setLiquidityAccountId(undefined); setLiquidityFocus(undefined); setLiquidityRequestId(previous.liquidityRequest ?? 0);
    setActivePageId(previous.page);
  };
  const move = (offset: number) => {
    if (hasUnresolvedSheetConfirmation()) return;
    const index = cursor + offset;
    const destination = entries[index];
    if (!destination?.snapshot) return;
    const source = saveCurrent();
    setEntries(previous => previous.map((entry, i) => i === cursor ? source : entry));
    setRecent(previous => recordRecent(previous.filter(entry => entry.id !== source.id), source));
    setCursor(index);
    restore(destination.snapshot, destination.id);
  };
  const back = () => move(-1);
  const selectAccount = (accountId: string | null) => {
    if (hasUnresolvedSheetConfirmation()) return;
    if (accountId) { visit({ page: "accounts", accountId }, undefined, false); return; }
    const previous = entries[cursor - 1]?.snapshot;
    if (previous?.page === "accounts" && !previous.accountId) { back(); return; }
    // The dedicated account-list return keeps the original cross-page source.
    const snapshot = { ...capture(), accountId: null, scroll: 0, focus: null };
    const replacement = { ...entries[cursor], target: revisitTarget({ page: "accounts", filter: accountFilter }), snapshot };
    setEntries(previous => previous.map((entry, index) => index === cursor ? replacement : entry));
    restore(snapshot, replacement.id);
  };
  const onAccountCreated = (accountId: string, tracksHoldings: boolean) => {
    if (!returnToHoldingsAfterCreate || trail[trail.length - 1]?.page !== "portfolio") return false;
    if (!tracksHoldings) { setReturnToHoldingsAfterCreate(false); return false; }
    back();
    setHoldingDraftRequest({ requestId: ++nextHoldingDraftRequest.current, accountId });
    return true;
  };
  // Tasks entries own editable sessions, including Forward and discarded branches.
  // Keep them until the ledger closes; the bounded admission check protects drafts.
  const reconciliationEntries = new Map(retainedReconciliation);
  for (const entry of allSnapshots) {
    if (entry.page === "tasks") reconciliationEntries.set(entry.reconciliationRequestId, entry.reconciliationFocus);
  }
  reconciliationEntries.set(reconciliationRequestId, reconciliationFocus);
  const retained = (page: PageId) => activePageId === page || (page === "tasks" && retainedReconciliation.size > 0) || (!(page === "overview" && (activePageId === "historical-overview" || activePageId === "settings")) && allSnapshots.some(entry => entry.page === page));
  return (
    <NavigationContext.Provider value={{ open, openHealth }}>
    <AppShell activePageId={activePageId} onNavigate={handleNavigate} settings={settings.data} navigationHistory={{
      canBack: cursor > 0, canForward: cursor < entries.length - 1,
      back, forward: () => move(1),
      sessions: Array.from(retainedTaskVisits).reverse().map(([requestId, entry]) => ({ id: `tasks-${requestId}`, ordinal: requestId, target: entry.target, onSelect: () => visit(entry.target, entry.snapshot, false) })),
      recent: recent.map(entry => ({ id: String(entry.id), target: entry.target, onSelect: () => visit(entry.target, entry.snapshot, false) })),
    }}>
      <MarketDataSyncWorkspaceObserver />
      <AgentWorkspaceObserver />
      {entries[cursor].contextualBack && trail.length > 0 && <Button variant="ghost" className="mb-4" onClick={back}>{t("connections.back", { page: trail[trail.length - 1].page === "instrument-detail" ? t("instrumentDetail.title") : t(`nav.${({ "historical-overview": "historicalOverview", "data-health": "dataHealth", "market-data": "marketData", "return-analysis": "returnAnalysis", "asset-changes": "assetChanges", "available-funds": "availableFunds", tasks: "tasks" } as Record<string, string>)[trail[trail.length - 1].page] ?? trail[trail.length - 1].page}`) })}</Button>}
      {retained("overview") && (
        <div hidden={activePageId !== "overview"}>
        <OverviewPage
          isActive={activePageId === "overview"}
          onAddAccount={() => handleNavigate("accounts")}
          onOpenAccounts={() => handleNavigate("accounts")}
          onOpenHistory={() => handleNavigate("history")}
          onOpenMarketData={() => handleNavigate("market-data")}
          onOpenDataHealth={() => handleNavigate("data-health")}
        />
        </div>
      )}
      {retained("accounts") && (
        <div hidden={activePageId !== "accounts"}>
        <AccountsPage navigationFilter={accountFilter} onClearFilter={() => setAccountFilter(undefined)} updateValue={updateValue} selectedAccountId={selectedAccountId} onSelectAccount={selectAccount} createRequestId={accountCreateRequestId} onCreated={onAccountCreated} />
        </div>
      )}
      {retained("available-funds") && (
        <div hidden={activePageId !== "available-funds"}>
        <WorkspaceLazy>
          <AvailableFundsPage productId={liquidityProductId} accountId={liquidityAccountId} focus={liquidityFocus} navigationRequestId={liquidityRequestId} isActive={activePageId === "available-funds"} />
        </WorkspaceLazy>
        </div>
      )}
      {retained("tasks") && <div hidden={activePageId !== "tasks"}><WorkspaceLazy><TasksPage isActive={activePageId === "tasks"} tab={tasksTab ?? "upcoming"} onTabChange={setTasksTab} accountId={reconciliationFocus.accountId} sessionId={reconciliationFocus.sessionId} navigationRequestId={reconciliationRequestId} reconciliationEntries={Array.from(reconciliationEntries, ([navigationRequestId, focus]) => ({ ...focus, navigationRequestId }))} /></WorkspaceLazy></div>}
      {retained("portfolio") && (
        <div hidden={activePageId !== "portfolio"}>
        <WorkspaceLazy>
          <PortfolioPage onOpenAccount={openAccount} view={portfolioView} onViewChange={setPortfolioView} active={activePageId === "portfolio"} holdingDraftRequest={holdingDraftRequest} />
        </WorkspaceLazy>
        </div>
      )}
      {activePageId === "instrument-detail" && instrumentDetail && <WorkspaceLazy>
        <InstrumentDetailPage key={`${instrumentDetail.instrumentId}:${instrumentDetail.accountId ?? ""}`} context={instrumentDetail} onContextChange={setInstrumentDetail} />
      </WorkspaceLazy>}
      {retained("directory") && (
        <div hidden={activePageId !== "directory"}>
        <WorkspaceLazy>
          <DirectoryPage />
        </WorkspaceLazy>
        </div>
      )}
      {(retained("market-data") || retained("data-health")) && <div hidden={activePageId !== "market-data" && activePageId !== "data-health"}>
        <Tabs value={activePageId === "market-data" || activePageId === "data-health" ? activePageId : "none"}
          onValueChange={(value) => {
            if (hasUnresolvedSheetConfirmation()) return;
            setVisitedMaintenance(previous => ({ ...previous, [value === "data-health" ? "health" : "market"]: true }));
            open({ page: value === "data-health" ? "data-health" : "market-data" });
          }}>
          <TabsList aria-label={t("nav.marketData")}>
            <TabsTrigger value="data-health">{t("dataMaintenance.issues")}</TabsTrigger>
            <TabsTrigger value="market-data">{t("dataMaintenance.quotes")}</TabsTrigger>
          </TabsList>
          <TabsContent value="market-data" keepMounted>
            {visitedMaintenance.market && <WorkspaceLazy>
              <MarketDataPage focus={marketFocus} focusRequestId={marketFocusRequestId} searchQuery={marketSearchQuery} onSearchQueryChange={updateMarketSearchQuery} isActive={activePageId === "market-data"} onOpenDataHealth={() => open({ page: "data-health", focus: marketFocus })} />
            </WorkspaceLazy>}
          </TabsContent>
          <TabsContent value="data-health" keepMounted>
            {visitedMaintenance.health && <WorkspaceLazy>
              <DataHealthPage focus={healthFocus}
                onOpenSettings={() => handleNavigate("settings")}
                onOpenMarketData={() => open({ page: "market-data", focus: healthFocus })}
              />
            </WorkspaceLazy>}
          </TabsContent>
        </Tabs>
      </div>}
      {activePageId === "settings" && (
        <WorkspaceLazy>
          <SettingsPage />
        </WorkspaceLazy>
      )}
      {Array.from(new Map<number, { historicalDate?: string; historicalContext?: HistoricalNavigationContext; historicalCapture?: HistoricalCaptureContext }>([...allSnapshots.filter(entry => entry.page === "historical-overview").map(entry => [entry.historicalRequest, entry] as const), ...(activePageId === "historical-overview" ? [[historicalRequest, { historicalDate, historicalContext, historicalCapture }] as const] : [])])).map(([request, entry]) => <div key={request} hidden={activePageId !== "historical-overview" || request !== historicalRequest}><WorkspaceLazy><HistoricalOverviewPage initialDate={entry.historicalDate} initialContext={entry.historicalContext} initialCapture={entry.historicalCapture} onContextChange={request === historicalRequest && activePageId === "historical-overview" ? handleHistoricalContextChange : undefined} isActive={activePageId === "historical-overview" && request === historicalRequest} onExit={() => {
        if (cursor > 0) back();
        else { restorePosition.current = { scroll: 0, focus: null, focusId: "open-historical-overview" }; handleNavigate("overview"); }
      }} /></WorkspaceLazy></div>)}
      {activePageId === "history" && (
        <WorkspaceLazy>
          <HistoryPage navigationFilters={historyFilters} navigationActivityId={historyActivityId} navigationRequestId={historyRequestId} onFiltersChange={handleHistoryFiltersChange} onNavigationActivityClose={() => setHistoryActivityId(undefined)} />
        </WorkspaceLazy>
      )}
      {retained("return-analysis") && (
        <div hidden={activePageId !== "return-analysis"}>
        <WorkspaceLazy>
          <ReturnAnalysisPage onOpenAssetChanges={(analysis) => open({ page: "asset-changes", tab: "drivers", analysis })} onOpenHistory={(filters) => open({ page: "history", filters })} />
        </WorkspaceLazy>
        </div>
      )}
      {retained("asset-changes") && (
        <div hidden={activePageId !== "asset-changes"}>
        <WorkspaceLazy>
          <AssetChangesPage onOpenHistory={(filters, activityId) => open({ page: "history", filters, activityId })} onOpenReturnAnalysis={(analysis) => open({ page: "return-analysis", tab: "contribution", analysis })} />
        </WorkspaceLazy>
        </div>
      )}
    </AppShell>
    </NavigationContext.Provider>
  );
}

/** A ledger identity change releases all in-memory destinations, drafts and frozen captures. Restore requires process restart. */
function App() {
  const startup = useStartup();
  const settings = useSettings({ enabled: startup.data?.available === true });
  const bootstrap = useBootstrap({ enabled: startup.data?.available === true && Boolean(settings.data) });
  const ledgerIdentity = bootstrap.data?.household?.id ?? "startup";
  const [session, setSession] = useState({ identity: ledgerIdentity, key: ledgerIdentity });
  if (session.identity !== ledgerIdentity && !hasUnresolvedSheetConfirmation()) {
    // Initial hydration/onboarding supplies the first identity; it is not a ledger switch.
    setSession({ identity: ledgerIdentity, key: session.identity === "startup" ? session.key : ledgerIdentity });
  }
  return <Workspace key={session.key} />;
}

export default App;
