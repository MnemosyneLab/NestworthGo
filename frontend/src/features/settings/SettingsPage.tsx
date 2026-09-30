import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { SecretField } from "./SecretField";
import { ContinuousBackupSection } from "./ContinuousBackupSection";
import { useBootstrap } from "@/queries/household";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { FilterableSelect } from "@/components/ui/filterable-select";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { PageIntro } from "@/components/layout/PageHeader";
import { PageChrome } from "@/components/layout/PageChrome";
import { ErrorState, LoadingState } from "@/components/layout/PageState";
import { useSettings, useSaveSettings, useResetSettings, useFXProviders, useCoinGeckoKeyStatus, useSaveCoinGeckoAPIKey, useDeleteCoinGeckoAPIKey, useTiingoKeyStatus, useSaveTiingoAPIKey, useDeleteTiingoAPIKey, quoteCacheTtlOf, withQuoteCacheTtl, type QuoteCacheTTL } from "@/queries/settings";
import { useHistoryOrigin } from "@/queries/history";
import { useCatalog } from "@/queries/catalog";
import { AboutPage } from "@/features/about/AboutPage";
import { AgentSection } from "@/features/settings/AgentSection";
import { DataManagementSection } from "@/features/settings/DataManagementSection";
import { useUiStore, type Appearance, type Accent } from "@/stores/ui";
import { setLanguage, languageOptionKey } from "@/i18n";
import { displayError } from "@/lib/display";
import type { SettingsDTO as Settings } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings/models";
import { resolvedTimeZone, timeZoneOptions } from "@/lib/time";

/**
 * SettingsPage keeps editing local until Save changes is pressed. Discard
 * only returns to the last loaded/saved snapshot; Restore defaults is the
 * separate destructive operation and always asks for confirmation.
 */
const SECTION_IDS = ["general", "market", "agent", "data", "diagnostics", "about", "reset"] as const;
const ACCENT_ORDER = ["indigo", "lavender", "mint", "sky", "peach"] as const;
// Fixed preview colors so every accent stays visible regardless of the active theme.
const ACCENT_SWATCH: Record<(typeof ACCENT_ORDER)[number], string> = {
  indigo: "bg-[oklch(55%_0.19_268)]",
  lavender: "bg-[oklch(64%_0.15_300)]",
  mint: "bg-[oklch(60%_0.12_180)]",
  sky: "bg-[oklch(62%_0.16_250)]",
  peach: "bg-[oklch(72%_0.15_45)]",
};

