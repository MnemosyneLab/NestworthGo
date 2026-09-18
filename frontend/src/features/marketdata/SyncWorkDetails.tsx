import { useTranslation } from "react-i18next";
import type { SyncItemDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata/models";

export function SyncWorkDetails({ items = [], current }: { items?: SyncItemDTO[] | null; current?: SyncItemDTO | null }) {
  const { t } = useTranslation();
  const rows = current ? [current, ...(items ?? [])] : (items ?? []);
  if (!rows.length) return null;
  return (
    <div className="flex flex-col gap-2 text-sm" data-testid="sync-work-details">
      <p className="font-medium">{t("marketData.work.title")}</p>
      <ul className="max-h-80 space-y-2 overflow-y-auto" aria-live="polite">
        {rows.map((item, index) => (
          <li key={`${item.targetKey}-${item.kind}-${item.startDate}-${index}`} className="rounded-md border p-3">
            <p className="font-medium">{item.label || item.symbol || item.targetKey}{item.symbol && item.symbol !== item.label ? ` · ${item.symbol}` : ""}</p>
            <p>{t(`marketData.work.${item.kind === "instrument" || item.kind === "fx" ? `${item.startDate ? "history" : "latest"}_${item.kind}` : item.kind}`, { defaultValue: item.kind })} · {t(`marketData.work.${item.status}`, { defaultValue: item.status })}</p>
            {item.provider && <p className="text-muted-foreground">{item.provider}</p>}
            {item.startDate && <p>{item.startDate} → {item.endDate}</p>}
            {item.detail && <p className="text-muted-foreground">{t(`marketData.work.${item.detail}`, { defaultValue: item.detail })}</p>}
          </li>
        ))}
      </ul>
    </div>
  );
}
