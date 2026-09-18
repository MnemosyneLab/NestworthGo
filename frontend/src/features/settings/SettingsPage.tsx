import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
import { useSettings, useSaveSettings, useResetSettings, useFXProviders, useTiingoKeyStatus, useSaveTiingoAPIKey, useDeleteTiingoAPIKey, useWorkerTokenStatus, useSaveWorkerAPIToken, useDeleteWorkerAPIToken, quoteCacheTtlOf, withQuoteCacheTtl, type QuoteCacheTTL } from "@/queries/settings";
import { useHistoryOrigin } from "@/queries/history";
import { useCatalog } from "@/queries/catalog";
import { AboutPage } from "@/features/about/AboutPage";
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
export function SettingsPage() {
  const { t } = useTranslation();
  const settings = useSettings();
  const saveSettings = useSaveSettings();
  const resetSettings = useResetSettings();
  const bootstrap = useBootstrap();
  const fxProviders = useFXProviders();
  const tiingoKeyStatus = useTiingoKeyStatus();
  const saveTiingoKey = useSaveTiingoAPIKey();
  const deleteTiingoKey = useDeleteTiingoAPIKey();
  const workerTokenStatus = useWorkerTokenStatus();
  const saveWorkerToken = useSaveWorkerAPIToken();
  const deleteWorkerToken = useDeleteWorkerAPIToken();
  const catalog = useCatalog();
  const origin = useHistoryOrigin();
  const setAppearance = useUiStore((state) => state.setAppearance);
  const setAccent = useUiStore((state) => state.setAccent);
  const [draftOverride, setDraftOverride] = useState<Settings | null>(null);
  const [tiingoKey, setTiingoKey] = useState("");
  const [workerToken, setWorkerToken] = useState("");
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
    draft.workerBaseURL !== settings.data.workerBaseURL ||
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

  const submitTiingoKey = (event: React.FormEvent) => {
    event.preventDefault();
    saveTiingoKey.mutate(tiingoKey, {
      onSuccess: () => {
        setTiingoKey("");
        toast.success(t("settings.tiingoKeySaved"));
      },
    });
  };

  const submitWorkerToken = (event: React.FormEvent) => {
    event.preventDefault();
    saveWorkerToken.mutate(workerToken, {
      onSuccess: () => {
        setWorkerToken("");
        toast.success(t("settings.workerTokenSaved"));
      },
    });
  };

  return (
    <div className="flex flex-col gap-8">
      {pageChrome}
      <PageIntro description={t("settings.subtitle")} />

      <form onSubmit={submit} className="flex max-w-2xl flex-col gap-5" aria-label={t("settings.formLabel")}>
        <h2 className="font-medium">{t("review.displayPreferences")}</h2>
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
            <NativeSelect
              id="settings-accent"
              value={draft.accent}
              onChange={(event) => update({ accent: event.target.value as Settings["accent"] })}
            >
              {(catalog.data?.accents ?? []).map((accent) => (
                <option key={accent} value={accent}>
                  {t(`option.accent.${accent}`)}
                </option>
              ))}
            </NativeSelect>
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
            <Label htmlFor="settings-currency">{t("review.householdCurrency")}</Label>
            <Input id="settings-currency" readOnly value={bootstrap.data?.household?.baseCurrency ?? "—"} />
            <p className="text-xs text-muted-foreground">{t("review.householdCurrencyHelp")}</p>
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
        </div>
        <h2 className="border-t border-border pt-5 font-medium">{t("settings.marketDataTitle")}</h2>
        <div className="grid gap-5 sm:grid-cols-2">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="settings-fx-provider">{t("settings.providers.fxProvider")}</Label>
            <NativeSelect
              id="settings-fx-provider"
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
              id="settings-quote-cache-ttl"
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

        <div className="flex flex-col gap-2 border-t border-border pt-5">
          <Label htmlFor="settings-log-level">{t("diagnosticsSettings.level")}</Label>
          <NativeSelect id="settings-log-level" value={draft.logLevel || "off"} onChange={(event) => update({ logLevel: event.target.value })}>
            {["off", "error", "warn", "info", "debug"].map((level) => <option key={level} value={level}>{t(`diagnosticsSettings.levels.${level}`)}</option>)}
          </NativeSelect>
          <p className="text-xs text-muted-foreground">{t("diagnosticsSettings.help")}</p>
          {settings.data.logFilePath && <p className="break-all text-xs text-muted-foreground">{t("diagnosticsSettings.path")}: <span className="select-text font-mono">{settings.data.logFilePath}</span></p>}
        </div>

        {(saveSettings.isError || resetSettings.isError) && (
          <p role="alert" className="text-sm text-destructive">
            {saveSettings.isError
              ? displayError(saveSettings.error, t("settings.saveError"))
              : displayError(resetSettings.error, t("settings.loadError"))}
          </p>
        )}

        <div className="flex max-w-xl flex-col gap-1.5">
          <Label htmlFor="settings-worker-url">{t("settings.workerBaseURL")}</Label>
          <Input
            id="settings-worker-url"
            type="url"
            autoComplete="url"
            value={draft.workerBaseURL}
            onChange={(event) => update({ workerBaseURL: event.target.value })}
            placeholder={t("settings.workerBaseURLPlaceholder")}
          />
          <p className="text-xs text-muted-foreground">{t("settings.workerBaseURLHelp")}</p>
        </div>

        <div className="flex flex-wrap gap-2">
          <Button type="submit" disabled={!isDirty || saveSettings.isPending}>
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
      </form>

      <section className="max-w-2xl rounded-lg border border-border bg-card p-5" aria-labelledby="settings-market-data-title">
        <h2 id="settings-market-data-title" className="font-medium text-foreground">
          {t("review.credentialsTitle")}
        </h2>
        <p className="mt-1 text-sm leading-6 text-muted-foreground">{t("review.credentialsHelp")}</p>
        <form onSubmit={submitTiingoKey} className="mt-4 flex max-w-xl flex-col gap-2">
          <Label htmlFor="settings-tiingo-key">{t("settings.tiingoKey")}</Label>
          <div className="flex flex-col gap-2 sm:flex-row">
            <input
              id="settings-tiingo-key"
              type="password"
              autoComplete="off"
              value={tiingoKey}
              onChange={(event) => setTiingoKey(event.target.value)}
              placeholder={t("settings.tiingoKeyPlaceholder")}
              className="min-w-0 flex-1 rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
            />
            <Button type="submit" disabled={!tiingoKey.trim() || saveTiingoKey.isPending}>
              {saveTiingoKey.isPending ? t("common.pending") : t("settings.saveTiingoKey")}
            </Button>
          </div>
          {tiingoKeyStatus.data?.configured ? (
            <p className="text-sm text-success-foreground">{t("settings.tiingoKeyConfigured")}</p>
          ) : (
            <p className="text-sm text-muted-foreground">{t("settings.tiingoKeyMissing")}</p>
          )}
          {tiingoKeyStatus.data?.configured && (
            <Button
              type="button"
              variant="outline"
              className="self-start"
              onClick={() => deleteTiingoKey.mutate()}
              disabled={deleteTiingoKey.isPending}
            >
              {deleteTiingoKey.isPending ? t("common.pending") : t("settings.removeTiingoKey")}
            </Button>
          )}
          {(saveTiingoKey.isError || deleteTiingoKey.isError || tiingoKeyStatus.isError) && (
            <p role="alert" className="text-sm text-destructive">
              {t("settings.tiingoKeyError")}
            </p>
          )}
        </form>
        <form onSubmit={submitWorkerToken} className="mt-5 flex max-w-xl flex-col gap-2">
          <Label htmlFor="settings-worker-token">{t("settings.workerToken")}</Label>
          <div className="flex flex-col gap-2 sm:flex-row">
            <input
              id="settings-worker-token"
              type="password"
              autoComplete="off"
              value={workerToken}
              onChange={(event) => setWorkerToken(event.target.value)}
              placeholder={t("settings.workerTokenPlaceholder")}
              className="min-w-0 flex-1 rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ring"
            />
            <Button type="submit" disabled={!workerToken.trim() || saveWorkerToken.isPending}>
              {saveWorkerToken.isPending ? t("common.pending") : t("settings.saveWorkerToken")}
            </Button>
          </div>
          {workerTokenStatus.data?.configured ? (
            <p className="text-sm text-success-foreground">{t("settings.workerTokenConfigured")}</p>
          ) : (
            <p className="text-sm text-muted-foreground">{t("settings.workerTokenMissing")}</p>
          )}
          {workerTokenStatus.data?.configured && (
            <Button
              type="button"
              variant="outline"
              className="self-start"
              onClick={() => deleteWorkerToken.mutate()}
              disabled={deleteWorkerToken.isPending}
            >
              {deleteWorkerToken.isPending ? t("common.pending") : t("settings.removeWorkerToken")}
            </Button>
          )}
          {(saveWorkerToken.isError || deleteWorkerToken.isError || workerTokenStatus.isError) && (
            <p role="alert" className="text-sm text-destructive">
              {t("settings.workerTokenError")}
            </p>
          )}
        </form>
      </section>

      <section className="max-w-2xl rounded-lg border border-destructive/30 bg-destructive/5 p-5" aria-labelledby="settings-reset-title">
        <h2 id="settings-reset-title" className="font-medium text-foreground">
          {t("settings.resetSectionTitle")}
        </h2>
        <p className="mt-1 max-w-xl text-sm leading-6 text-muted-foreground">{t("settings.resetSectionDescription")}</p>
        <AlertDialog>
          <AlertDialogTrigger className="mt-4 rounded-md border border-destructive/40 px-3 py-2 text-sm font-medium text-destructive hover:bg-destructive/10">
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

      <DataManagementSection />

      <div className="max-w-2xl">
        <AboutPage />
      </div>
    </div>
  );
}
