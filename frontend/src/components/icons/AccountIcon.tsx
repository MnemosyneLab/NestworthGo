import { useInstitutions } from "@/queries/directory";
import { resolveAccountIcon, type AccountIconSource } from "@/lib/accountIcons";
import { EntityIcon } from "./EntityIcon";

/** Read the live directory, including archived institutions, without copying an inherited key into the account. */
export function AccountIcon({ account, className, label }: { account: AccountIconSource; className?: string; label?: string }) {
  const institutions = useInstitutions(true);
  return <EntityIcon iconKey={resolveAccountIcon(account, institutions.data)} kind="account" className={className} label={label} />;
}
