import { useFieldArray, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import { Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { BrandLockup } from "@/components/brand/BrandLockup";
import { useCompleteOnboarding } from "@/queries/household";
import { useSupportedCurrencies } from "@/queries/settings";
import { translateWailsError, type WireError } from "@/lib/wails";

const onboardingSchema = z.object({
  householdName: z.string().trim().min(1),
  baseCurrency: z.string().length(3),
  memberNames: z
    .array(z.object({ name: z.string() }))
    .superRefine((rows, ctx) => {
      if (rows.every((row) => row.name.trim() === "")) {
        ctx.addIssue({ code: "custom", path: [] });
      }
    }),
});

type OnboardingFormValues = z.infer<typeof onboardingSchema>;

/**
 * OnboardingPage implements the "Household creation, base currency, first
 * Members" flow end to end through HouseholdService. It intentionally does
 * not ask for a timezone here: leaving `CompleteOnboardingRequest.timezone`
 * empty means history has not started yet; Starting Point is a separate flow.
 */
export function OnboardingPage({ onCompleted }: { onCompleted?: () => void } = {}) {
  const { t } = useTranslation();
  const currencies = useSupportedCurrencies();
  const completeOnboarding = useCompleteOnboarding();

  const {
    register,
    control,
    getValues,
    handleSubmit,
    formState: { errors },
  } = useForm<OnboardingFormValues>({
    resolver: zodResolver(onboardingSchema),
    defaultValues: { householdName: "", baseCurrency: "CNY", memberNames: [{ name: "" }] },
  });
  const { fields, append, remove } = useFieldArray({ control, name: "memberNames" });

  const onSubmit = (values: OnboardingFormValues) => {
    completeOnboarding.mutate(
      {
        householdName: values.householdName,
        baseCurrency: values.baseCurrency,
        memberNames: values.memberNames.map((member) => member.name.trim()).filter(Boolean),
      },
      { onSuccess: onCompleted },
    );
  };

  return (
    <main className="min-h-[100dvh] bg-background px-6 py-10 text-foreground sm:px-10 sm:py-16">
      <div className="mx-auto grid w-full max-w-5xl gap-8 lg:grid-cols-[minmax(0,0.8fr)_minmax(360px,1fr)] lg:items-start">
        <section className="flex flex-col gap-5 pt-2 lg:pt-12">
          <BrandLockup />
          <div className="flex flex-col gap-3">
            <p className="text-sm font-medium text-primary">{t("onboarding.duration")}</p>
            <h1 className="max-w-lg text-3xl font-semibold tracking-tight sm:text-4xl">{t("onboarding.title")}</h1>
            <p className="max-w-xl text-base leading-7 text-muted-foreground">{t("onboarding.valueProposition")}</p>
          </div>
          <div className="flex max-w-xl flex-col gap-3 rounded-lg border border-border bg-card/70 p-4 text-sm leading-6 text-muted-foreground">
            <p>{t("onboarding.privacyNote")}</p>
            <p>{t("onboarding.nextStep")}</p>
          </div>
        </section>

        <Card className="w-full shadow-md">
          <CardHeader>
            <CardTitle>{t("onboarding.title")}</CardTitle>
            <CardDescription>{t("onboarding.description")}</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-5" aria-label={t("onboarding.formLabel")}>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="householdName">{t("onboarding.householdName")}</Label>
                <Input id="householdName" {...register("householdName")} autoFocus />
                <p className="text-xs leading-5 text-muted-foreground">{t("onboarding.householdHint")}</p>
                {errors.householdName && (
                  <p role="alert" className="text-xs text-destructive">
                    {t("error.validation.notEmpty")}
                  </p>
                )}
              </div>

              <div className="flex flex-col gap-1.5">
                <Label htmlFor="baseCurrency">{t("settings.household.baseCurrency")}</Label>
                <NativeSelect
                  id="baseCurrency"
                  {...register("baseCurrency")}
                >
                  {(currencies.data ?? []).map((currency) => (
                    <option key={currency} value={currency}>
                      {currency}
                    </option>
                  ))}
                </NativeSelect>
                <p className="text-xs leading-5 text-muted-foreground">{t("onboarding.baseCurrencyHint")}</p>
              </div>

              <fieldset className="flex flex-col gap-2">
                <legend className="text-sm font-medium">{t("onboarding.members")}</legend>
                {fields.map((field, index) => (
                  <div key={field.id} className="flex items-center gap-2">
                    <Input
                      aria-label={t("onboarding.memberNameLabel", { index: index + 1 })}
                      {...register(`memberNames.${index}.name` as const)}
                      placeholder={t("onboarding.memberNamePlaceholder")}
                    />
                    {fields.length > 1 && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        onClick={() => remove(index)}
                        aria-label={t("onboarding.removeMember", { index: index + 1 })}
                        title={t("onboarding.removeMember", { index: index + 1 })}
                      >
                        <Trash2 className="size-4" aria-hidden="true" />
                      </Button>
                    )}
                  </div>
                ))}
                <p className="text-xs leading-5 text-muted-foreground">{t("onboarding.memberHint")}</p>
                {errors.memberNames && (
                  <p role="alert" className="text-xs text-destructive">
                    {t("error.onboarding.memberRequired")}
                  </p>
                )}
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    const names = getValues("memberNames");
                    const last = names[names.length - 1];
                    if (last && !last.name.trim()) {
                      return;
                    }
                    append({ name: "" });
                  }}
                  className="self-start"
                >
                  <Plus className="size-4" aria-hidden="true" /> {t("onboarding.addMember")}
                </Button>
              </fieldset>

              {currencies.isError && <p className="text-xs text-warning-foreground">{t("onboarding.currencyLoadError")}</p>}

              {completeOnboarding.isError && (
                <p role="alert" className="text-sm text-destructive">
                  {translateWailsError((completeOnboarding.error as Error & { wireError?: WireError }).wireError ?? { code: "internal", message: "" })}
                </p>
              )}

              <Button type="submit" disabled={completeOnboarding.isPending}>
                {completeOnboarding.isPending ? t("common.pending") : t("onboarding.complete")}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </main>
  );
}
