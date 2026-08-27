import * as React from "react";
import { useTranslation } from "react-i18next";
import { PanelLeftClose, PanelLeftOpen, Moon, Sun, Monitor } from "lucide-react";
import { NAV_GROUPS, DEFAULT_PAGE_ID } from "@/app/navigation";
import { useUiStore, type Appearance } from "@/stores/ui";
import { useTheme } from "@/hooks/useTheme";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { SUPPORTED_LANGUAGES, languageOptionKey, setLanguage } from "@/i18n";
import { useAppInfo } from "@/queries/app";
import { useSaveSettings } from "@/queries/settings";
import { BrandLockup } from "@/components/brand/BrandLockup";
import { displayError } from "@/lib/display";
import { toast } from "sonner";
import type { Settings } from "../../bindings/github.com/waltwang/nestworth-go/internal/settings/models";

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
      className="h-8 px-2 text-xs"
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
  activePageId: string;
  onNavigate: (pageId: string) => void;
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
  const collapsed = useUiStore((state) => state.sidebarCollapsed);
  const toggleSidebar = useUiStore((state) => state.toggleSidebar);
  // Exercises the queries/ -> lib/wails.callService -> generated-binding
  // pipeline end to end inside the real app shell.
  const appInfo = useAppInfo();
  const saveSettings = useSaveSettings();
  const setAppearance = useUiStore((state) => state.setAppearance);

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
    <div className="flex h-[100dvh] w-screen overflow-hidden bg-background text-foreground">
      <aside
        className={cn(
          "flex flex-col border-r border-border bg-card/80 backdrop-blur-sm transition-[width] duration-150",
          collapsed ? "w-14" : "w-56",
        )}
      >
        <div className={cn("flex px-3", collapsed ? "flex-col items-center gap-2 py-3" : "h-14 items-center justify-between")}>
          <BrandLockup compact={collapsed} />
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
                      "flex items-center gap-2 rounded-lg px-2 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50",
                      active
                        ? "bg-gradient-to-r from-primary to-primary/85 text-primary-foreground shadow-sm"
                        : "text-foreground hover:bg-muted",
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
                : "…"}
          </div>
        )}
      </aside>
      <div className="flex flex-1 flex-col overflow-hidden">
        <header className="flex h-14 items-center justify-end gap-2 border-b border-border bg-card/60 px-4 backdrop-blur-sm">
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
        </header>
        <main id="main-content" className="flex-1 overflow-auto p-6 sm:p-8">
          <div className="mx-auto w-full max-w-7xl">{children}</div>
        </main>
      </div>
    </div>
  );
}

export { DEFAULT_PAGE_ID };
