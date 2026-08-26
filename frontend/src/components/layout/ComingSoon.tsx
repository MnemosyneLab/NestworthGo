import { useTranslation } from "react-i18next";

/**
 * ComingSoonPage is used for destinations whose product surface is not yet
 * implemented. It keeps the shell and localization usable while work is
 * deferred.
 */
export function ComingSoonPage({ titleKey }: { titleKey: string }) {
  const { t } = useTranslation();
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
      <h2 className="text-lg font-semibold text-foreground">{t(titleKey)}</h2>
      <p className="text-sm text-muted-foreground">{t("common.comingSoon")}</p>
    </div>
  );
}
