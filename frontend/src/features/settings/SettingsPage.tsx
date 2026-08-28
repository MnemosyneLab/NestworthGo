import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
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
import { PageHeader } from "@/components/layout/PageHeader";
import { ErrorState, LoadingState } from "@/components/layout/PageState";
import { useSettings, useSaveSettings, useResetSettings, useSupportedCurrencies, useFXProviders } from "@/queries/settings";
import { useHistoryOrigin } from "@/queries/history";
import { useCatalog } from "@/queries/catalog";
import { AboutPage } from "@/features/about/AboutPage";
import { useUiStore, type Appearance } from "@/stores/ui";
import { setLanguage, languageOptionKey } from "@/i18n";
import { displayError } from "@/lib/display";
import type { Settings } from "../../../bindings/github.com/waltwang/nestworth-go/internal/settings/models";
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
  const currencies = useSupportedCurrencies();
  const fxProviders = useFXProviders();
  const catalog = useCatalog();
  const origin = useHistoryOrigin();
  const setAppearance = useUiStore((state) => state.setAppearance);
  const [draftOverride, setDraftOverride] = useState<Settings | null>(null);

  if (settings.isLoading) {
    return <LoadingState label={t("ui.state.loadingPage")} />;
  }

  if (settings.isError || !settings.data) {
    return (
      <ErrorState
        title={t("settings.loadError")}
        description={t("ui.state.errorDescription")}
        onRetry={() => settings.refetch()}
        retryLabel={t("common.retryAction")}
      />
    );
  }

  const draft = draftOverride ?? settings.data;
  if (!draft) {
    return <LoadingState label={t("ui.state.loadingPage")} />;
  }

  const isDirty =
    draft.appearance !== settings.data.appearance ||
    draft.language !== settings.data.language ||
    draft.currency !== settings.data.currency ||
    draft.timezone !== settings.data.timezone ||
    draft.fx_provider !== settings.data.fx_provider;
  const update = (patch: Partial<Settings>) => {
    setDraftOverride((current) => ({ ...(current ?? settings.data), ...patch }));
  };

  const applyLivePreferences = (value: Settings) => {
    setAppearance(value.appearance as Appearance);
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

  return (
    <div className="flex flex-col gap-8">
      <PageHeader title={t("settings.title")} description={t("settings.subtitle")} />

      <form onSubmit={submit} className="flex max-w-2xl flex-col gap-5" aria-label={t("settings.formLabel")}>
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
            <Label htmlFor="settings-currency">{t("accounts.currency")}</Label>
            <NativeSelect
              id="settings-currency"
              value={draft.currency}
              onChange={(event) => update({ currency: event.target.value })}
            >
              {(currencies.data ?? [draft.currency]).map((currency) => (
                <option key={currency} value={currency}>
                  {currency}
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
            <Label htmlFor="settings-fx-provider">{t("settings.providers.fxProvider")}</Label>
            <NativeSelect
              id="settings-fx-provider"
              value={draft.fx_provider}
              onChange={(event) => update({ fx_provider: event.target.value })}
            >
              {(fxProviders.data ?? [draft.fx_provider]).map((provider) => (
                <option key={provider} value={provider}>
                  {t(`settings.provider.${provider}`, { defaultValue: provider })}
                </option>
              ))}
            </NativeSelect>
          </div>
        </div>

        {currencies.isError && <p className="text-sm text-warning-foreground">{t("onboarding.currencyLoadError")}</p>}

        {(saveSettings.isError || resetSettings.isError) && (
          <p role="alert" className="text-sm text-destructive">
            {saveSettings.isError
              ? displayError(saveSettings.error, t("settings.saveError"))
              : displayError(resetSettings.error, t("settings.loadError"))}
          </p>
        )}

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

      <div className="max-w-2xl">
        <AboutPage />
      </div>
    </div>
  );
}
