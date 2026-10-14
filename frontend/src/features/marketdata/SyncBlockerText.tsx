import { useTranslation } from "react-i18next";
import { useInstruments } from "@/queries/investments";
import { instrumentDisplayLabel } from "@/lib/instrumentDisplay";

/** Business labels for repair blockers; target keys remain internal identities. */
export function SyncBlockerText({ item }: { item: { targetKey: string; reason: string; code: string } }) {
  const { t } = useTranslation();
  const instruments = useInstruments(true);
  const instrument = instruments.data?.find(value => `instrument/${value.id}` === item.targetKey || `instrument:${value.id}` === item.targetKey);
  const target = instrument ? instrumentDisplayLabel(instrument) : item.targetKey === "snapshots" ? t("marketData.snapshotTarget") : /^fx[/:]/.test(item.targetKey) ? item.targetKey.slice(3).replace(/\//g, " / ") : t("portfolio.unknownInstrument");
  const reason = t(`syncReasons.${item.reason || item.code}`, { defaultValue: t("syncReasons.unavailable") });
  return <>{t("marketData.blocker", { target, reason })}</>;
}
