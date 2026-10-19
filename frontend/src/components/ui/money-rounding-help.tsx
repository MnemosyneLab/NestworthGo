import { useTranslation } from "react-i18next";
import { InfoPopover } from "./info-popover";

/** Presentation guidance only; all totals remain authoritative backend values. */
export function MoneyRoundingHelp() {
  const { t } = useTranslation();
  return <InfoPopover label={t("moneyDisplay.aboutRounding")}>{t("moneyDisplay.roundingHelp")}</InfoPopover>;
}
