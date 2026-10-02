import { iconChoice } from "@/lib/iconCatalog";
import { bankLogo, bankLogoPresentation, bankLogoUrl } from "@/lib/bankLogos";
import { cn } from "@/lib/utils";

export type EntityIconKind = "member" | "institution" | "group" | "account" | "instrument";

const KIND_FALLBACKS: Record<EntityIconKind, string> = {
  member: "user", institution: "bank", group: "folder", account: "account", instrument: "investment",
};

export function EntityIcon({ iconKey, kind, className, label }: { iconKey?: string | null; kind: EntityIconKind; className?: string; label?: string }) {
  const logo = bankLogoPresentation(iconKey);
  if (logo) return <img src={bankLogoUrl(logo.file)} alt={label ?? ""} aria-hidden={label ? undefined : true}
    className={cn("size-4 shrink-0 rounded-sm bg-white object-contain", className)} loading="lazy" draggable={false} />;
  const choice = iconChoice(iconKey);
  if (choice?.file) return <img src={`${import.meta.env.BASE_URL}${choice.file}`} alt={label ?? ""} aria-hidden={label ? undefined : true}
    className={cn("size-6 shrink-0 object-contain", className)} loading="lazy" draggable={false} />;
  const Icon = choice?.icon ?? iconChoice(KIND_FALLBACKS[kind])!.icon!;
  return <Icon className={cn("size-4 shrink-0", className)} aria-hidden={label ? undefined : "true"} aria-label={label} />;
}

export function hasIcon(key: string): boolean { return Boolean(iconChoice(key)) || Boolean(bankLogo(key)); }
