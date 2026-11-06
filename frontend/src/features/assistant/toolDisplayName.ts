import type { TFunction } from "i18next";
import { assistantEn } from "@/i18n/assistant";

export function toolDisplayName(t: TFunction, source: string): string {
  return t(Object.prototype.hasOwnProperty.call(assistantEn.aiChat.source, source) ? `aiChat.source.${source}` : "aiChat.unknownTool");
}
