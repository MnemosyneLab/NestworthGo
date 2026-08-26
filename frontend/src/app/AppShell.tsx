import * as React from "react";
import { useTranslation } from "react-i18next";
import { PanelLeftClose, PanelLeftOpen, Moon, Sun, Monitor } from "lucide-react";
import { NAV_GROUPS, DEFAULT_PAGE_ID } from "@/app/navigation";
import { useUiStore, type Appearance } from "@/stores/ui";
import { useTheme } from "@/hooks/useTheme";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { SUPPORTED_LANGUAGES, setLanguage } from "@/i18n";
import { useAppInfo } from "@/queries/app";

const APPEARANCE_ICONS: Record<Appearance, React.ComponentType<{ className?: string }>> = {
  system: Monitor,
  light: Sun,
  dark: Moon,
};

function AppearanceToggle() {
  const { t } = useTranslation();
  const appearance = useUiStore((state) => state.appearance);
  const setAppearance = useUiStore((state) => state.setAppearance);
  const order: Appearance[] = ["system", "light", "dark"];

  const cycle = () => {
    const next = order[(order.indexOf(appearance) + 1) % order.length];
    setAppearance(next);
  };

  const Icon = APPEARANCE_ICONS[appearance];
  return (
    <Button
      variant="ghost"
      size="icon"
      onClick={cycle}
      aria-label={t("ui.appearance.toggle")}
      title={t(`option.appearance.${appearance}`)}
    >
      <Icon className="size-4" />
    </Button>
  );
}

function LanguageSwitcher() {
  const { t, i18n: i18nInstance } = useTranslation();
  return (
    <select
      aria-label={t("settings.language.language")}
      className="h-8 rounded-md border border-border bg-card px-2 text-xs text-foreground"
      value={i18nInstance.language}
      onChange={(event) => setLanguage(event.target.value)}
    >
      {SUPPORTED_LANGUAGES.map((language) => (
        <option key={language} value={language}>
          {t(`option.language.${language === "zh-CN" ? "zhCN" : language === "zh-TW" ? "zhTW" : "en"}`)}
        </option>
      ))}
    </select>
  );
}

export interface AppShellProps {
  activePageId: string;
  onNavigate: (pageId: string) => void;
  children: React.ReactNode;
}

/**
 * AppShell is the persistent sidebar/header layout every page renders
 * inside. Navigation uses simple page-state rather than a router;
 * DEFAULT_PAGE_ID ("overview") is the product's default landing page.
 */
export function AppShell({ activePageId, onNavigate, children }: AppShellProps) {
  const { t } = useTranslation();
  useTheme();
  const collapsed = useUiStore((state) => state.sidebarCollapsed);
  const toggleSidebar = useUiStore((state) => state.toggleSidebar);
  // Exercises the queries/ -> lib/wails.callService -> generated-binding
  // pipeline end to end inside the real app shell.
  const appInfo = useAppInfo();

  return (
    <div className="flex h-[100dvh] w-screen overflow-hidden bg-background text-foreground">
      <aside
        className={cn(
          "flex flex-col border-r border-border bg-card transition-all duration-150",
          collapsed ? "w-14" : "w-56",
        )}
      >
        <div className="flex h-12 items-center justify-between px-3">
          {!collapsed && <span className="text-sm font-semibold">{t("app.name")}</span>}
          <Button variant="ghost" size="icon" onClick={toggleSidebar} aria-label={t("ui.navigation.toggleSidebar")}>
            {collapsed ? <PanelLeftOpen className="size-4" /> : <PanelLeftClose className="size-4" />}
          </Button>
        </div>
        <nav className="flex flex-1 flex-col gap-4 overflow-y-auto px-2" aria-label={t("ui.navigation.main")}>
          {NAV_GROUPS.map((group) => (
            <div key={group.id} className="flex flex-col gap-1">
              {!collapsed && group.translationKey && (
                <p className="px-2 pb-1 text-[0.68rem] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                  {t(group.translationKey)}
                </p>
              )}
              {group.items.map((item) => {
                const Icon = item.icon;
                const active = item.id === activePageId;
                const label = t(item.translationKey);
                return (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => onNavigate(item.id)}
                    aria-label={label}
                    title={collapsed ? label : undefined}
                    aria-current={active ? "page" : undefined}
                    className={cn(
                      "flex items-center gap-2 rounded-md px-2 py-2 text-sm font-medium transition-colors",
                      active ? "bg-primary text-primary-foreground" : "text-foreground hover:bg-muted",
                      collapsed && "justify-center",
                    )}
                  >
                    <Icon className="size-4 shrink-0" aria-hidden="true" />
                    {!collapsed && <span>{label}</span>}
                  </button>
                );
              })}
            </div>
          ))}
        </nav>
        {!collapsed && (
          <div className="px-3 py-2 text-xs text-muted-foreground">
            {appInfo.data
              ? appInfo.data.version.startsWith("v")
                ? appInfo.data.version
                : `v${appInfo.data.version}`
              : appInfo.isError
                ? ""
                : "..."}
          </div>
        )}
      </aside>
      <div className="flex flex-1 flex-col overflow-hidden">
        <header className="flex h-12 items-center justify-end gap-2 border-b border-border px-4">
          <LanguageSwitcher />
          <AppearanceToggle />
        </header>
        <main id="main-content" className="flex-1 overflow-auto p-6 sm:p-8">
          <div className="mx-auto w-full max-w-7xl">{children}</div>
        </main>
      </div>
    </div>
  );
}

export { DEFAULT_PAGE_ID };
