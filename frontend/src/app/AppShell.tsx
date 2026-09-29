import * as React from "react";
import { useTranslation } from "react-i18next";
import { PanelLeftClose, PanelLeftOpen, Moon, Sun, Monitor } from "lucide-react";
import { NAV_GROUPS, DEFAULT_PAGE_ID, targetForPage, type NavigationTarget, type PageId } from "@/app/navigation";
import { TONE_CLASSES, type Tone } from "@/lib/tone";
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

/** Each destination keeps its own hue so the sidebar reads as colorful landmarks, not a gray list. */
const PAGE_TONE: Record<PageId, Tone> = {
  overview: 8,
  accounts: 1,
  "available-funds": 2,
  portfolio: 5,
  history: 6,
  "return-analysis": 7,
  "asset-changes": 3,
  directory: 4,
  "market-data": 1,
  "data-health": 3,
  settings: 5,
};

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
      className="h-9 rounded-full border-transparent bg-muted px-3 text-xs font-medium shadow-none hover:bg-primary/10"
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
  const setAppearance = useUiStore((state) => state.setAppearance);
  const [pageBarTarget, setPageBarTarget] = React.useState<HTMLDivElement | null>(null);

  const persistHeaderSetting = (patch: Partial<Settings>) => {
    const next = { ...settings, ...patch };
    if (patch.appearance) {
      setAppearance(patch.appearance as Appearance);
    }
    if (patch.language) {
      setLanguage(patch.language);
    }
    saveSettings.mutate(next, {
      onError: (error) => {
        setAppearance(settings.appearance as Appearance);
        setLanguage(settings.language);
        toast.error(displayError(error, t("settings.saveError")));
      },
    });
  };

  return (
    <PageChromeProvider activePageId={activePageId} target={pageBarTarget}>
      <div className="fixed inset-0 flex min-h-0 min-w-0 overflow-hidden bg-background text-foreground">
      <aside
        className={cn(
          "flex shrink-0 flex-col border-r border-border bg-card transition-[width] duration-200 ease-out",
          collapsed ? "w-16" : "w-60",
        )}
      >
        <div className={cn("flex px-3", collapsed ? "flex-col items-center gap-2 py-3" : "h-14 items-center justify-between")}>
          <BrandLockup compact={collapsed} />
          <Button variant="ghost" size="icon" onClick={toggleSidebar} aria-label={t("ui.navigation.toggleSidebar")}>
            {collapsed ? <PanelLeftOpen className="size-4" /> : <PanelLeftClose className="size-4" />}
          </Button>
        </div>
        <nav className="flex flex-1 flex-col gap-4 overflow-y-auto px-3 pb-3" aria-label={t("ui.navigation.main")}>
          {NAV_GROUPS.map((group) => (
            <div key={group.id} className="flex flex-col gap-1">
              {!collapsed && group.translationKey && (
                <p className="px-3 pb-1 text-xs font-semibold text-muted-foreground">
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
                    onClick={() => { setNarrowExpanded(false); onNavigate(targetForPage(item.id)); }}
                    aria-label={label}
                    title={collapsed ? label : undefined}
                    aria-current={active ? "page" : undefined}
                    className={cn(
                      "group flex items-center gap-3 rounded-xl px-2 py-1.5 text-sm font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50",
                      active ? "bg-primary/12 text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground",
                      collapsed && "justify-center px-1",
                    )}
                  >
                    <span
                      aria-hidden="true"
                      className={cn(
                        "flex size-8 shrink-0 items-center justify-center rounded-lg transition-colors",
                        active ? cn(TONE_CLASSES[PAGE_TONE[item.id]].solid, "text-background") : cn(TONE_CLASSES[PAGE_TONE[item.id]].soft, TONE_CLASSES[PAGE_TONE[item.id]].text),
                      )}
                    >
                      <Icon className="size-4" />
                    </span>
                    {!collapsed && <span className="truncate">{label}</span>}
                  </button>
                );
              })}
            </div>
          ))}
        </nav>
      </aside>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
        <header className="flex min-h-14 shrink-0 flex-wrap items-center justify-between gap-2 border-b border-border bg-card/80 px-4 py-2 backdrop-blur-md sm:flex-nowrap sm:gap-3">
          <div ref={setPageBarTarget} className="order-1 flex min-w-0 basis-full flex-wrap items-center gap-x-3 gap-y-2 sm:flex-1 sm:basis-auto" />
          <div className="order-2 ml-auto flex shrink-0 items-center gap-2">
            <LanguageSwitcher
              language={settings.language}
              disabled={saveSettings.isPending}
              onChange={(language) => persistHeaderSetting({ language: language as Settings["language"] })}
            />
            <AppearanceToggle
              appearance={settings.appearance as Appearance}
              disabled={saveSettings.isPending}
              onChange={(appearance) => persistHeaderSetting({ appearance: appearance as Settings["appearance"] })}
            />
          </div>
        </header>
        <main id="main-content" className="min-h-0 min-w-0 flex-1 overscroll-y-contain overflow-x-hidden overflow-y-auto bg-[radial-gradient(48rem_20rem_at_100%_0%,var(--color-surface-tint),transparent)] p-6 sm:p-8">
          <div className="page-stage mx-auto min-w-0 w-full max-w-7xl">{children}</div>
        </main>
      </div>
      </div>
    </PageChromeProvider>
  );
}

export { DEFAULT_PAGE_ID };
