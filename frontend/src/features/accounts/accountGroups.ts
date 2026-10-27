import { signedAccountAmount } from "@/lib/liabilityDisplay";
import i18n from "@/i18n";
import { addCanonical, compareCanonical, sortByCanonicalDesc } from "@/lib/money";
import type { AccountRecordDTO, AccountValuationDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

export const UNASSIGNED_INSTITUTION = "unassigned";

export type AccountInstitutionGroup = {
  key: string;
  label: string;
  records: AccountRecordDTO[];
};

function valuationAmount(record: AccountRecordDTO, valuation: AccountValuationDTO | undefined): string {
  return signedAccountAmount(valuation?.baseValue?.amount ?? "0", record.account.balanceSheetRole === "liability");
}

/** No complete-looking subtotal when any price, FX or currency is unavailable. */
export function accountGroupNet(records: AccountRecordDTO[], valuations: Map<string, AccountValuationDTO>): { amount: string; currency: string } | undefined {
  if (!records.length) return undefined;
  let amount = "0", currency = "";
  for (const record of records) {
    const valuation = valuations.get(record.account.id), base = valuation?.baseValue;
    if (!valuation?.complete || !base || (currency && currency !== base.currency)) return undefined;
    currency = base.currency;
    amount = addCanonical(amount, signedAccountAmount(base.amount, record.account.balanceSheetRole === "liability"));
  }
  return { amount, currency };
}

function groupTotal(records: AccountRecordDTO[], valuationByAccountId: Map<string, AccountValuationDTO>): string {
  return records.reduce((sum, record) => addCanonical(sum, valuationAmount(record, valuationByAccountId.get(record.account.id))) || sum, "0");
}

/**
 * Groups accounts by institution. Groups and rows sort by household
 * base value descending. Unassigned stays last.
 */
export function groupAccounts(records: AccountRecordDTO[], valuations: AccountValuationDTO[] = []): AccountInstitutionGroup[] {
  const valuationByAccountId = new Map(valuations.map((valuation) => [valuation.account.id, valuation]));
  const groups = new Map<string, { label: string; records: AccountRecordDTO[] }>();
  for (const record of records) {
    const key = record.account.institutionId || UNASSIGNED_INSTITUTION;
    const label = record.institutionName || "";
    const existing = groups.get(key);
    if (existing) {
      existing.records.push(record);
    } else {
      groups.set(key, { label, records: [record] });
    }
  }
  return [...groups.entries()]
    .map(([key, value]) => ({
      key,
      label: value.label,
      records: sortByCanonicalDesc(
        value.records,
        (record) => valuationAmount(record, valuationByAccountId.get(record.account.id)),
        (left, right) => left.account.name.localeCompare(right.account.name, i18n.language || undefined),
      ),
    }))
    .sort((left, right) => {
      if (left.key === UNASSIGNED_INSTITUTION) {
        return 1;
      }
      if (right.key === UNASSIGNED_INSTITUTION) {
        return -1;
      }
      const comparison = compareCanonical(
        groupTotal(right.records, valuationByAccountId),
        groupTotal(left.records, valuationByAccountId),
      );
      if (comparison !== 0) {
        return comparison;
      }
      return left.label.localeCompare(right.label, i18n.language || undefined);
    });
}
