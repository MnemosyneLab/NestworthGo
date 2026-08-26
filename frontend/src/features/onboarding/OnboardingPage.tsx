import { useFieldArray, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import { Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useCompleteOnboarding } from "@/queries/household";
import { useSupportedCurrencies } from "@/queries/settings";
import { translateWailsError, type WireError } from "@/lib/wails";

const onboardingSchema = z.object({
  householdName: z.string().trim().min(1),
  baseCurrency: z.string().length(3),
  memberNames: z
    .array(z.object({ name: z.string().trim().min(1) }))
    .min(1),
});

type OnboardingFormValues = z.infer<typeof onboardingSchema>;

/**
 * OnboardingPage implements the "Household creation, base currency, first
 * Members" flow end to end through HouseholdService. It intentionally does
 * not ask for a timezone here: leaving `CompleteOnboardingRequest.timezone`
 * empty means history has not started yet; Starting Point is a separate flow.
 */
export function OnboardingPage() {
  const { t } = useTranslation();
  const currencies = useSupportedCurrencies();
  const completeOnboarding = useCompleteOnboarding();

  const {
    register,
    control,
    handleSubmit,
    formState: { errors },
  } = useForm<OnboardingFormValues>({
    resolver: zodResolver(onboardingSchema),
    defaultValues: { householdName: "", baseCurrency: "USD", memberNames: [{ name: "" }] },
  });
  const { fields, append, remove } = useFieldArray({ control, name: "memberNames" });

  const onSubmit = (values: OnboardingFormValues) => {
    completeOnboarding.mutate({
      householdName: values.householdName,
      baseCurrency: values.baseCurrency,
      memberNames: values.memberNames.map((member) => member.name),
    });
  };

  return (
    <div className="mx-auto flex max-w-md flex-col gap-6 py-12">
      <div className="flex flex-col gap-1 text-center">
        <h1 className="text-xl font-semibold text-foreground">{t("onboarding.title", { defaultValue: t("app.name") })}</h1>
      </div>
      <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4" aria-label="Onboarding">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="householdName">{t("onboarding.householdName", { defaultValue: "Household name" })}</Label>
          <Input id="householdName" {...register("householdName")} autoFocus />
          {errors.householdName && (
            <p role="alert" className="text-xs text-destructive">
              {t("error.validation.notEmpty")}
            </p>
          )}
        </div>

        <div className="flex flex-col gap-1.5">
          <Label htmlFor="baseCurrency">{t("settings.household.baseCurrency", { defaultValue: "Base currency" })}</Label>
          <select
            id="baseCurrency"
            {...register("baseCurrency")}
            className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground"
          >
            {(currencies.data ?? ["USD"]).map((currency) => (
              <option key={currency} value={currency}>
                {currency}
              </option>
            ))}
          </select>
        </div>

        <div className="flex flex-col gap-2">
          <Label>{t("onboarding.members", { defaultValue: "Members" })}</Label>
          {fields.map((field, index) => (
            <div key={field.id} className="flex items-center gap-2">
              <Input
                aria-label={`Member ${index + 1} name`}
                {...register(`memberNames.${index}.name` as const)}
                placeholder={t("onboarding.memberNamePlaceholder", { defaultValue: "Name" })}
              />
              {fields.length > 1 && (
                <Button type="button" variant="ghost" size="icon" onClick={() => remove(index)} aria-label="Remove member">
                  <Trash2 className="size-4" />
                </Button>
              )}
            </div>
          ))}
          {errors.memberNames && (
            <p role="alert" className="text-xs text-destructive">
              {t("error.onboarding.memberRequired")}
            </p>
          )}
          <Button type="button" variant="outline" size="sm" onClick={() => append({ name: "" })} className="self-start">
            <Plus className="size-4" /> {t("common.add")}
          </Button>
        </div>

        {completeOnboarding.isError && (
          <p role="alert" className="text-sm text-destructive">
            {translateWailsError((completeOnboarding.error as Error & { wireError?: WireError }).wireError ?? { code: "internal", message: "" })}
          </p>
        )}

        <Button type="submit" disabled={completeOnboarding.isPending}>
          {t("onboarding.complete", { defaultValue: "Get started" })}
        </Button>
      </form>
    </div>
  );
}
