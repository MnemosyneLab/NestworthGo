import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { IconPicker } from "@/components/forms/IconPicker";
import { useMembers, useInstitutions, useGroups, useCreateInstitution } from "@/queries/directory";
import { useSupportedCurrencies } from "@/queries/settings";
import { useCatalog } from "@/queries/catalog";
import { useBootstrap } from "@/queries/household";
import type { CreateAccountRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account/models";
import { displayEnum, displayError } from "@/lib/display";
import {
  defaultRole,
  matchingCombination,
  ownershipShares,
  roleLocked,
  rolesFor,
  splitAccountTypes,
  trackingMethodKey,
  trackingHelpKey,
  trackingModesFor,
  trackingPrompt,
} from "@/features/accounts/accountCatalog";
import { ACCOUNT_TYPE_ICONS } from "@/lib/defaultIcons";
import { EntityIcon } from "@/components/icons/EntityIcon";
import { EntitySelect } from "@/components/forms/EntitySelect";
import type { AccountFormExtras } from "@/features/accounts/AccountForm";

type WizardStep = "institution" | "type" | "tracking" | "details" | "review";

type DirectorySelection = {
  id: string;
  sortOrder: number;
  archivedAt?: string | null;
};

function lowestActiveDirectoryId(records: readonly DirectorySelection[] | null | undefined): string {
  return [...(records ?? [])]
    .filter((record) => !record.archivedAt)
    .sort((left, right) => (left.sortOrder ?? Number.MAX_SAFE_INTEGER) - (right.sortOrder ?? Number.MAX_SAFE_INTEGER) || left.id.localeCompare(right.id))[0]?.id ?? "";
}

/**
 * AccountCreateWizard is the real-world create path: institution, then account
 * type, then a tracking question only when the catalog has more than one
 * common recording method. Ordinary users never pick tracking-mode or
 * balance-sheet-role enums on the main path.
 */
export function AccountCreateWizard({
  onSubmit,
  isSubmitting,
  submissionError,
}: {
  onSubmit: (request: CreateAccountRequest, extras: AccountFormExtras) => void | Promise<void>;
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
  const createInstitution = useCreateInstitution();

  const combinations = catalog.data?.accountCombinations ?? [];
  const catalogTypes = catalog.data?.accountTypes ?? [];
  const { primary, more } = splitAccountTypes(catalogTypes);
  const householdCurrency = bootstrap.data?.household?.baseCurrency;
  const currencyOptions = currencies.data ?? (householdCurrency ? [householdCurrency] : []);

  const [step, setStep] = useState<WizardStep>("institution");
  const [institutionId, setInstitutionId] = useState("");
  const [showNewInstitution, setShowNewInstitution] = useState(false);
  const [newInstitutionName, setNewInstitutionName] = useState("");
  const [newInstitutionType, setNewInstitutionType] = useState("bank");
  const [institutionError, setInstitutionError] = useState<string | undefined>();
  const [accountType, setAccountType] = useState("");
  const [showMoreTypes, setShowMoreTypes] = useState(false);
  const [role, setRole] = useState("asset");
  const [trackingMode, setTrackingMode] = useState("balance");
  const [name, setName] = useState("");
  const [defaultCurrency, setDefaultCurrency] = useState("");
  const [initialAmount, setInitialAmount] = useState("");
  const [ownerIds, setOwnerIds] = useState<string[]>([]);
  const [ownershipPercentages, setOwnershipPercentages] = useState<string[]>([]);
  const [useCustomPercentages, setUseCustomPercentages] = useState(false);
  const [includeInNetWorth, setIncludeInNetWorth] = useState(true);
  const [includeInPortfolio, setIncludeInPortfolio] = useState(false);
  const [includeInLiquidAssets, setIncludeInLiquidAssets] = useState(false);
  const [groupId, setGroupId] = useState("");
  const [institutionTouched, setInstitutionTouched] = useState(false);
  const [groupTouched, setGroupTouched] = useState(false);
  const [iconKey, setIconKey] = useState("cash");
  const [iconCustomized, setIconCustomized] = useState(false);
  const [showMoreSettings, setShowMoreSettings] = useState(false);
  const [nameError, setNameError] = useState<string | undefined>();
  const [trackingError, setTrackingError] = useState<string | undefined>();

  const resolvedType = catalogTypes.includes(accountType) ? accountType : (primary[0] ?? catalogTypes[0] ?? "");
  const resolvedRole = accountType ? role : defaultRole(combinations, resolvedType);
  const resolvedPrompt = trackingPrompt(combinations, resolvedType, resolvedRole);
  const resolvedTracking = accountType ? trackingMode : resolvedPrompt.defaultMode;
  const currencyValue = defaultCurrency || householdCurrency || currencies.data?.[0] || "CNY";
  const typeLocked = roleLocked(combinations, resolvedType);
  const typeRoles = rolesFor(combinations, resolvedType);
  const prompt = accountType ? trackingPrompt(combinations, accountType, role) : resolvedPrompt;
  const defaultInstitutionId = useMemo(() => lowestActiveDirectoryId(institutions.data), [institutions.data]);
  const defaultGroupId = useMemo(() => lowestActiveDirectoryId(groups.data), [groups.data]);
  const selectedInstitutionId = institutionTouched ? institutionId : institutionId || defaultInstitutionId;
  const selectedGroupId = groupTouched ? groupId : groupId || defaultGroupId;

  const applyType = (nextType: string) => {
    const nextRole = defaultRole(combinations, nextType);
    const nextPrompt = trackingPrompt(combinations, nextType, nextRole);
    const match = matchingCombination(combinations, nextType, nextRole, nextPrompt.defaultMode) ?? combinations.find((item) => item.accountType === nextType);
    setAccountType(nextType);
    if (!iconCustomized) setIconKey(ACCOUNT_TYPE_ICONS[nextType] ?? "account");
    setRole(nextRole);
    setTrackingMode(nextPrompt.defaultMode);
    setTrackingError(undefined);
    if (match) {
      setIncludeInNetWorth(match.includeInNetWorth);
      setIncludeInPortfolio(match.includeInPortfolio);
      setIncludeInLiquidAssets(match.includeInLiquidAssets);
    }
  };

  const applyRole = (nextRole: string) => {
    const nextPrompt = trackingPrompt(combinations, accountType, nextRole);
    const match = matchingCombination(combinations, accountType, nextRole, nextPrompt.defaultMode);
    setRole(nextRole);
    setTrackingMode(nextPrompt.defaultMode);
    if (match) {
      setIncludeInNetWorth(match.includeInNetWorth);
      setIncludeInPortfolio(match.includeInPortfolio);
      setIncludeInLiquidAssets(match.includeInLiquidAssets);
    }
  };

  const applyTracking = (nextMode: string) => {
    const match = matchingCombination(combinations, accountType, role, nextMode);
    setTrackingMode(nextMode);
    setTrackingError(undefined);
    if (match) {
      setIncludeInNetWorth(match.includeInNetWorth);
      setIncludeInPortfolio(match.includeInPortfolio);
      setIncludeInLiquidAssets(match.includeInLiquidAssets);
    }
  };

  const institutionName = useMemo(
    () => (institutions.data ?? []).find((item) => item.id === selectedInstitutionId)?.name,
    [institutions.data, selectedInstitutionId],
  );

  const goNextFromInstitution = async () => {
    if (showNewInstitution) {
      const trimmed = newInstitutionName.trim();
      if (!trimmed) {
        setInstitutionError(t("directory.nameRequired"));
        return;
      }
      setInstitutionError(undefined);
      try {
        const created = await createInstitution.mutateAsync({ name: trimmed, institutionType: newInstitutionType, iconKey: "" });
        setInstitutionId(created.id);
        setShowNewInstitution(false);
        setNewInstitutionName("");
        setStep("type");
      } catch (error) {
        setInstitutionError(displayError(error, t("directory.createError")));
      }
      return;
    }
    setStep("type");
  };

  const goNextFromType = () => {
    if (!accountType && resolvedType) {
      applyType(resolvedType);
    }
    if (!typeLocked && !role) {
      return;
    }
    if (prompt.kind === "ask") {
      setStep("tracking");
      return;
    }
    setStep("details");
  };

  const goNextFromTracking = () => {
    if (!trackingMode) {
      setTrackingError(t("accounts.trackingQuestion"));
      return;
    }
    setStep("details");
  };

  const goNextFromDetails = () => {
    if (!name.trim()) {
      setNameError(t("error.validation.notEmpty"));
      return;
    }
    if (ownerIds.length === 0) {
      return;
    }
    setNameError(undefined);
    setStep("review");
  };

  const toggleOwner = (memberId: string) => {
    setOwnerIds((current) => (current.includes(memberId) ? current.filter((id) => id !== memberId) : [...current, memberId]));
  };

  const create = () => {
    if (ownerIds.length === 0) {
      return;
    }
    const request: CreateAccountRequest = {
      name: name.trim(),
      accountType: resolvedType,
      balanceSheetRole: resolvedRole,
      trackingMode: resolvedTracking,
      defaultCurrency: currencyValue,
      includeInNetWorth,
      includeInPortfolio,
      includeInLiquidAssets,
      ownership: ownershipShares(ownerIds, ownershipPercentages, useCustomPercentages),
      institutionId: selectedInstitutionId || undefined,
      groupId: selectedGroupId || undefined,
      iconKey: iconKey || undefined,
      initialAmount: resolvedTracking === "holdings" ? "" : initialAmount || "0",
    };
    void onSubmit(request, {});
  };

  const typeButton = (type: string) => {
    const selected = (accountType || resolvedType) === type;
    const typeRole = selected ? resolvedRole : defaultRole(combinations, type);
    const methods = trackingModesFor(combinations, type, typeRole)
      .map((mode) => t(trackingMethodKey(mode, type)))
      .join(t("accounts.trackingMethodSeparator"));
    return (
      <button
        key={type}
        type="button"
        className={`rounded-md border px-3 py-2 text-left text-sm ${selected ? "border-primary bg-primary/10 ring-1 ring-primary/30" : "border-border"}`}
        aria-label={displayEnum(t, "enum", type)}
        aria-pressed={selected}
        onClick={() => applyType(type)}
      >
        <span className="flex items-center gap-2 font-medium"><EntityIcon iconKey={ACCOUNT_TYPE_ICONS[type]} kind="account" />{displayEnum(t, "enum", type)}</span>
        <span className="mt-1 block text-xs text-muted-foreground">
          {t("accounts.supportedTrackingMethods", { methods })}
        </span>
      </button>
    );
  };

  return (
    <div className="flex flex-col gap-5" role="form" aria-label={t("accounts.wizardLabel")} data-testid={`account-wizard-${step}`}>
      {step === "institution" && (
        <div className="flex flex-col gap-4">
          <div>
            <h3 className="text-base font-semibold">{t("accounts.stepInstitution")}</h3>
            <p className="mt-1 text-sm text-muted-foreground">{t("accounts.institutionOptionalHint")}</p>
          </div>
          <div className="flex flex-col gap-2" role="listbox" aria-label={t("nav.institutions")}>
            <button
              type="button"
              className={`rounded-md border px-3 py-2 text-left text-sm ${selectedInstitutionId === "" && !showNewInstitution ? "border-primary bg-primary/10" : "border-border"}`}
              aria-selected={selectedInstitutionId === "" && !showNewInstitution}
              onClick={() => {
                setInstitutionTouched(true);
                setInstitutionId("");
                setShowNewInstitution(false);
              }}
            >
              {t("accounts.noInstitution")}
            </button>
            {(institutions.data ?? []).map((institution) => (
              <button
                key={institution.id}
                type="button"
                className={`rounded-md border px-3 py-2 text-left text-sm ${selectedInstitutionId === institution.id && !showNewInstitution ? "border-primary bg-primary/10" : "border-border"}`}
                aria-selected={selectedInstitutionId === institution.id && !showNewInstitution}
                onClick={() => {
                  setInstitutionTouched(true);
                  setInstitutionId(institution.id);
                  setShowNewInstitution(false);
                }}
              >
                <span className="flex items-center gap-2"><EntityIcon iconKey={institution.iconKey} kind="institution" />{institution.name}</span>
              </button>
            ))}
            <button
              type="button"
              className={`rounded-md border px-3 py-2 text-left text-sm ${showNewInstitution ? "border-primary bg-primary/10" : "border-border"}`}
              aria-pressed={showNewInstitution}
              onClick={() => {
                setInstitutionTouched(true);
                setShowNewInstitution(true);
              }}
            >
              {t("accounts.createInstitution")}
            </button>
          </div>
          {showNewInstitution && (
            <div className="flex flex-col gap-3">
              <div className="flex flex-col gap-1.5">
              <Label htmlFor="new-institution-name">{t("accounts.institutionName")}</Label>
              <Input
                id="new-institution-name"
                value={newInstitutionName}
                onChange={(event) => setNewInstitutionName(event.target.value)}
                autoFocus
              />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="new-institution-type">{t("directory.institutionType")}</Label>
                <NativeSelect id="new-institution-type" value={newInstitutionType} onChange={(event) => setNewInstitutionType(event.target.value)}>
                  {(catalog.data?.institutionTypes ?? []).map((type) => <option key={type} value={type}>{displayEnum(t, "institutionType", type)}</option>)}
                </NativeSelect>
              </div>
            </div>
          )}
          {institutionError && (
            <p role="alert" className="text-sm text-destructive">
              {institutionError}
            </p>
          )}
        </div>
      )}

      {step === "type" && (
        <div className="flex flex-col gap-4">
          <h3 className="text-base font-semibold">{t("accounts.stepType")}</h3>
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">{primary.map(typeButton)}</div>
          {more.length > 0 && (
            <>
              <Button type="button" variant="ghost" size="sm" className="self-start" onClick={() => setShowMoreTypes((value) => !value)}>
                {showMoreTypes ? t("accounts.hideMoreTypes") : t("accounts.moreTypes")}
              </Button>
              {showMoreTypes && <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">{more.map(typeButton)}</div>}
            </>
          )}
          {!typeLocked && typeRoles.length > 1 && (
            <fieldset className="flex flex-col gap-2">
              <legend className="text-sm font-medium">{t("accounts.roleQuestion")}</legend>
              {typeRoles.map((option) => (
                <label key={option} className="flex items-center gap-2 text-sm">
                  <input type="radio" name="account-role" checked={role === option} onChange={() => applyRole(option)} />
                  {option === "liability" ? t("accounts.liability") : t("accounts.asset")}
                </label>
              ))}
            </fieldset>
          )}
        </div>
      )}

      {step === "tracking" && (
        <div className="flex flex-col gap-4">
          <div>
            <h3 className="text-base font-semibold">{t("accounts.trackingQuestion")}</h3>
            <p className="mt-1 text-sm text-muted-foreground">{displayEnum(t, "enum", accountType)}</p>
          </div>
          <div className="flex flex-col gap-3" role="radiogroup" aria-label={t("accounts.trackingQuestion")}>
            {prompt.options.map((mode) => (
              <label key={mode} className="flex cursor-pointer flex-col gap-1 rounded-md border border-border p-3 has-[:checked]:border-primary has-[:checked]:bg-primary/10">
                <span className="flex items-center gap-2 text-sm font-medium">
                  <input type="radio" name="account-tracking" checked={trackingMode === mode} onChange={() => applyTracking(mode)} />
                  {t(trackingMethodKey(mode, accountType))}
                  {mode === prompt.defaultMode && (
                    <span className="rounded-full bg-muted px-2 py-0.5 text-xs font-normal text-muted-foreground">
                      {t("accounts.recommended")}
                    </span>
                  )}
                </span>
                <span className="pl-6 text-xs text-muted-foreground">
                  {t(trackingHelpKey(mode, accountType))}
                </span>
              </label>
            ))}
          </div>
          {trackingError && (
            <p role="alert" className="text-sm text-destructive">
              {trackingError}
            </p>
          )}
        </div>
      )}

      {step === "details" && (
        <div className="flex flex-col gap-4">
          <h3 className="text-base font-semibold">{t("accounts.stepDetails")}</h3>
          <p className="text-sm text-muted-foreground">
            {institutionName || t("accounts.noInstitution")} · {displayEnum(t, "enum", accountType)}
          </p>
          <p className="text-sm">
            <span className="text-muted-foreground">
              {t("accounts.selectedTrackingMethod", { method: t(trackingMethodKey(resolvedTracking, resolvedType)) })}
            </span>
          </p>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="account-name">{t("accounts.name")}</Label>
            <Input id="account-name" value={name} onChange={(event) => setName(event.target.value)} autoFocus />
            {nameError && (
              <p role="alert" className="text-xs text-destructive">
                {nameError}
              </p>
            )}
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="account-currency">{t("accounts.currency")}</Label>
            <NativeSelect id="account-currency" value={currencyValue} onChange={(event) => setDefaultCurrency(event.target.value)}>
              {currencyOptions.map((currency) => (
                <option key={currency} value={currency}>
                  {currency}
                </option>
              ))}
            </NativeSelect>
          </div>
          {resolvedTracking !== "holdings" && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="account-initial-amount">{t("accounts.amount")}</Label>
              <Input id="account-initial-amount" inputMode="decimal" value={initialAmount} onChange={(event) => setInitialAmount(event.target.value)} placeholder="0" />
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
                  <input type="checkbox" id={`wizard-owner-${member.id}`} checked={checked} onChange={() => toggleOwner(member.id)} className="size-4" />
                  <Label htmlFor={`wizard-owner-${member.id}`} className="flex-1 font-normal">
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
                        setOwnershipPercentages(next);
                      }}
                      placeholder="%"
                    />
                  )}
                </div>
              );
            })}
            {ownerIds.length > 1 && (
              <label className="flex items-center gap-2 text-xs text-muted-foreground">
                <input type="checkbox" checked={useCustomPercentages} onChange={(event) => setUseCustomPercentages(event.target.checked)} />
                {t("accounts.ownershipSharePlaceholder")}
              </label>
            )}
          </fieldset>
          <Button type="button" variant="ghost" size="sm" className="self-start" onClick={() => setShowMoreSettings((value) => !value)} aria-expanded={showMoreSettings}>
            {t("accounts.moreSettings")}
          </Button>
          {showMoreSettings && (
            <div className="flex flex-col gap-3 rounded-md border border-border p-3">
              <label className="flex items-center gap-2 text-sm">
                <input type="checkbox" checked={includeInNetWorth} onChange={(event) => setIncludeInNetWorth(event.target.checked)} /> {t("accounts.includeInNetWorth")}
              </label>
              <EntitySelect id="wizard-group" label={t("nav.groups")} value={selectedGroupId} options={groups.data ?? []} emptyLabel={t("accounts.none")} kind="group" onChange={(value) => { setGroupTouched(true); setGroupId(value); }} />
              <IconPicker id="wizard-icon" value={iconKey} kind="account" onChange={(key) => { setIconKey(key); setIconCustomized(true); }} />
            </div>
          )}
        </div>
      )}

      {step === "review" && (
        <div className="flex flex-col gap-4">
          <h3 className="text-base font-semibold">{t("accounts.stepReview")}</h3>
          <div className="rounded-md border border-border p-3 text-sm">
            <p className="font-medium">
              {institutionName ? `${institutionName} · ${displayEnum(t, "enum", accountType)}` : displayEnum(t, "enum", accountType)}
            </p>
            <p className="mt-1">{name}</p>
            <p className="mt-2">{t(trackingMethodKey(resolvedTracking, resolvedType))}</p>
            <p className="text-muted-foreground">{t("accounts.reviewTrackingImmutable")}</p>
          </div>
          {submissionError && (
            <p role="alert" className="text-sm text-destructive">
              {submissionError}
            </p>
          )}
        </div>
      )}

      <div className="flex gap-2">
        {step !== "institution" && (
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              if (step === "type") setStep("institution");
              else if (step === "tracking") setStep("type");
              else if (step === "details") setStep(prompt.kind === "ask" ? "tracking" : "type");
              else setStep("details");
            }}
            disabled={isSubmitting}
          >
            {t("accounts.back")}
          </Button>
        )}
        <Button
          type="button"
          onClick={
            step === "review"
              ? create
              : step === "institution"
                ? () => void goNextFromInstitution()
                : step === "type"
                  ? goNextFromType
                  : step === "tracking"
                    ? goNextFromTracking
                    : goNextFromDetails
          }
          disabled={isSubmitting || createInstitution.isPending || ((step === "details" || step === "review") && ownerIds.length === 0)}
          autoFocus={step !== "details"}
        >
          {isSubmitting || createInstitution.isPending ? t("common.pending") : step === "review" ? t("accounts.create") : t("accounts.continue")}
        </Button>
      </div>
    </div>
  );
}