export function SettingsPage() {
  const { t } = useTranslation();
  const settings = useSettings();
  const saveSettings = useSaveSettings();
  const resetSettings = useResetSettings();
  const bootstrap = useBootstrap();
  const fxProviders = useFXProviders();
  const coinGeckoKeyStatus = useCoinGeckoKeyStatus();
  const saveCoinGeckoKey = useSaveCoinGeckoAPIKey();
  const deleteCoinGeckoKey = useDeleteCoinGeckoAPIKey();
  const tiingoKeyStatus = useTiingoKeyStatus();
  const saveTiingoKey = useSaveTiingoAPIKey();
  const deleteTiingoKey = useDeleteTiingoAPIKey();
  const catalog = useCatalog();
  const origin = useHistoryOrigin();
  const setAppearance = useUiStore((state) => state.setAppearance);
  const setAccent = useUiStore((state) => state.setAccent);
  const [activeSection, setActiveSection] = useState("general");
  const ready = Boolean(settings.data);
  useEffect(() => {
    if (!ready || typeof IntersectionObserver === "undefined") {
      return;
    }
    const targets = SECTION_IDS.map((id) => document.getElementById(`settings-${id}`)).filter((node): node is HTMLElement => node !== null);
    const observer = new IntersectionObserver((entries) => {
      const visible = entries.filter((entry) => entry.isIntersecting).sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)[0];
      if (visible) {
        setActiveSection(visible.target.id.replace("settings-", ""));
      }
    }, { rootMargin: "-10% 0px -70% 0px" });
    targets.forEach((node) => observer.observe(node));
    return () => observer.disconnect();
  }, [ready]);
  const [draftOverride, setDraftOverride] = useState<Settings | null>(null);
  const [coinGeckoKey, setCoinGeckoKey] = useState("");
  const [tiingoKey, setTiingoKey] = useState("");
  const pageChrome = <PageChrome pageId="settings" title={t("settings.title")} />;

  if (settings.isLoading) {
    return <>{pageChrome}<LoadingState label={t("ui.state.loadingPage")} /></>;
  }

  if (settings.isError || !settings.data) {
    return (
      <>
        {pageChrome}
        <ErrorState
          title={t("settings.loadError")}
          description={t("ui.state.errorDescription")}
          onRetry={() => settings.refetch()}
          retryLabel={t("common.retryAction")}
        />
      </>
    );
  }

  const draft = draftOverride ?? settings.data;
  if (!draft) {
    return <>{pageChrome}<LoadingState label={t("ui.state.loadingPage")} /></>;
  }

  const isDirty =
    (draft.logLevel || "off") !== (settings.data.logLevel || "off") ||
    draft.appearance !== settings.data.appearance ||
    draft.accent !== settings.data.accent ||
    draft.language !== settings.data.language ||
    draft.timezone !== settings.data.timezone ||
    draft.fxProvider !== settings.data.fxProvider ||
    quoteCacheTtlOf(draft) !== quoteCacheTtlOf(settings.data);
  const update = (patch: Partial<Settings>) => {
    setDraftOverride((current) => ({ ...(current ?? settings.data), ...patch }));
  };

  const applyLivePreferences = (value: Settings) => {
    setAppearance(value.appearance as Appearance);
    setAccent(value.accent as Accent);
    setLanguage(value.language);
  };

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    saveSettings.mutate(draft, {
      onSuccess: () => {
        setDraftOverride(null);
        toast.success(t("settings.changesSaved"));
        applyLivePreferences(draft);
      },
    });
  };

  const submitCoinGeckoKey = (event: React.FormEvent) => {
    event.preventDefault();
    saveCoinGeckoKey.mutate(coinGeckoKey, {
      onSuccess: () => {
        setCoinGeckoKey("");
        toast.success(t("settings.coinGeckoKeySaved"));
      },
    });
  };

  const submitTiingoKey = (event: React.FormEvent) => {
    event.preventDefault();
    saveTiingoKey.mutate(tiingoKey, {
      onSuccess: () => {
        setTiingoKey("");
        toast.success(t("settings.tiingoKeySaved"));
      },
    });
  };

  return (
    <div className="flex flex-col gap-6">
      {pageChrome}
      <PageIntro description={t("settings.subtitle")} />
      <div className="grid items-start gap-8 xl:grid-cols-[10rem_minmax(0,1fr)]">
        <nav aria-label={t("settings.sections.navigation")} className="flex flex-wrap gap-1 xl:sticky xl:top-6 xl:flex-col">
          {SECTION_IDS.map((id) => (
            <a key={id} href={`#settings-${id}`} onClick={() => setActiveSection(id)} aria-current={activeSection === id ? "location" : undefined} className={`rounded-xl px-3 py-2 text-sm transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50 ${activeSection === id ? "bg-primary/12 font-semibold text-primary" : "text-muted-foreground"}`}>{t(`settings.sections.${id}`)}</a>
          ))}
        </nav>
        <div className="min-w-0 max-w-3xl space-y-8 pb-4">
      <section id="settings-general" aria-labelledby="settings-general-title" className="scroll-mt-6">
      <form id="settings-preferences" onSubmit={submit} className="space-y-5" aria-label={t("settings.formLabel")}>
        <h2 id="settings-general-title" className="text-base font-semibold">{t("settings.sections.general")}</h2>
        <div>
        <div className="grid gap-5 sm:grid-cols-2">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="settings-appearance">{t("settings.appearance.mode")}</Label>
            <NativeSelect
              id="settings-appearance"
              value={draft.appearance}
              onChange={(event) => update({ appearance: event.target.value as Settings["appearance"] })}
            >
              {(catalog.data?.appearances ?? []).map((appearance) => (
                <option key={appearance} value={appearance}>
                  {t(`option.appearance.${appearance}`)}
                </option>
              ))}
            </NativeSelect>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="settings-accent">{t("settings.appearance.accent")}</Label>
            <div className="flex items-center gap-3">
              <NativeSelect
                id="settings-accent"
                className="min-w-0 flex-1"
                value={draft.accent}
                onChange={(event) => update({ accent: event.target.value as Settings["accent"] })}
              >
                {(catalog.data?.accents ?? []).map((accent) => (
                  <option key={accent} value={accent}>
                    {t(`option.accent.${accent}`)}
                  </option>
                ))}
              </NativeSelect>
              {/* Pointer shortcut only; the select above is the accessible control. */}
              <div className="flex shrink-0 items-center gap-2" aria-hidden="true">
                {ACCENT_ORDER.map((accent) => (
                  <button
                    key={accent}
                    type="button"
                    tabIndex={-1}
                    data-testid={`accent-swatch-${accent}`}
                    title={t(`option.accent.${accent}`)}
                    onClick={() => update({ accent: accent as Settings["accent"] })}
                    className={`size-7 shrink-0 cursor-pointer rounded-full ring-offset-2 ring-offset-background transition-transform hover:scale-110 ${ACCENT_SWATCH[accent]} ${draft.accent === accent ? "ring-2 ring-primary" : ""}`}
                  />
                ))}
              </div>
            </div>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="settings-language">{t("settings.language.language")}</Label>
            <NativeSelect
              id="settings-language"
              value={draft.language}
              onChange={(event) => update({ language: event.target.value as Settings["language"] })}
            >
              {(catalog.data?.languages ?? []).map((language) => (
                <option key={language} value={language}>
                  {t(`option.language.${languageOptionKey(language)}`)}
                </option>
              ))}
            </NativeSelect>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="settings-timezone">{t("settings.language.timezone")}</Label>
            <FilterableSelect
              id="settings-timezone"
              value={draft.timezone}
              options={[
                { value: "system", label: t("option.timezone.system") },
                ...timeZoneOptions().map((timezone) => ({ value: timezone, label: timezone })),
              ]}
              noOptionsLabel={t("common.noMatches")}
              onValueChange={(timezone) => update({ timezone })}
            />
            <p className="text-xs text-muted-foreground">
              {t("settings.timezoneResolved", { timezone: resolvedTimeZone(draft.timezone) })}
            </p>
            {origin.data && resolvedTimeZone(draft.timezone) !== origin.data.timezone && (
              <p className="text-xs text-muted-foreground">
                {t("history.settingsTimezoneDiff", { origin: origin.data.timezone, presentation: resolvedTimeZone(draft.timezone) })}
              </p>
            )}
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="settings-currency">{t("review.householdCurrency")}</Label>
            <Input id="settings-currency" readOnly value={bootstrap.data?.household?.baseCurrency ?? "—"} />
            <p className="text-xs text-muted-foreground">{t("review.householdCurrencyHelp")}</p>
          </div>

        </div>
        </div>
      </form>
      </section>
      <section id="settings-market" aria-labelledby="settings-market-title" className="scroll-mt-6 space-y-5 border-t border-border pt-6">
        <h2 id="settings-market-title" className="text-base font-semibold">{t("settings.sections.market")}</h2>
        <div className="grid gap-5 sm:grid-cols-2">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="settings-fx-provider">{t("settings.providers.fxProvider")}</Label>
            <NativeSelect
              id="settings-fx-provider" form="settings-preferences"
              value={draft.fxProvider}
              onChange={(event) => update({ fxProvider: event.target.value })}
            >
              {(fxProviders.data ?? [draft.fxProvider]).map((provider) => (
                <option key={provider} value={provider}>
                  {t(`settings.provider.${provider}`, { defaultValue: provider })}
                </option>
              ))}
            </NativeSelect>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label htmlFor="settings-quote-cache-ttl">{t("settings.quoteCacheTtl")}</Label>
            <NativeSelect
              id="settings-quote-cache-ttl" form="settings-preferences"
              value={quoteCacheTtlOf(draft)}
              onChange={(event) => setDraftOverride(withQuoteCacheTtl(draft, event.target.value as QuoteCacheTTL))}
            >
              {(["1h", "3h", "12h", "24h"] as QuoteCacheTTL[]).map((ttl) => (
                <option key={ttl} value={ttl}>
                  {t(`settings.quoteCacheTtlOption.${ttl}`)}
                </option>
              ))}
            </NativeSelect>
            <p className="text-xs text-muted-foreground">{t("settings.quoteCacheTtlHelp")}</p>
          </div>
        </div>

        <p className="text-sm text-muted-foreground">{t("settings.sections.credentialsHelp")}</p>
        <div className="divide-y divide-border overflow-hidden rounded-2xl border border-border bg-card shadow-xs">
          <div className="p-5"><h3 className="font-medium">{t("settings.sections.coinGecko")}</h3>
        <form onSubmit={submitCoinGeckoKey} className="mt-4 flex max-w-xl flex-col gap-2">
          <SecretField key={saveCoinGeckoKey.isSuccess ? saveCoinGeckoKey.submittedAt : "coinGecko"} id="settings-coinGecko-key" label={t("settings.coinGeckoKey")} hasValue={Boolean(coinGeckoKeyStatus.data?.configured)} value={coinGeckoKey} onChange={setCoinGeckoKey} disabled={saveCoinGeckoKey.isPending || deleteCoinGeckoKey.isPending} onRemove={() => deleteCoinGeckoKey.mutate()} removeLabel={t("settings.removeCoinGeckoKey")} />
          <Button type="submit" disabled={!coinGeckoKey.trim() || saveCoinGeckoKey.isPending}>{saveCoinGeckoKey.isPending ? t("common.pending") : t("settings.saveCoinGeckoKey")}</Button>
          {coinGeckoKeyStatus.data?.configured ? (
            <p className="text-sm text-success-foreground">{t("settings.coinGeckoKeyConfigured")}</p>
          ) : (
            <p className="text-sm text-muted-foreground">{t("settings.coinGeckoKeyMissing")}</p>
          )}

          {(saveCoinGeckoKey.isError || deleteCoinGeckoKey.isError || coinGeckoKeyStatus.isError) && (
            <p role="alert" className="text-sm text-destructive">
              {t("settings.coinGeckoKeyError")}
            </p>
          )}
        </form>
          </div>
          <div className="p-5"><h3 className="font-medium">{t("settings.sections.tiingo")}</h3>
        <form onSubmit={submitTiingoKey} className="mt-4 flex max-w-xl flex-col gap-2">
          <SecretField key={saveTiingoKey.isSuccess ? saveTiingoKey.submittedAt : "tiingo"} id="settings-tiingo-key" label={t("settings.tiingoKey")} hasValue={Boolean(tiingoKeyStatus.data?.configured)} value={tiingoKey} onChange={setTiingoKey} disabled={saveTiingoKey.isPending || deleteTiingoKey.isPending} onRemove={() => deleteTiingoKey.mutate()} removeLabel={t("settings.removeTiingoKey")} />
          <Button type="submit" disabled={!tiingoKey.trim() || saveTiingoKey.isPending}>{saveTiingoKey.isPending ? t("common.pending") : t("settings.saveTiingoKey")}</Button>
          {tiingoKeyStatus.data?.configured ? (
            <p className="text-sm text-success-foreground">{t("settings.tiingoKeyConfigured")}</p>
          ) : (
            <p className="text-sm text-muted-foreground">{t("settings.tiingoKeyMissing")}</p>
          )}

          {(saveTiingoKey.isError || deleteTiingoKey.isError || tiingoKeyStatus.isError) && (
            <p role="alert" className="text-sm text-destructive">
              {t("settings.tiingoKeyError")}
            </p>
          )}
        </form>
          </div>
        </div>
      </section>
      <AgentSection />
      <div id="settings-data" className="scroll-mt-6 space-y-8 border-t border-border pt-6"><DataManagementSection /><ContinuousBackupSection /></div>
      <section id="settings-diagnostics" aria-labelledby="settings-diagnostics-title" className="scroll-mt-6 space-y-5 border-t border-border pt-6">
        <h2 id="settings-diagnostics-title" className="text-base font-semibold">{t("settings.sections.diagnostics")}</h2>
        <div className="flex flex-col gap-2">
          <Label htmlFor="settings-log-level">{t("diagnosticsSettings.level")}</Label>
          <NativeSelect id="settings-log-level" form="settings-preferences" value={draft.logLevel || "off"} onChange={(event) => update({ logLevel: event.target.value })}>
            {["off", "error", "warn", "info", "debug"].map((level) => <option key={level} value={level}>{t(`diagnosticsSettings.levels.${level}`)}</option>)}
          </NativeSelect>
          <p className="text-xs text-muted-foreground">{t("diagnosticsSettings.help")}</p>
          {settings.data.logFilePath && <p className="break-all text-xs text-muted-foreground">{t("diagnosticsSettings.path")}: <span className="select-text font-mono">{settings.data.logFilePath}</span></p>}
        </div>

      </section>
      <div id="settings-about" className="scroll-mt-6 border-t border-border pt-6"><AboutPage /></div>
      <div id="settings-reset" className="scroll-mt-6">
      <section className="max-w-2xl rounded-2xl border border-destructive/30 bg-destructive/5 p-5" aria-labelledby="settings-reset-title">
        <h2 id="settings-reset-title" className="font-medium text-foreground">
          {t("settings.resetSectionTitle")}
        </h2>
        <p className="mt-1 max-w-xl text-sm leading-6 text-muted-foreground">{t("settings.resetSectionDescription")}</p>
        <AlertDialog>
          <AlertDialogTrigger className="mt-4 rounded-xl border border-destructive/40 px-3 py-2 text-sm font-medium text-destructive transition-colors hover:bg-destructive/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-destructive/50">
            {t("settings.resetSectionTitle")}
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t("settings.resetDialogTitle")}</AlertDialogTitle>
              <AlertDialogDescription>{t("settings.resetDialogDescription")}</AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
              <AlertDialogAction
                disabled={resetSettings.isPending}
                onClick={() =>
                  resetSettings.mutate(undefined, {
                    onSuccess: (defaults) => {
                      setDraftOverride(defaults);
                      applyLivePreferences(defaults);
                      toast.success(t("settings.resetDone"));
                    },
                  })
                }
              >
                {resetSettings.isPending ? t("common.pending") : t("settings.resetConfirmed")}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </section>

      </div>
      <div className="sticky bottom-3 z-10 flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-border bg-card/95 p-3 shadow-lg backdrop-blur">
        <p className="max-w-sm text-xs text-muted-foreground">{t(isDirty ? "settings.sections.unsaved" : "settings.sections.saveHelp")}</p>
        {(saveSettings.isError || resetSettings.isError) && (
          <p role="alert" className="text-sm text-destructive">
            {saveSettings.isError
              ? displayError(saveSettings.error, t("settings.saveError"))
              : displayError(resetSettings.error, t("settings.loadError"))}
          </p>
        )}

        <div className="flex flex-wrap gap-2">
          <Button type="submit" form="settings-preferences" disabled={!isDirty || saveSettings.isPending}>
            {saveSettings.isPending ? t("common.pending") : t("settings.saveChanges")}
          </Button>
          {isDirty && (
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setDraftOverride(null);
                toast.success(t("settings.changesDiscarded"));
              }}
            >
              {t("settings.discardChanges")}
            </Button>
          )}
        </div>
      </div>
        </div>
      </div>
    </div>
  );
}
