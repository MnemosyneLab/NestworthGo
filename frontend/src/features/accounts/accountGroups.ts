import { addCanonical, compareCanonical, sortByCanonicalDesc } from "@/lib/money";
import type { AccountRecordDTO, AccountValuationDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

export const UNASSIGNED_INSTITUTION = "unassigned";

export type AccountInstitutionGroup = {
  key: string;
  label: string;
  records: AccountRecordDTO[];
};

function valuationAmount(valuation: AccountValuationDTO | undefined): string {
  return valuation?.baseValue?.amount ?? "0";
}

function groupTotal(records: AccountRecordDTO[], valuationByAccountId: Map<string, AccountValuationDTO>): string {
  return records.reduce((sum, record) => addCanonical(sum, valuationAmount(valuationByAccountId.get(record.account.id))) || sum, "0");
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
        (record) => valuationAmount(valuationByAccountId.get(record.account.id)),
        (left, right) => left.account.name.localeCompare(right.account.name),
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
      return left.label.localeCompare(right.label);
    });
}
