import { useTranslation } from "react-i18next";

/**
 * ComingSoonPage is the Phase 3 placeholder for every nav destination
 * that does not have a real page yet, mirroring internal/ui's existing
 * NewComingSoonPage pattern (implementation plan Phase 3's smoke-test
 * requirement: the app shell must render with zero real pages, in all
 * three locales, without runtime errors). Phase 4/5 replace each of
 * these one at a time.
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
