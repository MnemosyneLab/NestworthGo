import { useTranslation } from "react-i18next";
import { formatCivilDateRange } from "@/lib/time";
import type { SyncItemDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata/models";

const PLAN_DETAILS = new Set(["missing_or_recheck", "missing_or_due", "recently_checked"]);
const ADVERSE_STATUSES = new Set(["failed", "blocked", "rate_limited"]);

type Translator = (key: string, options?: Record<string, unknown>) => string;

function civilDateSpan(value: string | undefined): { start: string; end: string } | undefined {
  const match = /^(\d{4}-\d{2}-\d{2})(?:\.\.(\d{4}-\d{2}-\d{2}))?$/.exec(value?.trim() ?? "");
  if (!match) return undefined;
  return { start: match[1], end: match[2] ?? match[1] };
}

function exactText(t: Translator, key: string, options?: Record<string, unknown>): string | undefined {
  const value = t(key, { ...options, defaultValue: "" });
  const text = typeof value === "string" ? value.trim() : "";
  if (!text || text === key) return undefined;
  return text;
}

// Dates already render from the range fields. Unknown text is not a connection failure.
function workDetailText(t: Translator, item: SyncItemDTO): string | undefined {
  const detail = item.detail?.trim() ?? "";
  if (!detail || civilDateSpan(detail)) return undefined;
  const mayExplainFailure = ADVERSE_STATUSES.has(item.status) || item.status === "skipped";
  if (mayExplainFailure) {
    const reason = exactText(t, `syncReasons.${detail}`)
      ?? exactText(t, detail, { ns: "errorCode" })
      ?? exactText(t, `dataHealth.reason.${detail}`);
    if (reason) return reason;
  }
  if (PLAN_DETAILS.has(detail)) return exactText(t, `marketData.work.${detail}`);
  return undefined;
}

export function SyncWorkDetails({ items = [], current }: { items?: SyncItemDTO[] | null; current?: SyncItemDTO | null }) {
  const { t } = useTranslation();
  const rows = current ? [current, ...(items ?? [])] : (items ?? []);
  if (!rows.length) return null;
  return (
    <div className="flex flex-col gap-2 text-sm" data-testid="sync-work-details">
      <p className="font-medium">{t("marketData.work.title")}</p>
      <ul className="max-h-80 space-y-2 overflow-y-auto" aria-live="polite">
        {rows.map((item, index) => {
          const span = civilDateSpan(item.detail);
          const start = item.startDate || span?.start;
          const end = item.endDate || span?.end || start;
          const ranged = Boolean(start);
          const kind = item.kind === "instrument" || item.kind === "fx" ? `${ranged ? "history" : "latest"}_${item.kind}` : item.kind;
          const detail = workDetailText(t, item);
          return (
            <li key={`${item.targetKey}-${item.kind}-${item.startDate}-${index}`} className="rounded-md border p-3">
              <p className="font-medium">{item.label || item.symbol || t("portfolio.unknownInstrument")}{item.symbol && item.symbol !== item.label ? ` · ${item.symbol}` : ""}</p>
              <p>{t(`marketData.work.${kind}`, { defaultValue: t("common.unknownValue") })} · {t(`marketData.work.${item.status}`, { defaultValue: t("common.unknownValue") })}</p>
              {item.provider && <p className="text-muted-foreground">{item.provider}</p>}
              {start && <p>{formatCivilDateRange(start, end || start)}</p>}
              {detail && <p className="text-muted-foreground">{detail}</p>}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
