import logos from "./bankLogos.json";
import regional from "./regionalBankLogos.json";
import presentations from "./bankLogoPresentation.json";

// Keep the full original catalog for validation and old persisted IDs.
export const BANK_LOGOS = [...logos, ...regional];
// Wordmarks remain resolvable for saved settings, but are not new choices.
export const BANK_LOGO_PRESENTATIONS = presentations;
export const BANK_LOGO_CHOICES = presentations.filter(logo => logo.kind === "symbol");
const byKey = new Map(BANK_LOGOS.map((logo) => [logo.key, logo]));
const displayByKey = new Map(presentations.flatMap((logo) =>
  [logo.key, ...logo.legacyKeys].map((key) => [key, logo] as const)));
export function bankLogo(key?: string | null) { return byKey.get(key ?? ""); }
export function bankLogoPresentation(key?: string | null) { return displayByKey.get(key ?? ""); }
export function bankLogoUrl(file: string) { return `${import.meta.env.BASE_URL}bank-logos/${file}`; }
