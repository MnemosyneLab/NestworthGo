import { useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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

const defaultValues: AccountFormValues = {
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

/**
 * AccountForm implements Account creation "with minimal required fields
 * first, then progressive disclosure of institution/group/ownership"
 * (implementation plan Phase 4, citing interaction brief Sec8.2). Icon/
 * logo selection is deferred: it needs the native file-picker flow
 * (media.Service.PickImage, wired in Phase 2) integrated into a page,
 * which is Phase 5 scope alongside the other Directory pages that share
 * the same picker.
 */
export function AccountForm({
  onSubmit,
  submitLabel,
  isSubmitting,
}: {
  onSubmit: (request: CreateAccountRequest) => void;
  submitLabel: string;
  isSubmitting: boolean;
}) {
  const { t } = useTranslation();
  const members = useMembers();
  const institutions = useInstitutions();
  const groups = useGroups();
  const currencies = useSupportedCurrencies();
  const [showMoreOptions, setShowMoreOptions] = useState(false);
  const [useCustomPercentages, setUseCustomPercentages] = useState(false);

  const {
    register,
    watch,
    setValue,
    handleSubmit,
    formState: { errors },
  } = useForm<AccountFormValues>({ resolver: zodResolver(accountFormSchema), defaultValues });

  const primaryCategory = watch("primaryCategory") as PrimaryCategory;
  const trackingMode = watch("trackingMode");
  const ownerIds = watch("ownerIds");
  const ownershipPercentages = watch("ownershipPercentages") ?? [];

  const secondaryOptions = useMemo(() => SECONDARY_CATEGORIES_BY_PRIMARY[primaryCategory] ?? [], [primaryCategory]);
  const trackingModeOptions = useMemo(() => TRACKING_MODES_BY_PRIMARY[primaryCategory] ?? [], [primaryCategory]);

  const handlePrimaryCategoryChange = (value: PrimaryCategory) => {
    setValue("primaryCategory", value);
    setValue("secondaryCategory", defaultSecondaryCategory(value));
    setValue("trackingMode", defaultTrackingMode(value));
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
      ownerIds: values.ownerIds,
      ownershipPercentages: useCustomPercentages ? values.ownershipPercentages : undefined,
      institutionId: values.institutionId || undefined,
      groupId: values.groupId || undefined,
      initialAmount: values.trackingMode === "holdings" ? "" : values.initialAmount || "0",
    };
    onSubmit(request);
  };

  return (
    <form onSubmit={handleSubmit(submit)} className="flex flex-col gap-4" aria-label="Account form">
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
        <select
          id="account-primary-category"
          value={primaryCategory}
          onChange={(event) => handlePrimaryCategoryChange(event.target.value as PrimaryCategory)}
          className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground"
        >
          {PRIMARY_CATEGORIES.map((category) => (
            <option key={category} value={category}>
              {category}
            </option>
          ))}
        </select>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="account-secondary-category">{t("accounts.secondaryCategory")}</Label>
        <select id="account-secondary-category" {...register("secondaryCategory")} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
          {secondaryOptions.map((category) => (
            <option key={category} value={category}>
              {category}
            </option>
          ))}
        </select>
      </div>

      {trackingModeOptions.length > 1 && (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="account-tracking-mode">{t("accounts.trackingMode")}</Label>
          <select id="account-tracking-mode" {...register("trackingMode")} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
            {trackingModeOptions.map((mode) => (
              <option key={mode} value={mode}>
                {mode}
              </option>
            ))}
          </select>
        </div>
      )}

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="account-currency">{t("accounts.currency")}</Label>
        <select id="account-currency" {...register("defaultCurrency")} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
          {(currencies.data ?? ["USD"]).map((currency) => (
            <option key={currency} value={currency}>
              {currency}
            </option>
          ))}
        </select>
      </div>

      {trackingMode !== "holdings" && (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="account-initial-amount">{t("accounts.amount")}</Label>
          <Input id="account-initial-amount" inputMode="decimal" {...register("initialAmount")} placeholder="0" />
        </div>
      )}

      <div className="flex flex-col gap-2">
        <Label>{t("accounts.owner")}</Label>
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
                  aria-label={`${member.name} ownership percentage`}
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
      </div>

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

      <Button type="button" variant="ghost" size="sm" className="self-start" onClick={() => setShowMoreOptions((value) => !value)}>
        {showMoreOptions ? "− " : "+ "}
        {t("common.details")}
      </Button>
      {showMoreOptions && (
        <div className="flex flex-col gap-3 rounded-md border border-border p-3">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="account-institution">Institution</Label>
            <select id="account-institution" {...register("institutionId")} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
              <option value="">{t("accounts.none")}</option>
              {(institutions.data ?? []).map((institution) => (
                <option key={institution.id} value={institution.id}>
                  {institution.name}
                </option>
              ))}
            </select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="account-group">Group</Label>
            <select id="account-group" {...register("groupId")} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
              <option value="">{t("accounts.none")}</option>
              {(groups.data ?? []).map((group) => (
                <option key={group.id} value={group.id}>
                  {group.name}
                </option>
              ))}
            </select>
          </div>
        </div>
      )}

      <Button type="submit" disabled={isSubmitting}>
        {submitLabel}
      </Button>
    </form>
  );
}
