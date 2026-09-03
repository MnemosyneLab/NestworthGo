import { useEffect, useMemo, useState } from "react";
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
import type { InstrumentDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import { displayEnum } from "@/lib/display";
import { IconPicker } from "@/components/forms/IconPicker";
import { INSTRUMENT_TYPE_ICONS } from "@/lib/defaultIcons";
import { countryForMarket, marketsForCountry } from "@/lib/instrumentMarkets";

const instrumentFormSchema = z.object({
  name: z.string().trim().min(1),
  type: z.string(),
  quoteCurrency: z.string().length(3),
  symbol: z.string().optional(),
  marketCode: z.string().optional(),
  countryCode: z.string().optional(),
  isin: z.string().optional(),
  note: z.string().optional(),
  quoteSource: z.string(),
  providerKey: z.string().optional(),
  providerSymbol: z.string().optional(),
  iconKey: z.string(),
});

type InstrumentFormValues = z.infer<typeof instrumentFormSchema>;

function marketLabel(code: string, t: (key: string, options?: Record<string, unknown>) => string): string {
  const name = displayEnum(t, "portfolio.market", code);
  return name ? `${code} - ${name}` : code;
}

/** InstrumentForm creates or replaces an Instrument.
 * Provider-bound instruments require a provider key/symbol per
 * domain.NewInstrument's validation; this form only collects them when
 * quoteSource is "provider," matching the backend's rule. */
export function InstrumentForm({ instrument, onSubmit, isSubmitting, submissionError, submitLabel }: { instrument?: InstrumentDTO; onSubmit: (request: InstrumentRequest) => void; isSubmitting: boolean; submissionError?: string; submitLabel?: string }) {
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
    defaultValues: {
      name: instrument?.name ?? "",
      type: instrument?.type ?? "stock",
      quoteCurrency: instrument?.quoteCurrency ?? "CNY",
      symbol: instrument?.symbol ?? "",
      marketCode: instrument?.marketCode ?? "",
      countryCode: instrument?.countryCode ?? "",
      isin: instrument?.isin ?? "",
      note: instrument?.note ?? "",
      quoteSource: instrument?.quoteSource ?? "manual",
      providerKey: instrument?.providerKey ?? "",
      providerSymbol: instrument?.providerSymbol ?? "",
      iconKey: instrument?.iconKey ?? "stock",
    },
  });
  const quoteSource = useWatch({ control, name: "quoteSource" });
  const instrumentType = useWatch({ control, name: "type" });
  const iconKey = useWatch({ control, name: "iconKey" });
  const countryCode = useWatch({ control, name: "countryCode" }) ?? "";
  const marketCode = useWatch({ control, name: "marketCode" }) ?? "";
  const [iconCustomized, setIconCustomized] = useState(Boolean(instrument));
  const householdCurrency = bootstrap.data?.household?.baseCurrency;
  const currencyOptions = currencies.data ?? (householdCurrency ? [householdCurrency] : []);
  const instrumentTypes = catalog.data?.instrumentTypes ?? [];
  const quoteSources = catalog.data?.quoteSources ?? [];
  const instrumentProviders = catalog.data?.instrumentProviders ?? [];
  const allMarketCodes = useMemo(
    () => Array.from(new Set([...(catalog.data?.instrumentMarketCodes ?? []), ...(instrument?.marketCode ? [instrument.marketCode] : [])])),
    [catalog.data?.instrumentMarketCodes, instrument?.marketCode],
  );
  const countryOptions = Array.from(new Set([...(catalog.data?.instrumentCountryCodes ?? []), ...(instrument?.countryCode ? [instrument.countryCode] : [])]));
  const marketOptions = marketsForCountry(allMarketCodes, countryCode, marketCode);
  const defaultProviderKey = catalog.data?.instrumentProviders?.[0];

  useEffect(() => {
    if (instrument) return;
    const next = householdCurrency ?? currencies.data?.[0];
    if (next) {
      setValue("quoteCurrency", next);
    }
  }, [currencies.data, householdCurrency, instrument, setValue]);

  useEffect(() => {
    if (instrument) return;
    if (quoteSource === "provider" && defaultProviderKey) {
      setValue("providerKey", defaultProviderKey);
    }
  }, [defaultProviderKey, instrument, quoteSource, setValue]);

  useEffect(() => {
    if (!iconCustomized) setValue("iconKey", INSTRUMENT_TYPE_ICONS[instrumentType] ?? "investment");
  }, [iconCustomized, instrumentType, setValue]);

  const submit = (values: InstrumentFormValues) => {
    onSubmit({
      replace: Boolean(instrument),
      name: values.name,
      type: values.type,
      quoteCurrency: values.quoteCurrency,
      symbol: values.symbol?.trim() || undefined,
      marketCode: values.marketCode?.trim() || undefined,
      countryCode: values.countryCode?.trim().toUpperCase() || undefined,
      isin: values.isin?.trim() || undefined,
      note: values.note?.trim() || undefined,
      quoteSource: values.quoteSource,
      providerKey: values.quoteSource === "provider" ? values.providerKey : undefined,
      providerSymbol: values.quoteSource === "provider" ? values.providerSymbol : undefined,
      iconKey: values.iconKey,
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
      <IconPicker id="instrument-icon" value={iconKey} kind="instrument" onChange={(key) => { setValue("iconKey", key); setIconCustomized(true); }} />
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="instrument-country-code">{t("portfolio.countryCode")}</Label>
          <NativeSelect
            id="instrument-country-code"
            value={countryCode}
            onChange={(event) => {
              const next = event.target.value;
              setValue("countryCode", next);
              if (marketCode && next && countryForMarket(marketCode) !== next) {
                setValue("marketCode", "");
              }
            }}
          >
            <option value="">{t("common.selectOption")}</option>
            {countryOptions.map((country) => (
              <option key={country} value={country}>{displayEnum(t, "portfolio.countryRegion", country)} · {country}</option>
            ))}
          </NativeSelect>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="instrument-market-code">{t("portfolio.marketCode")}</Label>
          <NativeSelect
            id="instrument-market-code"
            value={marketCode}
            onChange={(event) => {
              const next = event.target.value;
              setValue("marketCode", next);
              const country = countryForMarket(next);
              if (country) {
                setValue("countryCode", country);
              }
            }}
          >
            <option value="">{t("common.selectOption")}</option>
            {marketOptions.map((market) => <option key={market} value={market}>{marketLabel(market, t)}</option>)}
          </NativeSelect>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="instrument-symbol">{t("portfolio.symbol")}</Label>
          <Input id="instrument-symbol" {...register("symbol")} />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="instrument-isin">{t("portfolio.isin")}</Label>
          <Input id="instrument-isin" {...register("isin")} />
        </div>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
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
      </div>
      {quoteSource === "provider" && (
        <>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="instrument-provider-key">{t("portfolio.providerKey")}</Label>
            <NativeSelect id="instrument-provider-key" {...register("providerKey")}>
              {instrumentProviders.map((provider) => (
                <option key={provider} value={provider}>
                  {displayEnum(t, "settings.provider", provider)}
                </option>
              ))}
            </NativeSelect>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="instrument-provider-symbol">{t("portfolio.providerSymbol")}</Label>
            <Input id="instrument-provider-symbol" {...register("providerSymbol")} placeholder={t("portfolio.providerSymbolPlaceholder")} />
          </div>
        </>
      )}
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="instrument-note">{t("portfolio.note")}</Label>
        <Input id="instrument-note" {...register("note")} />
      </div>
      {submissionError && (
        <p role="alert" className="text-sm text-destructive">
          {submissionError}
        </p>
      )}
      <Button type="submit" disabled={isSubmitting}>
        {isSubmitting ? t("common.pending") : submitLabel ?? t("portfolio.addInstrument")}
      </Button>
    </form>
  );
}
