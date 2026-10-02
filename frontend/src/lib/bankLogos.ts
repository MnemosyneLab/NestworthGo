import logos from "./bankLogos.json";

export const BANK_LOGOS = logos;
const byKey = new Map(logos.map((logo) => [logo.key, logo]));
export function bankLogo(key?: string | null) { return byKey.get(key ?? ""); }
export function bankLogoUrl(file: string) { return `${import.meta.env.BASE_URL}bank-logos/${file}`; }
