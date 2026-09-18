import { MetalConversionDetails } from "./MetalConversionDetails";
import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Service as MarketDataService } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata";
import * as InstrumentService from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument/service";
import { callService } from "@/lib/wails";
import { formatAmount } from "@/lib/money";
import { metalUnitLabel } from "@/lib/preciousMetals";
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
import { isYahooSearchableInstrumentType } from "@/queries/marketdata";
import { InstrumentYahooSearch } from "@/features/investments/InstrumentYahooSearch";
import type { InstrumentSearchHitDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata/models";

const instrumentFormSchema = z.object({
  metalTemplate: z.string(),
  quantityUnit: z.string(),
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

function defaultQuoteSourceForType(type: string): string {
  return isYahooSearchableInstrumentType(type) ? "provider" : "manual";
}

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
      metalTemplate: instrument?.metalTemplate ?? "",
      quantityUnit: instrument?.quantityUnit ?? "",
      name: instrument?.name ?? "",
      type: instrument?.type ?? "stock",
      quoteCurrency: instrument?.quoteCurrency ?? "CNY",
      symbol: instrument?.symbol ?? "",
      marketCode: instrument?.marketCode ?? "",
      countryCode: instrument?.countryCode ?? "",
      isin: instrument?.isin ?? "",
      note: instrument?.note ?? "",
      quoteSource: instrument?.quoteSource ?? defaultQuoteSourceForType(instrument?.type ?? "stock"),
      providerKey: instrument?.providerKey ?? "",
      providerSymbol: instrument?.providerSymbol ?? "",
      iconKey: instrument?.iconKey ?? "stock",
    },
  });
  const metalTemplate = useWatch({ control, name: "metalTemplate" });
  const quantityUnit = useWatch({ control, name: "quantityUnit" });
  const quoteCurrency = useWatch({ control, name: "quoteCurrency" });
  const quoteSource = useWatch({ control, name: "quoteSource" });
  const providerKey = useWatch({ control, name: "providerKey" });
  const instrumentType = useWatch({ control, name: "type" }) ?? "stock";
  const iconKey = useWatch({ control, name: "iconKey" });
  const countryCode = useWatch({ control, name: "countryCode" }) ?? "";
  const marketCode = useWatch({ control, name: "marketCode" }) ?? "";
  const [iconCustomized, setIconCustomized] = useState(Boolean(instrument));
  const householdCurrency = bootstrap.data?.household?.baseCurrency;
  const cryptoCurrencies = useQuery({
    queryKey: ["coingecko-currencies"],
    queryFn: () => callService(() => MarketDataService.CoinGeckoQuoteCurrencies()),
    enabled: instrumentType === "crypto" && providerKey === "coingecko",
    staleTime: 86_400_000,
    retry: false,
  });
  const allCurrencies = currencies.data ?? (householdCurrency ? [householdCurrency] : []);
  const currencyOptions = instrumentType === "crypto" && providerKey === "coingecko" && cryptoCurrencies.data?.length
    ? allCurrencies.filter((currency) => cryptoCurrencies.data!.includes(currency))
    : allCurrencies;
  const instrumentTypes = catalog.data?.instrumentTypes?.length ? catalog.data.instrumentTypes : [instrument?.type ?? "stock"];
  const quoteSources = catalog.data?.quoteSources?.length ? catalog.data.quoteSources : ["manual", "provider"];
  const instrumentProviders = catalog.data?.instrumentProviders?.length ? catalog.data.instrumentProviders : ["yahoo_finance"];
  const allMarketCodes = Array.from(new Set([...(catalog.data?.instrumentMarketCodes ?? []), ...(instrument?.marketCode ? [instrument.marketCode] : []), ...(marketCode ? [marketCode] : [])]));
  const countryOptions = Array.from(new Set([...(catalog.data?.instrumentCountryCodes ?? []), ...(instrument?.countryCode ? [instrument.countryCode] : [])]));
  const marketOptions = marketsForCountry(allMarketCodes, countryCode, marketCode);
  const defaultProviderKey = instrumentType === "crypto" ? "coingecko" : catalog.data?.instrumentProviders?.[0] ?? "yahoo_finance";

  useEffect(() => {
    if (instrument) return;
    const next = instrumentType === "crypto" ? "USD" : householdCurrency ?? currencies.data?.[0];
    if (next) {
      setValue("quoteCurrency", next);
    }
  }, [currencies.data, householdCurrency, instrument, instrumentType, setValue]);

  useEffect(() => {
    if (instrument) return;
    if (!instrumentType) return;
    setValue("quoteSource", defaultQuoteSourceForType(instrumentType));
  }, [instrument, instrumentType, setValue]);

  useEffect(() => {
    if (instrument) return;
    if (!metalTemplate && quoteSource === "provider" && defaultProviderKey) {
      setValue("providerKey", defaultProviderKey);
    }
  }, [defaultProviderKey, instrument, metalTemplate, quoteSource, setValue]);

  const metalPreview = useQuery({
    queryKey: ["metal-quote-preview", metalTemplate, quantityUnit, quoteCurrency],
    queryFn: () => callService(() => InstrumentService.PreviewMetalQuote(metalTemplate, quantityUnit, quoteCurrency)),
    enabled: Boolean(metalTemplate && quantityUnit && quoteCurrency && quoteSource === "provider"),
    staleTime: 60_000,
    retry: false,
    refetchOnWindowFocus: false,
  });

  const applyMetalTemplate = (template: string) => {
    setValue("metalTemplate", template);
    setValue("quantityUnit", template ? "g" : "");
    setValue("quoteSource", template ? "provider" : "manual");
    setValue("providerKey", template ? "yahoo_finance" : "");
    setValue("providerSymbol", template === "gold" ? "GC=F" : template === "silver" ? "SI=F" : "");
    setValue("symbol", "");
    setValue("marketCode", template ? "COMEX" : "");
    setValue("countryCode", "");
    setValue("isin", "");
    if (template) setValue("name", t(`metals.${template}`));
  };

  const applyYahooSearchHit = (hit: InstrumentSearchHitDTO) => {
    setValue("type", hit.type || instrumentType);
    setValue("name", hit.name);
    setValue("symbol", hit.symbol);
    setValue("quoteSource", "provider");
    setValue("providerKey", hit.providerKey || defaultProviderKey || "yahoo_finance");
    setValue("providerSymbol", hit.providerSymbol);
    if (hit.type === "crypto") { setValue("countryCode", ""); setValue("isin", ""); }
    if (hit.countryCode) {
      setValue("countryCode", hit.countryCode);
    }
    if (hit.marketCode) {
      setValue("marketCode", hit.marketCode);
    }
    if (!instrument && hit.quoteCurrency) {
      setValue("quoteCurrency", hit.quoteCurrency);
    }
  };

  useEffect(() => {
    if (!iconCustomized) setValue("iconKey", INSTRUMENT_TYPE_ICONS[instrumentType] ?? "investment");
  }, [iconCustomized, instrumentType, setValue]);

  const submit = (values: InstrumentFormValues) => {
    onSubmit({
      replace: Boolean(instrument),
      metalTemplate: values.metalTemplate || undefined,
      quantityUnit: values.quantityUnit || undefined,
      name: values.name,
      type: values.type,
      quoteCurrency: values.quoteCurrency,
      symbol: values.symbol?.trim() || undefined,
      marketCode: values.marketCode?.trim() || undefined,
      countryCode: values.countryCode?.trim().toUpperCase() || undefined,
      isin: values.isin?.trim() || undefined,
      note: values.note?.trim() || undefined,
      quoteSource: values.quoteSource,
      providerKey: values.quoteSource === "provider" || values.metalTemplate ? values.providerKey : undefined,
      providerSymbol: values.quoteSource === "provider" || values.metalTemplate ? values.providerSymbol : undefined,
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
        <NativeSelect id="instrument-type" {...register("type")} disabled={Boolean(instrument?.metalTemplate)} onChange={(event) => { setValue("type", event.target.value); if (!instrument && (instrumentType === "crypto" || event.target.value === "crypto")) { setValue("providerSymbol", ""); setValue("symbol", ""); setValue("countryCode", ""); setValue("marketCode", event.target.value === "crypto" ? "CRYPTO" : ""); } if (!instrument && metalTemplate) applyMetalTemplate(""); }}>
          {instrumentTypes.map((type) => (
            <option key={type} value={type}>
              {displayEnum(t, "enum", type)}
            </option>
          ))}
        </NativeSelect>
      </div>
      {instrumentType === "precious_metal" && (
        <div className="flex flex-col gap-3 rounded-md border border-border p-3">
          <Label htmlFor="metal-template">{t("metals.template")}</Label>
          <NativeSelect id="metal-template" value={metalTemplate} disabled={Boolean(instrument)} onChange={(event) => applyMetalTemplate(event.target.value)}>
            <option value="">{t("metals.custom")}</option>
            <option value="gold">{t("metals.gold")}</option>
            <option value="silver">{t("metals.silver")}</option>
          </NativeSelect>
          {metalTemplate && <>
            <Label htmlFor="metal-unit">{t("metals.holdingUnit")}</Label>
            <NativeSelect id="metal-unit" {...register("quantityUnit")} disabled={Boolean(instrument)}>
              <option value="g">{t("metals.gram")}</option>
              <option value="troy_oz">{t("metals.troyOunce")}</option>
            </NativeSelect>
            <p className="text-xs text-muted-foreground">{t("metals.unitHelp")}</p>
            {instrument && <p className="text-xs text-muted-foreground">{t("metals.immutable")}</p>}
          </>}
        </div>
      )}
      {isYahooSearchableInstrumentType(instrumentType) && (
        <InstrumentYahooSearch key={instrumentType} instrumentType={instrumentType} onSelect={applyYahooSearchHit} />
      )}
      {instrumentType === "crypto" && providerKey === "coingecko" && <p className="text-xs text-muted-foreground">{t("portfolio.coinGeckoNotice")} · <a href="https://www.coingecko.com/en/api" target="_blank" rel="noreferrer" className="underline">{t("portfolio.coinGeckoAttribution")}</a></p>}
      <IconPicker id="instrument-icon" value={iconKey} kind="instrument" onChange={(key) => { setValue("iconKey", key); setIconCustomized(true); }} />
      <details open={!metalTemplate}><summary className="cursor-pointer text-sm text-muted-foreground">{t("metals.identityDetails")}</summary>
      <div className="mt-3 grid gap-4 sm:grid-cols-2">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="instrument-country-code">{t("portfolio.countryCode")}</Label>
          <NativeSelect
            disabled={Boolean(metalTemplate)}
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
            disabled={Boolean(metalTemplate)}
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
      </details>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="instrument-currency">{t("accounts.currency")}</Label>
          {instrument ? (
          <>
            <Input id="instrument-currency" readOnly {...register("quoteCurrency")} />
            <p className="text-xs text-muted-foreground">{t("review.currencyImmutable")}</p>
          </>
        ) : (
          <NativeSelect id="instrument-currency" {...register("quoteCurrency")}>
            {currencyOptions.map((currency) => <option key={currency} value={currency}>{currency}</option>)}
          </NativeSelect>
        )}
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
      {quoteSource === "provider" && !metalTemplate && (
        <>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="instrument-provider-key">{t("portfolio.providerKey")}</Label>
            <NativeSelect id="instrument-provider-key" {...register("providerKey")}>
              {instrumentProviders.filter((provider) => provider !== "coingecko" || instrumentType === "crypto").map((provider) => (
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
      {metalTemplate && <div className="rounded-md bg-muted p-3 text-sm" aria-live="polite">
        {quoteSource === "provider" && <>
          <p className="font-medium">{metalPreview.isFetching ? t("metals.loading") : metalPreview.data ? `${formatAmount(metalPreview.data.unitPrice, metalPreview.data.currency)} / ${metalUnitLabel(quantityUnit, t)}` : t("metals.previewUnavailable")}</p>
          <p className="mt-1 text-xs text-muted-foreground">Yahoo · {metalTemplate === "gold" ? "GC=F" : "SI=F"} · USD / oz t</p>
          {metalPreview.data && <MetalConversionDetails evidence={metalPreview.data.conversionJSON} />}
          {metalPreview.isError && <Button type="button" size="sm" variant="ghost" onClick={() => void metalPreview.refetch()}>{t("metals.retry")}</Button>}
        </>}
        <p className="mt-1 text-xs text-muted-foreground">{t("metals.referenceDisclaimer")}</p>
      </div>}
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
