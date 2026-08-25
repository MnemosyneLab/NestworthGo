import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { useSettings, useSaveSettings, useResetSettings } from "@/queries/settings";
import { useSupportedCurrencies } from "@/queries/settings";
import { useUiStore, type Appearance } from "@/stores/ui";
import { setLanguage } from "@/i18n";
import type { Settings } from "../../../bindings/github.com/waltwang/nestworth-go/internal/settings/models";

/**
 * SettingsPage implements the persisted-preferences page
 * (implementation plan Phase 5) through SettingsService, which already
 * delegates FX-provider changes to MarketDataService.SetFXProvider
 * exactly as Controller.updatePreference does today (technical design
 * Sec6). Changing appearance/language here also updates the live
 * stores/ui.ts value and i18next instance so the effect is immediate,
 * not only after a reload.
 */
export function SettingsPage() {
  const { t } = useTranslation();
  const settings = useSettings();
  const saveSettings = useSaveSettings();
  const resetSettings = useResetSettings();
  const currencies = useSupportedCurrencies();
  const setAppearance = useUiStore((state) => state.setAppearance);
  const [draft, setDraft] = useState<Settings | null>(null);
  // Initializes local editable state from the loaded Settings exactly
  // once (React's render-phase "adjusting state when data arrives"
  // pattern, rather than a setState-in-effect that would cause an extra
  // render): https://react.dev/learn/you-might-not-need-an-effect
  const [syncedFrom, setSyncedFrom] = useState<Settings | null>(null);
  if (settings.data && settings.data !== syncedFrom) {
    setSyncedFrom(settings.data);
    setDraft(settings.data);
  }

  if (!draft) {
    return <p className="text-sm text-muted-foreground">{t("common.comingSoon")}</p>;
  }

  const update = (patch: Partial<Settings>) => setDraft({ ...draft, ...patch });

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    saveSettings.mutate(draft, {
      onSuccess: () => {
        toast.success(t("common.add"));
        setAppearance(draft.appearance as Appearance);
        setLanguage(draft.language);
      },
    });
  };

  return (
    <form onSubmit={submit} className="flex max-w-md flex-col gap-4" aria-label="Settings">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="settings-appearance">{t("settings.appearance.mode")}</Label>
        <select
          id="settings-appearance"
          value={draft.appearance}
          onChange={(event) => update({ appearance: event.target.value as Settings["appearance"] })}
          className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground"
        >
          <option value="system">system</option>
          <option value="light">light</option>
          <option value="dark">dark</option>
        </select>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="settings-language">{t("settings.language.language")}</Label>
        <select
          id="settings-language"
          value={draft.language}
          onChange={(event) => update({ language: event.target.value as Settings["language"] })}
          className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground"
        >
          <option value="system">system</option>
          <option value="en">en</option>
          <option value="zh-CN">zh-CN</option>
          <option value="zh-TW">zh-TW</option>
        </select>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="settings-currency">{t("accounts.currency")}</Label>
        <select
          id="settings-currency"
          value={draft.currency}
          onChange={(event) => update({ currency: event.target.value })}
          className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground"
        >
          {(currencies.data ?? [draft.currency]).map((currency) => (
            <option key={currency} value={currency}>
              {currency}
            </option>
          ))}
        </select>
      </div>

      {saveSettings.isError && (
        <p role="alert" className="text-sm text-destructive">
          {(saveSettings.error as Error).message}
        </p>
      )}

      <div className="flex gap-2">
        <Button type="submit" disabled={saveSettings.isPending}>
          {t("common.add")}
        </Button>
        <Button
          type="button"
          variant="outline"
          onClick={() =>
            resetSettings.mutate(undefined, {
              onSuccess: (defaults) => {
                setDraft(defaults);
                setAppearance(defaults.appearance as Appearance);
                setLanguage(defaults.language);
              },
            })
          }
        >
          {t("common.cancel")}
        </Button>
      </div>
    </form>
  );
}
