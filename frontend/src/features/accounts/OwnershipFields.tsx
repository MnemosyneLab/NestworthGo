import type { Ref } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { FieldError } from "@/components/ui/field-error";
import { EntityIcon } from "@/components/icons/EntityIcon";

export interface OwnershipMember {
  id: string;
  name: string;
  iconKey?: string;
}

/**
 * OwnershipFields is the shared owner checklist with optional custom share
 * percentages, used by both the create wizard and the account editor.
 */
export function OwnershipFields({
  idPrefix,
  members,
  ownerIds,
  onToggleOwner,
  useCustomPercentages,
  onUseCustomPercentagesChange,
  percentages,
  onPercentageChange,
  showError = false,
  focusRef,
  submissionProps,
}: {
  idPrefix: string;
  members: readonly OwnershipMember[];
  ownerIds: readonly string[];
  onToggleOwner: (memberId: string) => void;
  useCustomPercentages: boolean;
  onUseCustomPercentagesChange: (value: boolean) => void;
  percentages: readonly string[];
  onPercentageChange: (index: number, value: string) => void;
  showError?: boolean;
  focusRef?: Ref<HTMLFieldSetElement>;
  submissionProps?: { "data-submission-field": string; "aria-invalid": boolean; "aria-describedby"?: string };
}) {
  const { t } = useTranslation();
  const description = [showError ? `${idPrefix}-error` : undefined, submissionProps?.["aria-describedby"]].filter(Boolean).join(" ") || undefined;
  const invalid = showError || Boolean(submissionProps?.["aria-invalid"]);
  return (
    <fieldset {...submissionProps} ref={focusRef} tabIndex={-1} aria-invalid={invalid} aria-describedby={description} className="flex flex-col gap-2">
      <legend className="text-sm font-medium">{t("accounts.owner")}</legend>
      <p className="text-xs text-muted-foreground">{t("accounts.ownershipHint")}</p>
      {members.map((member) => {
        const index = ownerIds.indexOf(member.id);
        const checked = index !== -1;
        return (
          <div key={member.id} className="flex items-center gap-2">
            <input type="checkbox" id={`${idPrefix}-${member.id}`} checked={checked} aria-invalid={invalid} aria-describedby={description} onChange={() => onToggleOwner(member.id)} className="size-4 accent-primary" />
            <Label htmlFor={`${idPrefix}-${member.id}`} className="flex-1 font-normal">
              <span className="flex items-center gap-2"><EntityIcon iconKey={member.iconKey} kind="member" />{member.name}</span>
            </Label>
            {useCustomPercentages && checked && (
              <Input
                aria-label={t("accounts.ownershipPercentageFor", { name: member.name })}
                className="w-20"
                value={percentages[index] ?? ""}
                onChange={(event) => onPercentageChange(index, event.target.value)}
                placeholder="%"
              />
            )}
          </div>
        );
      })}
      {ownerIds.length > 1 && (
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          <input type="checkbox" className="accent-primary" checked={useCustomPercentages} onChange={(event) => onUseCustomPercentagesChange(event.target.checked)} />
          {t("accounts.ownershipSharePlaceholder")}
        </label>
      )}
      {showError && <FieldError id={`${idPrefix}-error`}>{t("error.ownership.atLeastOneOwner")}</FieldError>}
    </fieldset>
  );
}
