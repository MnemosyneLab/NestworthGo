import { useValuationDayBoundary } from "@/queries/useValuationDayBoundary";
import * as React from "react";
import { useTranslation } from "react-i18next";
import { PanelLeftClose, PanelLeftOpen, Moon, Sun, Monitor, RefreshCw, Languages } from "lucide-react";
import { NAV_GROUPS, DEFAULT_PAGE_ID, targetForPage, type NavigationTarget, type PageId } from "@/app/navigation";
import { formatTasksBadge, useTasksCounts } from "@/features/tasks/useTasksCounts";
import { useUiStore, type Appearance } from "@/stores/ui";
import { useTheme } from "@/hooks/useTheme";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { SUPPORTED_LANGUAGES, languageOptionKey, setLanguage } from "@/i18n";
import { useSaveSettings } from "@/queries/settings";
import { BrandLockup } from "@/components/brand/BrandLockup";
import { displayError } from "@/lib/display";
import { toast } from "sonner";
import type { SettingsDTO as Settings } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings/models";
import { PageChromeProvider } from "@/components/layout/PageChrome";
import { AppFocusBoundary } from "./AppFocusBoundary";

const APPEARANCE_ICONS: Record<Appearance, React.ComponentType<{ className?: string }>> = {
  system: Monitor,
  light: Sun,
  dark: Moon,
};

function AppearanceToggle({ appearance, disabled, onChange }: { appearance: Appearance; disabled: boolean; onChange: (appearance: Appearance) => void }) {
  const { t } = useTranslation();
  const order: Appearance[] = ["system", "light", "dark"];

  const cycle = () => {
    const next = order[(order.indexOf(appearance) + 1) % order.length];
    onChange(next);
  };

  const Icon = APPEARANCE_ICONS[appearance];
  return (
    <Button
      variant="ghost"
      size="icon"
      onClick={cycle}
      disabled={disabled}
      aria-label={t("ui.appearance.toggle")}
      title={t(`option.appearance.${appearance}`)}
    >
      <Icon className="size-4" />
    </Button>
  );
}

function LanguageSwitcher({ language, disabled, onChange }: { language: string; disabled: boolean; onChange: (language: string) => void }) {
  const { t, i18n: i18nInstance } = useTranslation();
  return (
    <NativeSelect
      aria-label={t("settings.language.language")}
      className="h-8 min-w-0 flex-1 rounded-lg border-transparent bg-transparent pl-2 pr-7 text-xs font-medium text-muted-foreground shadow-none hover:bg-muted hover:text-foreground"
      value={language === "system" ? i18nInstance.language : language}
      onChange={(event) => onChange(event.target.value)}
      disabled={disabled}
    >
      {SUPPORTED_LANGUAGES.map((language) => (
        <option key={language} value={language}>
          {t(`option.language.${languageOptionKey(language)}`)}
        </option>
      ))}
    </NativeSelect>
  );
}

export interface AppShellProps {
  activePageId: PageId;
  onNavigate: (target: NavigationTarget) => void;
  settings: Settings;
  children: React.ReactNode;
}

/**
 * AppShell is the persistent sidebar/header layout every page renders
 * inside. Navigation uses simple page-state rather than a router;
 * DEFAULT_PAGE_ID ("overview") is the product's default landing page.
 */
