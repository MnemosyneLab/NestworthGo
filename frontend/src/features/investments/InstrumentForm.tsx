import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useSupportedCurrencies } from "@/queries/settings";
import type { InstrumentRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument/models";

const INSTRUMENT_TYPES = ["stock", "etf", "mutual_fund", "crypto", "bond", "precious_metal", "bank_investment_product", "other"];

const instrumentFormSchema = z.object({
  name: z.string().trim().min(1),
  type: z.string(),
  quoteCurrency: z.string().length(3),
  quoteSource: z.enum(["manual", "provider"]),
  providerKey: z.string().optional(),
  providerSymbol: z.string().optional(),
});

type InstrumentFormValues = z.infer<typeof instrumentFormSchema>;

/** InstrumentForm creates an Instrument.
 * Provider-bound instruments require a provider key/symbol per
 * domain.NewInstrument's validation; this form only collects them when
 * quoteSource is "provider," matching the backend's rule. */
export function InstrumentForm({ onSubmit, isSubmitting }: { onSubmit: (request: InstrumentRequest) => void; isSubmitting: boolean }) {
  const { t } = useTranslation();
  const currencies = useSupportedCurrencies();
  const {
    register,
    watch,
    handleSubmit,
    formState: { errors },
  } = useForm<InstrumentFormValues>({
    resolver: zodResolver(instrumentFormSchema),
    defaultValues: { name: "", type: "stock", quoteCurrency: "USD", quoteSource: "manual" },
  });
  const quoteSource = watch("quoteSource");

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
    <form onSubmit={handleSubmit(submit)} className="flex flex-col gap-4" aria-label="Instrument form">
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
        <Label htmlFor="instrument-type">Type</Label>
        <select id="instrument-type" {...register("type")} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
          {INSTRUMENT_TYPES.map((type) => (
            <option key={type} value={type}>
              {type}
            </option>
          ))}
        </select>
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="instrument-currency">{t("accounts.currency")}</Label>
        <select id="instrument-currency" {...register("quoteCurrency")} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
          {(currencies.data ?? ["USD"]).map((currency) => (
            <option key={currency} value={currency}>
              {currency}
            </option>
          ))}
        </select>
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="instrument-quote-source">Quote source</Label>
        <select id="instrument-quote-source" {...register("quoteSource")} className="h-9 rounded-md border border-border bg-card px-3 text-sm text-foreground">
          <option value="manual">manual</option>
          <option value="provider">provider</option>
        </select>
      </div>
      {quoteSource === "provider" && (
        <>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="instrument-provider-key">Provider key</Label>
            <Input id="instrument-provider-key" {...register("providerKey")} placeholder="yahoo_finance" />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="instrument-provider-symbol">Provider symbol</Label>
            <Input id="instrument-provider-symbol" {...register("providerSymbol")} placeholder="NVDA" />
          </div>
        </>
      )}
      <Button type="submit" disabled={isSubmitting}>
        {t("common.add")}
      </Button>
    </form>
  );
}
