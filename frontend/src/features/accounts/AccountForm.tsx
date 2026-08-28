import { useEffect, useMemo, useState } from "react";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { IconPicker } from "@/components/forms/IconPicker";
import { EntitySelect } from "@/components/forms/EntitySelect";
import { EntityIcon } from "@/components/icons/EntityIcon";
import { useMembers, useInstitutions, useGroups } from "@/queries/directory";
import { useSupportedCurrencies } from "@/queries/settings";
import { useCatalog } from "@/queries/catalog";
import { useBootstrap } from "@/queries/household";
import type { CreateAccountRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account/models";
import type { AccountRecordDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { displayEnum } from "@/lib/display";
import { ownershipShares, trackingMethodKey } from "@/features/accounts/accountCatalog";
import { ACCOUNT_TYPE_ICONS } from "@/lib/defaultIcons";

const accountFormSchema = z.object({
  name: z.string().trim().min(1),
  accountType: z.string(),
  balanceSheetRole: z.string(),
  trackingMode: z.string(),
  defaultCurrency: z.string().length(3),
  initialAmount: z.string().optional(),
  ownerIds: z.array(z.string()).min(1),
  ownershipPercentages: z.array(z.string()).optional(),
  institutionId: z.string().optional(),
  groupId: z.string().optional(),
  includeInNetWorth: z.boolean(),
  includeInPortfolio: z.boolean(),
  includeInLiquidAssets: z.boolean(),
});

export type AccountFormValues = z.infer<typeof accountFormSchema>;

export type AccountFormExtras = Record<string, never>;

const emptyValues: AccountFormValues = {
  name: "",
  accountType: "cash_on_hand", balanceSheetRole: "asset",
  trackingMode: "balance",
  defaultCurrency: "CNY",
  initialAmount: "",
  ownerIds: [],
  ownershipPercentages: [],
  institutionId: "",
  groupId: "",
  includeInNetWorth: true,
  includeInPortfolio: false,
  includeInLiquidAssets: false,
};

function valuesFromRecord(record: AccountRecordDTO): AccountFormValues {
  const ownerIds = (record.ownership ?? []).map((share) => share.memberId);
  const percentages = (record.ownership ?? []).map((share) => String(share.shareBps / 100));
  const equal = percentages.length <= 1 || percentages.every((value) => value === percentages[0]);
  return {
    name: record.account.name,
    accountType: record.account.accountType,
    balanceSheetRole: record.account.balanceSheetRole,
    trackingMode: record.account.trackingMode,
    defaultCurrency: record.account.defaultCurrency,
    initialAmount: "",
    ownerIds,
    ownershipPercentages: equal ? [] : percentages,
    institutionId: record.account.institutionId ?? "",
    groupId: record.account.groupId ?? "",
    includeInNetWorth: record.account.includeInNetWorth,
    includeInPortfolio: record.account.includeInPortfolio,
    includeInLiquidAssets: record.account.includeInLiquidAssets,
  };
}

/**
 * AccountForm implements Account creation and metadata edit. Minimal
 * required fields come first; institution, group, and icon sit behind
 * progressive disclosure so the keyboard-only create path remains compact.
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
  const catalog = useCatalog();
  const bootstrap = useBootstrap();
  const isEdit = Boolean(record);
  const [showMoreOptions, setShowMoreOptions] = useState(isEdit);
  const [iconCustomized, setIconCustomized] = useState(Boolean(record?.account.iconKey));
  const [iconKey, setIconKey] = useState(record?.account.iconKey ?? ACCOUNT_TYPE_ICONS.cash_on_hand);
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

  const accountType = useWatch({ control, name: "accountType" });
  const balanceSheetRole = useWatch({ control, name: "balanceSheetRole" });
  const trackingMode = useWatch({ control, name: "trackingMode" });
  const includeInPortfolio = useWatch({ control, name: "includeInPortfolio" });
  const ownerIds = useWatch({ control, name: "ownerIds" });
  const ownershipPercentages = useWatch({ control, name: "ownershipPercentages" }) ?? [];
  const institutionId = useWatch({ control, name: "institutionId" });
  const groupId = useWatch({ control, name: "groupId" });

  const combinations = useMemo(
    () => catalog.data?.accountCombinations ?? [],
    [catalog.data?.accountCombinations],
  );
  const frozenRole = record?.account.balanceSheetRole ?? balanceSheetRole;
  const frozenTracking = record?.account.trackingMode ?? trackingMode;
  const typeOptions = useMemo(() => {
    const catalogTypes = catalog.data?.accountTypes ?? [];
    if (!isEdit) {
      return catalogTypes;
    }
    const compatible = new Set(
      combinations
        .filter((item) => item.balanceSheetRole === frozenRole && item.trackingMode === frozenTracking)
        .map((item) => item.accountType),
    );
    if (accountType) {
      compatible.add(accountType);
    }
    return catalogTypes.filter((type) => compatible.has(type));
  }, [isEdit, catalog.data?.accountTypes, combinations, frozenRole, frozenTracking, accountType]);
  const matchingCombinations = useMemo(
    () => combinations.filter((item) => item.accountType === accountType),
    [combinations, accountType],
  );
  const trackingModeOptions = useMemo(
    () =>
      matchingCombinations
        .filter((item) => item.balanceSheetRole === balanceSheetRole)
        .map((item) => item.trackingMode),
    [matchingCombinations, balanceSheetRole],
  );
  const householdCurrency = bootstrap.data?.household?.baseCurrency;
  const currencyOptions = currencies.data ?? (householdCurrency ? [householdCurrency] : []);

  useEffect(() => {
    if (isEdit) {
      return;
    }
    const next = householdCurrency ?? currencies.data?.[0];
    if (next) {
      setValue("defaultCurrency", next);
    }
  }, [currencies.data, householdCurrency, isEdit, setValue]);

  const applyCombinationDefaults = (nextType: string, nextRole?: string, nextTracking?: string) => {
    const forType = combinations.filter((item) => item.accountType === nextType);
    const role = nextRole ?? forType[0]?.balanceSheetRole ?? "";
    const forRole = forType.filter((item) => item.balanceSheetRole === role);
    const tracking = nextTracking ?? forRole[0]?.trackingMode ?? "";
    const match = forRole.find((item) => item.trackingMode === tracking) ?? forRole[0];
    setValue("accountType", nextType);
    setValue("balanceSheetRole", role);
    setValue("trackingMode", tracking);
    if (!isEdit && !iconCustomized) {
      setIconKey(ACCOUNT_TYPE_ICONS[nextType] ?? "account");
    }
    if (!isEdit && match) {
      setValue("includeInNetWorth", match.includeInNetWorth, { shouldDirty: true, shouldTouch: true });
      setValue("includeInPortfolio", match.includeInPortfolio, { shouldDirty: true, shouldTouch: true });
      setValue("includeInLiquidAssets", match.includeInLiquidAssets, { shouldDirty: true, shouldTouch: true });
    }
  };

  const handleAccountTypeChange = (value: string) => {
    if (isEdit) {
      setValue("accountType", value);
      return;
    }
    applyCombinationDefaults(value);
  };

  const toggleOwner = (memberId: string) => {
    const next = ownerIds.includes(memberId) ? ownerIds.filter((id) => id !== memberId) : [...ownerIds, memberId];
    setValue("ownerIds", next);
  };

  const submit = (values: AccountFormValues) => {
    if (values.ownerIds.length === 0) {
      return;
    }
    const request: CreateAccountRequest = {
      name: values.name,
      accountType: values.accountType,
      balanceSheetRole: values.balanceSheetRole,
      trackingMode: values.trackingMode,
      defaultCurrency: values.defaultCurrency,
      includeInNetWorth: values.includeInNetWorth,
      includeInPortfolio: values.includeInPortfolio,
      includeInLiquidAssets: values.includeInLiquidAssets,
      ownership: ownershipShares(values.ownerIds, values.ownershipPercentages, useCustomPercentages),
      institutionId: values.institutionId || undefined,
      groupId: values.groupId || undefined,
      iconKey: iconKey || undefined,
      initialAmount: isEdit || values.trackingMode === "holdings" ? "" : values.initialAmount || "0",
    };
    onSubmit(request, {});
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
        <Label htmlFor="account-type">{t("accounts.accountType")}</Label>
        <NativeSelect
          id="account-type"
          value={accountType}
          onChange={(event) => handleAccountTypeChange(event.target.value)}
        >
          {typeOptions.map((type) => (
            <option key={type} value={type}>
              {displayEnum(t, "enum", type)}
            </option>
          ))}
        </NativeSelect>
      </div>

      <div className="flex flex-col gap-1.5">
        <Label id="account-role-label">{t("accounts.balanceSheetRole")}</Label>
        <p id="account-role" aria-labelledby="account-role-label" className="text-sm text-muted-foreground">
          {balanceSheetRole === "liability" ? t("accounts.liability") : t("accounts.asset")}
        </p>
        {isEdit && <p className="text-xs text-muted-foreground">{t("accounts.roleImmutable")}</p>}
      </div>

      {trackingModeOptions.length > 1 && !isEdit && (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="account-tracking-mode">{t("accounts.trackingMethod")}</Label>
          <NativeSelect
            id="account-tracking-mode"
            value={trackingMode}
            onChange={(event) => applyCombinationDefaults(accountType, balanceSheetRole, event.target.value)}
          >
            {trackingModeOptions.map((mode) => (
              <option key={mode} value={mode}>
                {t(trackingMethodKey(mode))}
              </option>
            ))}
          </NativeSelect>
        </div>
      )}
      {isEdit && (
        <div className="flex flex-col gap-1">
          <p className="text-sm text-muted-foreground">
            {t("accounts.trackingMethod")}: {t(trackingMethodKey(trackingMode))}
          </p>
          <p className="text-xs text-muted-foreground">{t("accounts.trackingImmutable")}</p>
        </div>
      )}

      <div className="flex flex-col gap-1.5">
        <Label htmlFor="account-currency">{t("accounts.currency")}</Label>
        <NativeSelect id="account-currency" {...register("defaultCurrency")}>
          {(currencyOptions).map((currency) => (
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
                <span className="flex items-center gap-2"><EntityIcon iconKey={member.iconKey} kind="member" />{member.name}</span>
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
          <input type="checkbox" {...register("includeInPortfolio")} /> {t("accounts.includeInPortfolio")}
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" {...register("includeInLiquidAssets")} /> {t("accounts.includeInLiquidAssets")}
        </label>
        {trackingMode === "holdings" && includeInPortfolio && (
          <p className="text-xs text-muted-foreground">{t("accounts.wholeAccountPortfolio")}</p>
        )}
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
          <EntitySelect id="account-institution" label={t("nav.institutions")} value={institutionId ?? ""} options={institutions.data ?? []} emptyLabel={t("accounts.none")} kind="institution" onChange={(value) => setValue("institutionId", value)} />
          <EntitySelect id="account-group" label={t("nav.groups")} value={groupId ?? ""} options={groups.data ?? []} emptyLabel={t("accounts.none")} kind="group" onChange={(value) => setValue("groupId", value)} />
          <IconPicker id="account-icon" value={iconKey} kind="account" onChange={(key) => { setIconKey(key); setIconCustomized(true); }} />
        </div>
      )}

      {submissionError && (
        <p role="alert" className="text-sm text-destructive">
          {submissionError}
        </p>
      )}

      <Button type="submit" disabled={isSubmitting || ownerIds.length === 0}>
        {submitLabel}
      </Button>
    </form>
  );
}
