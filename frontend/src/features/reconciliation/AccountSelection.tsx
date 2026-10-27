import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { useMembers } from "@/queries/directory";
import { displayEnum } from "@/lib/display";
import type { AccountRecordDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

/** Filtering changes visibility only; bulk actions preserve hidden selections. */
export function AccountSelection({ accounts, selected, onChange }: {
  accounts: AccountRecordDTO[];
  selected: string[];
  onChange: (ids: string[]) => void;
}) {
  const { t } = useTranslation();
  const id = useId();
  const members = useMembers(true);
  const [owner, setOwner] = useState("");
  const [type, setType] = useState("");
  const [institution, setInstitution] = useState("");
  const visible = accounts.filter(a => (!owner || a.ownership?.some(s => s.memberId === owner))
    && (!type || a.account.accountType === type)
    && (!institution || (institution === "none" ? !a.account.institutionId : a.account.institutionId === institution)));
  const visibleIds = new Set(visible.map(a => a.account.id));
  const visibleSelected = selected.filter(value => visibleIds.has(value)).length;
  const ownerNames = (record: AccountRecordDTO) => (record.ownership ?? []).map(s => members.data?.find(m => m.id === s.memberId)?.name ?? t("common.unknownValue")).join(" · ");
  const institutions = new Map(accounts.filter(a => a.account.institutionId).map(a => [a.account.institutionId!, a.institutionName || t("common.unknownValue")]));
  return <div className="flex min-w-0 flex-col gap-3">
    {members.isError && <div role="alert" className="text-sm text-muted-foreground">{t("members.loadError")} <Button type="button" variant="link" size="sm" onClick={() => void members.refetch()}>{t("common.retryAction")}</Button></div>}
    <div className="grid gap-3 sm:grid-cols-3">
      <div><Label htmlFor={`${id}-owner`}>{t("accounts.owner")}</Label><NativeSelect className="w-full" id={`${id}-owner`} value={owner} onChange={e => setOwner(e.target.value)}>
        <option value="">{t("reconciliation.allAccountFilters")}</option>
        {(members.data ?? []).filter(m => accounts.some(a => a.ownership?.some(s => s.memberId === m.id))).map(m => <option key={m.id} value={m.id}>{m.name}</option>)}
      </NativeSelect></div>
      <div><Label htmlFor={`${id}-type`}>{t("accounts.accountType")}</Label><NativeSelect className="w-full" id={`${id}-type`} value={type} onChange={e => setType(e.target.value)}>
        <option value="">{t("reconciliation.allAccountFilters")}</option>
        {[...new Set(accounts.map(a => a.account.accountType))].sort().map(value => <option key={value} value={value}>{displayEnum(t, "enum", value)}</option>)}
      </NativeSelect></div>
      <div><Label htmlFor={`${id}-institution`}>{t("reconciliation.institution")}</Label><NativeSelect className="w-full" id={`${id}-institution`} value={institution} onChange={e => setInstitution(e.target.value)}>
        <option value="">{t("reconciliation.allAccountFilters")}</option>
        {[...institutions].map(([value, name]) => <option key={value} value={value}>{name}</option>)}
        <option value="none">{t("reconciliation.noInstitution")}</option>
      </NativeSelect></div>
    </div>
    <div className="flex flex-wrap gap-2">
      <Button type="button" variant="ghost" size="sm" disabled={!owner && !type && !institution} onClick={() => { setOwner(""); setType(""); setInstitution(""); }}>{t("reconciliation.clearFilters")}</Button>
      <Button type="button" variant="outline" size="sm" disabled={!visible.length || visibleSelected === visible.length} onClick={() => onChange([...new Set([...selected, ...visibleIds])])}>{t("reconciliation.selectVisible")}</Button>
      <Button type="button" variant="outline" size="sm" disabled={!visibleSelected} onClick={() => onChange(selected.filter(value => !visibleIds.has(value)))}>{t("reconciliation.deselectVisible")}</Button>
      <Button type="button" variant="ghost" size="sm" disabled={!selected.length} onClick={() => onChange([])}>{t("reconciliation.deselectAll")}</Button>
    </div>
    <p className="text-sm text-muted-foreground" role="status">{t("reconciliation.selectionSummary", { selected: selected.length, total: accounts.length, visible: visible.length, visibleSelected, hidden: selected.length - visibleSelected })}</p>
    {!visible.length && <p className="text-sm text-muted-foreground">{t("reconciliation.noAccountMatches")}</p>}
    <div className="grid gap-2 sm:grid-cols-2">
      {visible.map(a => <label key={a.account.id} className="flex min-w-0 cursor-pointer items-center gap-3 rounded-xl border border-border bg-card px-3 py-2.5 text-sm focus-within:ring-2 focus-within:ring-primary/50 has-[:checked]:border-primary/40 has-[:checked]:bg-primary/5">
        <input type="checkbox" aria-label={`${a.account.name} · ${a.account.defaultCurrency}`} className="size-4 shrink-0 accent-primary" checked={selected.includes(a.account.id)} onChange={e => onChange(e.target.checked ? [...new Set([...selected, a.account.id])] : selected.filter(value => value !== a.account.id))} />
        <span className="min-w-0 break-words"><span className="font-medium">{a.account.name} · {a.account.defaultCurrency}</span><span className="block text-xs text-muted-foreground">{[ownerNames(a), a.institutionName, displayEnum(t, "enum", a.account.accountType)].filter(Boolean).join(" · ")}</span></span>
      </label>)}
    </div>
  </div>;
}
