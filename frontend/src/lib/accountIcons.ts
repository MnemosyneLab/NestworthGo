import { ACCOUNT_TYPE_ICONS } from "./defaultIcons";

export type AccountIconSource = { iconKey?: string | null; institutionId?: string | null; accountType: string };
export function resolveAccountIcon(account: AccountIconSource, institutions?: { id: string; iconKey: string }[] | null) {
  return account.iconKey || institutions?.find((item) => item.id === account.institutionId)?.iconKey || ACCOUNT_TYPE_ICONS[account.accountType] || "account";
}