export function AppShell({ activePageId, onNavigate, settings, children }: AppShellProps) {
 useValuationDayBoundary();
  const { t } = useTranslation();
  useTheme();
  const storedCollapsed = useUiStore((state) => state.sidebarCollapsed);
  const toggleStoredSidebar = useUiStore((state) => state.toggleSidebar);
  // Narrow windows start with the icon rail; expanding there is temporary and does not change the saved preference.
  const narrow = useMediaQuery("(max-width: 959px)");
  const [narrowExpanded, setNarrowExpanded] = React.useState(false);
  const collapsed = narrow ? !narrowExpanded : storedCollapsed;
  const toggleSidebar = narrow ? () => setNarrowExpanded((value) => !value) : toggleStoredSidebar;
  const saveSettings = useSaveSettings();
  const tasksCounts = useTasksCounts();
  const tasksStale = (tasksCounts.upcoming.isError && Boolean(tasksCounts.upcoming.data)) || (tasksCounts.review.isError && Boolean(tasksCounts.review.data));
  const retryStaleTasks = () => {
    if (tasksCounts.upcoming.isError) void tasksCounts.upcoming.refetch();
    if (tasksCounts.review.isError) void tasksCounts.review.refetch();
  };
  const setAppearance = useUiStore((state) => state.setAppearance);
  const [pageBarTarget, setPageBarTarget] = React.useState<HTMLDivElement | null>(null);

  const persistHeaderSetting = (patch: Partial<Settings>) => {
    saveSettings.mutate(patch, {
      onSuccess: (saved) => {
        setAppearance(saved.appearance as Appearance);
        setLanguage(saved.language);
      },
      onError: (error) => {
        toast.error(displayError(error, t("settings.saveError")));
      },
    });
  };

  return (
    <PageChromeProvider activePageId={activePageId} target={pageBarTarget}>
      <AppFocusBoundary className="fixed inset-0 flex min-h-0 min-w-0 overflow-hidden bg-background text-foreground">
      <aside
        className={cn(
          "flex shrink-0 flex-col border-r border-border bg-sidebar transition-[width] duration-200 ease-out",
          collapsed ? "w-16" : "w-56",
        )}
      >
        <div className={cn("flex px-3", collapsed ? "flex-col items-center gap-2 py-3" : "h-14 items-center justify-between")}>
          <BrandLockup compact={collapsed} />
          <Button variant="ghost" size="icon" onClick={toggleSidebar} aria-label={t("ui.navigation.toggleSidebar")}>
            {collapsed ? <PanelLeftOpen className="size-4" /> : <PanelLeftClose className="size-4" />}
          </Button>
        </div>
        <nav className="flex flex-1 flex-col gap-5 overflow-y-auto px-3 pb-3 pt-1" aria-label={t("ui.navigation.main")}>
          {NAV_GROUPS.map((group) => (
            <div key={group.id} className={cn("flex flex-col gap-0.5", group.id === "manage" && "mt-auto border-t border-border pt-4")}>
              {!collapsed && group.translationKey && (
                <p className="px-2.5 pb-1.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-muted-foreground/80">
                  {t(group.translationKey)}
                </p>
              )}
              {group.items.map((item) => {
                const Icon = item.icon;
                const active = item.id === activePageId ||
                  (item.id === "market-data" && activePageId === "data-health") ||
                  (item.id === "portfolio" && activePageId === "instrument-detail");
                const label = t(item.translationKey);
                const taskCount = item.id === "tasks" ? tasksCounts.sidebarCount : 0;
                const ariaLabel = taskCount > 0 ? t("tasks.sidebarLabel", { page: label, count: taskCount }) : label;
                return (
                  <div key={item.id} className="flex flex-col gap-1">
                  <button
                    type="button"
                    onClick={() => { setNarrowExpanded(false); onNavigate(targetForPage(item.id)); }}
                    aria-label={ariaLabel}
                    title={collapsed ? label : undefined}
                    aria-current={active ? "page" : undefined}
                    data-nav-id={item.id}
                    className={cn(
                      "group relative flex h-9 items-center gap-3 rounded-lg px-2.5 text-[13px] font-medium transition-colors duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50",
                      active
                        ? "bg-card text-foreground shadow-xs ring-1 ring-border before:absolute before:inset-y-2 before:left-0 before:w-0.5 before:rounded-full before:bg-primary"
                        : "text-muted-foreground hover:bg-card/70 hover:text-foreground",
                      collapsed && "justify-center px-0",
                    )}
                  >
                    <span aria-hidden="true" className="relative flex size-5 shrink-0 items-center justify-center">
                      <Icon className={cn("size-[18px] transition-colors", active ? "text-primary" : "text-muted-foreground group-hover:text-foreground")} strokeWidth={1.75} />
                      {item.id === "tasks" && collapsed && taskCount > 0 && (
                        <span data-testid="tasks-sidebar-dot" className="absolute -right-1 -top-1 size-2 rounded-full bg-warning ring-2 ring-sidebar" />
                      )}
                    </span>
                    {!collapsed && <span className="truncate">{label}</span>}
                    {item.id === "tasks" && !collapsed && taskCount > 0 && (
                      <span data-testid="tasks-sidebar-count" aria-hidden="true" className="num ml-auto rounded-full bg-warning/18 px-1.5 text-[11px] font-semibold text-warning-foreground">{formatTasksBadge(taskCount)}</span>
                    )}
                  </button>
                  {item.id === "tasks" && tasksStale && (
                    <div className={cn("flex min-w-0", collapsed ? "justify-center" : "flex-col items-start gap-1 px-2")}>
                      <p role="status" className={cn("text-xs leading-4 text-warning-foreground", collapsed && "sr-only")}>{t("tasks.summaryStale")}</p>
                      <Button type="button" variant="outline" size="sm" className={cn(collapsed && "size-8 px-0")} aria-label={t("tasks.summaryStaleRetry", { section: label })} onClick={retryStaleTasks}>
                        {collapsed ? <RefreshCw aria-hidden="true" className="size-3.5" /> : t("common.retryAction")}
                      </Button>
                    </div>
                  )}
                  </div>
                );
              })}
            </div>
          ))}
        </nav>
        <div className={cn("flex shrink-0 items-center gap-1 border-t border-border px-3 py-2", collapsed && "justify-center px-0")}>
          {!collapsed && (
            <div className="flex min-w-0 flex-1 items-center gap-1.5">
              <Languages aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
              <LanguageSwitcher
                language={settings.language}
                disabled={saveSettings.isPending}
                onChange={(language) => persistHeaderSetting({ language: language as Settings["language"] })}
              />
            </div>
          )}
          <AppearanceToggle
            appearance={settings.appearance as Appearance}
            disabled={saveSettings.isPending}
            onChange={(appearance) => persistHeaderSetting({ appearance: appearance as Settings["appearance"] })}
          />
        </div>
      </aside>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
        <header className="flex min-h-14 shrink-0 items-center gap-3 border-b border-border bg-background/85 px-6 py-2 backdrop-blur-md sm:px-8">
          <div ref={setPageBarTarget} className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-2" />
        </header>
        <main id="main-content" tabIndex={-1} className="min-h-0 min-w-0 flex-1 overscroll-y-contain overflow-x-hidden overflow-y-auto p-6 sm:p-8">
          <div className="page-stage mx-auto min-w-0 w-full max-w-7xl">{children}</div>
        </main>
      </div>
      </AppFocusBoundary>
    </PageChromeProvider>
  );
}

export { DEFAULT_PAGE_ID };
