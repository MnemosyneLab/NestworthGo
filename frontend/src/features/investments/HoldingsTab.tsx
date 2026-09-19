import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Plus } from "lucide-react";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { PageChrome } from "@/components/layout/PageChrome";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { useHistoryOrigin } from "@/queries/history";
import { useAccounts } from "@/queries/accounts";
import {
  useInstruments,
  useCreateHolding,
} from "@/queries/investments";
import { useInstrumentHoldings } from "@/queries/analytics";
import { displayError } from "@/lib/display";
import { instrumentDisplayLabel } from "@/lib/instrumentDisplay";
import { InstrumentHoldingsTable } from "./InstrumentHoldingsTable";

export function HoldingsTab({ onOpenAccount, active = true }: { onOpenAccount?: (id: string) => void; active?: boolean }) {
  const { t } = useTranslation();
  const accounts = useAccounts({});
  const origin = useHistoryOrigin();
  const instruments = useInstruments();
  const createHolding = useCreateHolding();
  const [open, setOpen] = useState(false);
  const [accountId, setAccountId] = useState("");
  const [instrumentId, setInstrumentId] = useState("");
  const [quantity, setQuantity] = useState("");
  const [unitCost, setUnitCost] = useState("");
  const [formError, setFormError] = useState<string | undefined>();

  const holdingsAccounts = (accounts.data ?? []).filter(
    (record) => record.account.trackingMode === "holdings" && record.account.accountType !== "cash_on_hand" && !record.account.archivedAt,
  );
  const holdings = useInstrumentHoldings();

  if (accounts.isLoading || instruments.isLoading || holdings.isLoading || origin.isLoading) {
    return <LoadingState label={t("portfolio.loading")} />;
  }

  if (accounts.isError || instruments.isError || holdings.isError || origin.isError) {
    return (
      <ErrorState
        title={t("portfolio.loadError")}
        description={t("ui.state.errorDescription")}
        onRetry={() => {
          void accounts.refetch();
          void instruments.refetch();
          void holdings.refetch();
          void origin.refetch();
        }}
        retryLabel={t("common.retryAction")}
      />
    );
  }

  const allHoldings = holdings.data ?? [];

  if (holdingsAccounts.length === 0) {
    return <EmptyState title={t("portfolio.noHoldingsAccount")} description={t("portfolio.noHoldingsDescription")} />;
  }

  const submit = () => {
    if (!accountId || !instrumentId || !quantity.trim()) {
      setFormError(t("portfolio.holdingRequired"));
      return;
    }
    setFormError(undefined);
    createHolding.mutate(
      { accountId, instrumentId, quantity: quantity.trim(), ...(origin.data && unitCost.trim() ? { unitCost: unitCost.trim() } : {}) },
      {
        onSuccess: () => {
          toast.success(t("portfolio.holdingCreated"));
          setOpen(false);
          setAccountId("");
          setInstrumentId("");
          setQuantity("");
          setUnitCost("");
        },
      },
    );
  };

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-muted-foreground">{t("portfolio.description")}</p>
      <p className="text-sm text-muted-foreground">{t("portfolio.holdingsIndexDescription")}</p>
      <PageChrome
        pageId="portfolio"
        enabled={active}
        actions={
          <Sheet open={open} onOpenChange={setOpen}>
            <SheetTrigger className={buttonVariants({ size: "sm" })}>
              <Plus className="size-4" aria-hidden="true" /> {t("portfolio.addHolding")}
            </SheetTrigger>
            <SheetContent className="overflow-y-auto overscroll-contain">
              <SheetHeader>
                <SheetTitle>{t("portfolio.addHolding")}</SheetTitle>
              </SheetHeader>
              <div className="flex flex-col gap-4">
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="holding-account">{t("history.accountSelect")}</Label>
                  <NativeSelect id="holding-account" value={accountId} onChange={(event) => setAccountId(event.target.value)}>
                    <option value="">{t("history.accountSelectEmpty")}</option>
                    {holdingsAccounts.map((record) => (
                      <option key={record.account.id} value={record.account.id}>
                        {record.account.name}
                      </option>
                    ))}
                  </NativeSelect>
                </div>
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="holding-instrument">{t("history.instrument")}</Label>
                  <NativeSelect id="holding-instrument" value={instrumentId} onChange={(event) => setInstrumentId(event.target.value)}>
                    <option value="">{t("history.selectInstrument")}</option>
                    {(instruments.data ?? []).map((instrument) => (
                      <option key={instrument.id} value={instrument.id}>
                        {instrumentDisplayLabel(instrument, instrument.name)}
                      </option>
                    ))}
                  </NativeSelect>
                </div>
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="holding-quantity">{t("history.quantity")}</Label>
                  <Input id="holding-quantity" value={quantity} onChange={(event) => setQuantity(event.target.value)} inputMode="decimal" />
                </div>
                {origin.data && (
                  <div className="flex flex-col gap-1.5">
                    <Label htmlFor="holding-unit-cost">{t("error.field.unitCost")}</Label>
                    <Input id="holding-unit-cost" value={unitCost} onChange={event => setUnitCost(event.target.value)} inputMode="decimal" aria-describedby="holding-cost-hint" />
                    <p id="holding-cost-hint" className="text-xs text-muted-foreground">{t("holdingsSummary.unitCostHint")}</p>
                  </div>
                )}
                {(formError || createHolding.isError) && (
                  <p role="alert" className="text-sm text-destructive">
                    {formError ?? displayError(createHolding.error, t("portfolio.createError"))}
                  </p>
                )}
                <Button onClick={submit} disabled={createHolding.isPending}>
                  {createHolding.isPending ? t("common.pending") : t("portfolio.addHolding")}
                </Button>
              </div>
            </SheetContent>
          </Sheet>
        }
      />

      {allHoldings.length === 0 ? (
        <EmptyState title={t("portfolio.noHoldings")} description={t("portfolio.noHoldingsDescription")} />
      ) : (
        <InstrumentHoldingsTable groups={allHoldings} onOpenAccount={onOpenAccount} />
      )}
    </div>
  );
}
