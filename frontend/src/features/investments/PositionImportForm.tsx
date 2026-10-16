import { useTranslation } from "react-i18next";
import { RecordChangeForm } from "@/features/history/RecordChangeForm";
import { emptyChangeRequest } from "@/features/history/activityToCommand";
import { ChangeCommandKind, type HistoryOriginDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";
import type { AccountRecordDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

/** Existing holdings are imported into dated history, never recorded as a cash-funded trade. */
export function PositionImportForm({ account, originOverride, onRecorded }: {
  account?: AccountRecordDTO["account"];
  originOverride?: HistoryOriginDTO;
  onRecorded: () => void;
}) {
  const { t } = useTranslation();
  const kind = ChangeCommandKind.ChangePositionImport;
  return <div className="flex flex-col gap-3">
    <p className="text-sm text-muted-foreground">{t("accounts.recordPositionHelp")}</p>
    <RecordChangeForm key={account?.id ?? "all-accounts"}
      initial={{ ...emptyChangeRequest(kind, account?.defaultCurrency ?? ""), accountId: account?.id, currency: account?.defaultCurrency }}
      prefill={account ? { account } : undefined}
      lock={{ kind, hideKind: true, accountId: account?.id }}
      originOverride={originOverride}
      onRecorded={onRecorded} />
  </div>;
}
