import { useMemo, useState } from "react";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { IconPicker } from "@/components/forms/IconPicker";
import { ImagePicker } from "@/components/forms/ImagePicker";
import { useMembers, useInstitutions, useGroups } from "@/queries/directory";
import { useSupportedCurrencies } from "@/queries/settings";
import {
  PRIMARY_CATEGORIES,
  SECONDARY_CATEGORIES_BY_PRIMARY,
  TRACKING_MODES_BY_PRIMARY,
  defaultSecondaryCategory,
  defaultTrackingMode,
  type PrimaryCategory,
} from "@/features/accounts/accountTaxonomy";
import type { CreateAccountRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account/models";
import type { AccountRecordDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { displayEnum } from "@/lib/display";

const accountFormSchema = z.object({
  name: z.string().trim().min(1),
  primaryCategory: z.string(),
  secondaryCategory: z.string(),
  trackingMode: z.string(),
  defaultCurrency: z.string().length(3),
  initialAmount: z.string().optional(),
  ownerIds: z.array(z.string()).min(1),
  ownershipPercentages: z.array(z.string()).optional(),
  institutionId: z.string().optional(),
  groupId: z.string().optional(),
  includeInNetWorth: z.boolean(),
  includeInInvestment: z.boolean(),
  includeInLiquidAssets: z.boolean(),
});

export type AccountFormValues = z.infer<typeof accountFormSchema>;

const TOTAL_OWNERSHIP_BPS = 10000;

function ownershipShares(ownerIds: string[], percentages: string[] | undefined, useCustom: boolean): { memberId: string; shareBps: number }[] {
  if (useCustom && percentages && percentages.length === ownerIds.length) {
    return ownerIds.map((memberId, index) => ({
      memberId,
      shareBps: Math.round(Number(percentages[index]) * 100),
    }));
  }
  const base = Math.floor(TOTAL_OWNERSHIP_BPS / ownerIds.length);
  const remainder = TOTAL_OWNERSHIP_BPS % ownerIds.length;
  return ownerIds.map((memberId, index) => ({
    memberId,
    shareBps: index < remainder ? base + 1 : base,
  }));
}

export type AccountFormExtras = {
  pendingImage?: string;
};

const emptyValues: AccountFormValues = {
  name: "",
  primaryCategory: "cash_equivalent",
  secondaryCategory: defaultSecondaryCategory("cash_equivalent"),
  trackingMode: defaultTrackingMode("cash_equivalent"),
  defaultCurrency: "USD",
  initialAmount: "",
  ownerIds: [],
  ownershipPercentages: [],
  institutionId: "",
  groupId: "",
  includeInNetWorth: true,
  includeInInvestment: false,
  includeInLiquidAssets: false,
};

function valuesFromRecord(record: AccountRecordDTO): AccountFormValues {
  const ownerIds = (record.ownership ?? []).map((share) => share.memberId);
  const percentages = (record.ownership ?? []).map((share) => String(share.shareBps / 100));
  const equal = percentages.length <= 1 || percentages.every((value) => value === percentages[0]);
  return {
    name: record.account.name,
    primaryCategory: record.account.primaryCategory,
    secondaryCategory: record.account.secondaryCategory,
    trackingMode: record.account.trackingMode,
    defaultCurrency: record.account.defaultCurrency,
    initialAmount: "",
    ownerIds,
    ownershipPercentages: equal ? [] : percentages,
    institutionId: record.account.institutionId ?? "",
    groupId: record.account.groupId ?? "",
    includeInNetWorth: record.account.includeInNetWorth,
    includeInInvestment: record.account.includeInInvestment,
    includeInLiquidAssets: record.account.includeInLiquidAssets,
  };
}

/**
 * AccountForm implements Account creation and metadata edit. Minimal
 * required fields come first; institution/group/icon/logo sit behind
 * progressive disclosure. The native image picker lives in that disclosure
 * so the keyboard-only create path (Tab through required fields to submit)
 * is unchanged.
 */
export function AccountForm({
  record,
  onSubmit,
  submitLabel,
  isSubmitting,
  submissionError,
}: {
  record?: AccountRecordDTO;
  onSubmit: (request: CreateAccountRequest, extras: AccountFormExtras) => void | Promise<void>;
  submitLabel: string;
  isSubmitting: boolean;
  submissionError?: string;
}) {
  const { t } = useTranslation();
  const members = useMembers();
  const institutions = useInstitutions();
  const groups = useGroups();
  const currencies = useSupportedCurrencies();
  const isEdit = Boolean(record);
  const [showMoreOptions, setShowMoreOptions] = useState(isEdit);
  const [iconKey, setIconKey] = useState(record?.account.iconKey ?? "");
  const [pendingImage, setPendingImage] = useState<string | undefined>(undefined);
  const initialValues = record ? valuesFromRecord(record) : emptyValues;
  const [useCustomPercentages, setUseCustomPercentages] = useState(
    (initialValues.ownershipPercentages ?? []).length > 0,
  );

  const {
    register,
    control,
    setValue,
    handleSubmit,
    formState: { errors },
  } = useForm<AccountFormValues>({ resolver: zodResolver(accountFormSchema), defaultValues: initialValues });

  const primaryCategory = useWatch({ control, name: "primaryCategory" }) as PrimaryCategory;
  const trackingMode = useWatch({ control, name: "trackingMode" });
  const ownerIds = useWatch({ control, name: "ownerIds" });
  const ownershipPercentages = useWatch({ control, name: "ownershipPercentages" }) ?? [];

  const secondaryOptions = useMemo(() => SECONDARY_CATEGORIES_BY_PRIMARY[primaryCategory] ?? [], [primaryCategory]);
  const trackingModeOptions = useMemo(() => TRACKING_MODES_BY_PRIMARY[primaryCategory] ?? [], [primaryCategory]);

  const handlePrimaryCategoryChange = (value: PrimaryCategory) => {
    setValue("primaryCategory", value);
    setValue("secondaryCategory", defaultSecondaryCategory(value));
    setValue("trackingMode", defaultTrackingMode(value));
    if (!isEdit) {
      setValue("includeInInvestment", value === "investment", { shouldDirty: true, shouldTouch: true });
    }
  };

  const toggleOwner = (memberId: string) => {
    const next = ownerIds.includes(memberId) ? ownerIds.filter((id) => id !== memberId) : [...ownerIds, memberId];
    setValue("ownerIds", next);
  };

  const submit = (values: AccountFormValues) => {
    const request: CreateAccountRequest = {
      name: values.name,
      primaryCategory: values.primaryCategory,
      secondaryCategory: values.secondaryCategory,
      trackingMode: values.trackingMode,
      defaultCurrency: values.defaultCurrency,
      includeInNetWorth: values.includeInNetWorth,
      includeInInvestment: values.includeInInvestment,
      includeInLiquidAssets: values.includeInLiquidAssets,
      ownership: ownershipShares(values.ownerIds, values.ownershipPercentages, useCustomPercentages),
      institutionId: values.institutionId || undefined,
      groupId: values.groupId || undefined,
      iconKey: iconKey || undefined,
      initialAmount: isEdit || values.trackingMode === "holdings" ? "" : values.initialAmount || "0",
    };
    onSubmit(request, { pendingImage });
  };

  return (
    <form onSubmit={handleSubmit(submit)} className="flex flex-col gap-4" aria-label={t("accounts.formLabel")}>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="account-name">{t("accounts.name")}</Label>
        <Input id="account-name" {...register("name")} />
        {errors.name && (
          <p role="alert" className="text-xs text-destructive">
            {t("error.validation.notEmpty")}
          </p>
        )}
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="account-primary-category">{t("accounts.category")}</Label>
        <NativeSelect
          id="account-primary-category"
          value={primaryCategory}
          onChange={(event) => handlePrimaryCategoryChange(event.target.value as PrimaryCategory)}
        >
          {PRIMARY_CATEGORIES.map((category) => (
            <option key={category} value={category}>
              {displayEnum(t, "enum", category)}
            </option>
          ))}
        </NativeSelect>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="account-secondary-category">{t("accounts.secondaryCategory")}</Label>
        <NativeSelect id="account-secondary-category" {...register("secondaryCategory")}>
          {secondaryOptions.map((category) => (
            <option key={category} value={category}>
              {displayEnum(t, "enum", category)}
            </option>
          ))}
        </NativeSelect>
      </div>

      {trackingModeOptions.length > 1 && (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="account-tracking-mode">{t("accounts.trackingMode")}</Label>
          <NativeSelect id="account-tracking-mode" {...register("trackingMode")}>
            {trackingModeOptions.map((mode) => (
              <option key={mode} value={mode}>
                {displayEnum(t, "enum", mode)}
              </option>
            ))}
          </NativeSelect>
        </div>
      )}

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="account-currency">{t("accounts.currency")}</Label>
        <NativeSelect id="account-currency" {...register("defaultCurrency")}>
          {(currencies.data ?? ["USD"]).map((currency) => (
            <option key={currency} value={currency}>
              {currency}
            </option>
          ))}
        </NativeSelect>
      </div>

      {!isEdit && trackingMode !== "holdings" && (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="account-initial-amount">{t("accounts.amount")}</Label>
          <Input id="account-initial-amount" inputMode="decimal" {...register("initialAmount")} placeholder="0" />
        </div>
      )}

      <fieldset className="flex flex-col gap-2">
        <legend className="text-sm font-medium">{t("accounts.owner")}</legend>
        <p className="text-xs text-muted-foreground">{t("accounts.ownershipHint")}</p>
        {(members.data ?? []).map((member) => {
          const index = ownerIds.indexOf(member.id);
          const checked = index !== -1;
          return (
            <div key={member.id} className="flex items-center gap-2">
              <input
                type="checkbox"
                id={`owner-${member.id}`}
                checked={checked}
                onChange={() => toggleOwner(member.id)}
                className="size-4"
              />
              <Label htmlFor={`owner-${member.id}`} className="flex-1 font-normal">
                {member.name}
              </Label>
              {useCustomPercentages && checked && (
                <Input
                  aria-label={t("accounts.ownershipPercentageFor", { name: member.name })}
                  className="w-20"
                  value={ownershipPercentages[index] ?? ""}
                  onChange={(event) => {
                    const next = [...ownershipPercentages];
                    next[index] = event.target.value;
                    setValue("ownershipPercentages", next);
                  }}
                  placeholder="%"
                />
              )}
            </div>
          );
        })}
        {ownerIds.length > 1 && (
          <label className="flex items-center gap-2 text-xs text-muted-foreground">
            <input
              type="checkbox"
              checked={useCustomPercentages}
              onChange={(event) => setUseCustomPercentages(event.target.checked)}
            />
            {t("accounts.ownershipSharePlaceholder")}
          </label>
        )}
        {errors.ownerIds && (
          <p role="alert" className="text-xs text-destructive">
            {t("error.ownership.atLeastOneOwner")}
          </p>
        )}
      </fieldset>

      <div className="flex flex-col gap-2">
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" {...register("includeInNetWorth")} /> {t("accounts.includeInNetWorth")}
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" {...register("includeInInvestment")} /> {t("accounts.includeInInvestment")}
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" {...register("includeInLiquidAssets")} /> {t("accounts.includeInLiquidAssets")}
        </label>
      </div>

      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="self-start"
        onClick={() => setShowMoreOptions((value) => !value)}
        aria-expanded={showMoreOptions}
        aria-controls="account-more-options"
      >
        <span aria-hidden="true">{showMoreOptions ? "−" : "+"}</span>
        {t("accounts.details")}
      </Button>
      {showMoreOptions && (
        <div id="account-more-options" className="flex flex-col gap-3 rounded-md border border-border p-3">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="account-institution">{t("nav.institutions")}</Label>
            <NativeSelect id="account-institution" {...register("institutionId")}>
              <option value="">{t("accounts.none")}</option>
              {(institutions.data ?? []).map((institution) => (
                <option key={institution.id} value={institution.id}>
                  {institution.name}
                </option>
              ))}
            </NativeSelect>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="account-group">{t("nav.groups")}</Label>
            <NativeSelect id="account-group" {...register("groupId")}>
              <option value="">{t("accounts.none")}</option>
              {(groups.data ?? []).map((group) => (
                <option key={group.id} value={group.id}>
                  {group.name}
                </option>
              ))}
            </NativeSelect>
          </div>
          <IconPicker id="account-icon" value={iconKey} onChange={setIconKey} />
          <ImagePicker
            label={t("common.media")}
            value={pendingImage}
            existingAssetId={record?.account.logoAssetId}
            onChange={setPendingImage}
          />
        </div>
      )}

      {submissionError && (
        <p role="alert" className="text-sm text-destructive">
          {submissionError}
        </p>
      )}

      <Button type="submit" disabled={isSubmitting}>
        {submitLabel}
      </Button>
    </form>
  );
}
