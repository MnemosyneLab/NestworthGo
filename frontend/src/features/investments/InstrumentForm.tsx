import { useEffect } from "react";
import { useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { useSupportedCurrencies } from "@/queries/settings";
import { useCatalog } from "@/queries/catalog";
import { useBootstrap } from "@/queries/household";
import type { InstrumentRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument/models";
import { displayEnum } from "@/lib/display";

const instrumentFormSchema = z.object({
  name: z.string().trim().min(1),
  type: z.string(),
  quoteCurrency: z.string().length(3),
  quoteSource: z.string(),
  providerKey: z.string().optional(),
  providerSymbol: z.string().optional(),
});

type InstrumentFormValues = z.infer<typeof instrumentFormSchema>;

/** InstrumentForm creates an Instrument.
 * Provider-bound instruments require a provider key/symbol per
 * domain.NewInstrument's validation; this form only collects them when
 * quoteSource is "provider," matching the backend's rule. */
export function InstrumentForm({ onSubmit, isSubmitting, submissionError }: { onSubmit: (request: InstrumentRequest) => void; isSubmitting: boolean; submissionError?: string }) {
  const { t } = useTranslation();
  const currencies = useSupportedCurrencies();
  const catalog = useCatalog();
  const bootstrap = useBootstrap();
  const {
    register,
    control,
    setValue,
    handleSubmit,
    formState: { errors },
  } = useForm<InstrumentFormValues>({
    resolver: zodResolver(instrumentFormSchema),
    defaultValues: { name: "", type: "stock", quoteCurrency: "CNY", quoteSource: "manual" },
  });
  const quoteSource = useWatch({ control, name: "quoteSource" });
  const householdCurrency = bootstrap.data?.household?.baseCurrency;
  const currencyOptions = currencies.data ?? (householdCurrency ? [householdCurrency] : []);
  const instrumentTypes = catalog.data?.instrumentTypes ?? [];
  const quoteSources = catalog.data?.quoteSources ?? [];

  useEffect(() => {
    const next = householdCurrency ?? currencies.data?.[0];
    if (next) {
      setValue("quoteCurrency", next);
    }
  }, [currencies.data, householdCurrency, setValue]);

  const submit = (values: InstrumentFormValues) => {
    onSubmit({
      name: values.name,
      type: values.type,
      quoteCurrency: values.quoteCurrency,
      quoteSource: values.quoteSource,
      providerKey: values.quoteSource === "provider" ? values.providerKey : undefined,
      providerSymbol: values.quoteSource === "provider" ? values.providerSymbol : undefined,
    });
  };

  return (
    <form onSubmit={handleSubmit(submit)} className="flex flex-col gap-4" aria-label={t("portfolio.instrumentFormLabel")}>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="instrument-name">{t("accounts.name")}</Label>
        <Input id="instrument-name" {...register("name")} />
        {errors.name && (
          <p role="alert" className="text-xs text-destructive">
            {t("error.validation.notEmpty")}
          </p>
        )}
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="instrument-type">{t("portfolio.type")}</Label>
        <NativeSelect id="instrument-type" {...register("type")}>
          {instrumentTypes.map((type) => (
            <option key={type} value={type}>
              {displayEnum(t, "enum", type)}
            </option>
          ))}
        </NativeSelect>
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="instrument-currency">{t("accounts.currency")}</Label>
        <NativeSelect id="instrument-currency" {...register("quoteCurrency")}>
          {currencyOptions.map((currency) => (
            <option key={currency} value={currency}>
              {currency}
            </option>
          ))}
        </NativeSelect>
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="instrument-quote-source">{t("portfolio.quoteSource")}</Label>
        <NativeSelect id="instrument-quote-source" {...register("quoteSource")}>
          {quoteSources.map((source) => (
            <option key={source} value={source}>
              {displayEnum(t, "portfolio", source)}
            </option>
          ))}
        </NativeSelect>
      </div>
      {quoteSource === "provider" && (
        <>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="instrument-provider-key">{t("portfolio.providerKey")}</Label>
            <Input id="instrument-provider-key" {...register("providerKey")} placeholder={t("portfolio.providerKeyPlaceholder")} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="instrument-provider-symbol">{t("portfolio.providerSymbol")}</Label>
            <Input id="instrument-provider-symbol" {...register("providerSymbol")} placeholder={t("portfolio.providerSymbolPlaceholder")} />
          </div>
        </>
      )}
      {submissionError && (
        <p role="alert" className="text-sm text-destructive">
          {submissionError}
        </p>
      )}
      <Button type="submit" disabled={isSubmitting}>
        {isSubmitting ? t("common.pending") : t("portfolio.addInstrument")}
      </Button>
    </form>
  );
}
