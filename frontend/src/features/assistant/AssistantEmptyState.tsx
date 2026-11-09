import { useTranslation } from "react-i18next";
import { Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { IconTile } from "@/components/ui/icon-tile";

const suggestions = ["suggestionPosition", "suggestionChange", "suggestionPage"] as const;

/** Empty transcript with starter questions. A suggestion only fills the draft;
 * it never sends, so the user still reviews and submits every request. */
export function AssistantEmptyState({ onSuggest, disabled = false }: { onSuggest?: (text: string) => void; disabled?: boolean }) {
  const { t } = useTranslation();
  return <div className="flex min-h-48 flex-col items-center justify-center gap-4 py-6 text-center">
    <IconTile icon={Sparkles} tone={1} size="lg" />
    <p className="max-w-xs text-sm text-muted-foreground">{t("aiChat.empty")}</p>
    {onSuggest && <div role="group" aria-label={t("aiChat.suggestionsLabel")} className="flex w-full max-w-xs flex-col gap-2">
      {suggestions.map(key => <Button key={key} type="button" variant="outline" disabled={disabled}
        className="h-auto min-h-11 justify-start whitespace-normal px-3 py-2 text-left text-xs font-normal xl:min-h-9"
        onClick={() => onSuggest(t(`aiChat.${key}`))}>{t(`aiChat.${key}`)}</Button>)}
    </div>}
  </div>;
}
