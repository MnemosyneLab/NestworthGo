import { useTranslation } from "react-i18next";
import { useSettings } from "@/queries/settings";
import { formatTimestamp } from "@/lib/time";

export function useReconciliationTimestamp() {
  const settings = useSettings();
  const { i18n } = useTranslation();
  return (value: string) => formatTimestamp(value, settings.data?.timezone, i18n.language);
}
