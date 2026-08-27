import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import logoMark from "@/assets/brand/logo-mark.png";
import wordmark from "@/assets/brand/wordmark.png";

/**
 * BrandLockup is the product mark used in the shell sidebar and onboarding.
 * The mark is decorative next to the wordmark; when the wordmark is hidden
 * the accessible name stays on the cluster.
 */
export function BrandLockup({ compact = false, className }: { compact?: boolean; className?: string }) {
  const { t } = useTranslation();
  const name = t("app.name");

  return (
    <span translate="no" className={cn("flex min-w-0 items-center gap-2", className)}>
      <img src={logoMark} alt="" width={28} height={28} className="size-7 shrink-0" />
      {compact ? (
        <span className="sr-only">{name}</span>
      ) : (
        <img
          src={wordmark}
          alt={name}
          width={120}
          height={24}
          className="h-6 w-auto max-w-[7.5rem] object-contain object-left"
        />
      )}
    </span>
  );
}
