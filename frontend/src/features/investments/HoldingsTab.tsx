import { ordinaryInstrument } from "@/lib/instrumentEligibility";
import { useEffect, useRef, useState } from "react";
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
  useCreateInstrument,
} from "@/queries/investments";
import { useObjectNavigation } from "@/app/NavigationContext";
import { useInstrumentHoldingGroups } from "@/queries/analytics";
import { displayError } from "@/lib/display";
import { instrumentSelectionLabels } from "@/lib/instrumentDisplay";
import { HoldingGroupsList } from "./HoldingGroupsList";
import { useProducts } from "@/queries/liquidity";
import { InstrumentForm } from "./InstrumentForm";
import { PositionImportForm } from "./PositionImportForm";

export function HoldingsTab({ onOpenAccount, active = true, draftRequest }: { onOpenAccount?: (id: string) => void; active?: boolean; draftRequest?: { requestId: number; accountId: string } }) {
  const { t } = useTranslation();
  const navigation = useObjectNavigation();
  const accounts = useAccounts({});
  const origin = useHistoryOrigin();
  const instruments = useInstruments();
  const createHolding = useCreateHolding();
  const createInstrument = useCreateInstrument();
  const [open, setOpen] = useState(false);
  const [creatingInstrument, setCreatingInstrument] = useState(false);
  const [accountId, setAccountId] = useState("");
  const [instrumentId, setInstrumentId] = useState("");
  const [quantity, setQuantity] = useState("");
  const [unitCost, setUnitCost] = useState("");
  const [formError, setFormError] = useState<string | undefined>();

  const holdingsAccounts = (accounts.data ?? []).filter(
    (record) => record.account.trackingMode === "holdings" && record.account.accountType !== "cash_on_hand" && !record.account.archivedAt,
  );
  const holdings = useInstrumentHoldingGroups();
  const products = useProducts({ includeClosed: true }, open && !origin.isLoading && !origin.data);
  const productsReady = products.isSuccess && products.fetchStatus === "idle";
  const excludedInstrumentIds = new Set((products.data ?? []).map((detail) => detail.product.instrumentId));
  const selectableInstruments = productsReady ? (instruments.data ?? []).filter((instrument) => ordinaryInstrument(instrument) && !excludedInstrumentIds.has(instrument.id)) : [];
  const selectionLabels = instrumentSelectionLabels(selectableInstruments);
  const handledDraft = useRef(0);
  useEffect(() => {
    if (!draftRequest || handledDraft.current === draftRequest.requestId || !holdingsAccounts.some((record) => record.account.id === draftRequest.accountId)) return;
    handledDraft.current = draftRequest.requestId;
    setAccountId(draftRequest.accountId);
    setOpen(true);
  }, [draftRequest, holdingsAccounts]);
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
    return <EmptyState title={t("portfolio.noHoldingsAccount")} description={t("portfolio.noHoldingsDescription")}
      action={navigation && <Button type="button" onClick={() => navigation.open({ page: "accounts", create: true })}>{t("investmentAssets.createAccount")}</Button>} />;
  }

  const submit = () => {
    if (!productsReady || !accountId || !selectableInstruments.some(item => item.id === instrumentId) || !quantity.trim()) {
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
      <PageChrome
        pageId="portfolio"
        enabled={active}
        actions={
          <Sheet open={open} onOpenChange={setOpen}>
            <SheetTrigger className={buttonVariants({ size: "sm" })}>
              <Plus className="size-4" aria-hidden="true" /> {t(origin.data ? "investmentAssets.recordExisting" : "portfolio.addHolding")}
            </SheetTrigger>
            <SheetContent className="overflow-y-auto overscroll-contain">
              <SheetHeader>
                <SheetTitle>{t(origin.data ? "investmentAssets.recordExisting" : "portfolio.addHolding")}</SheetTitle>
              </SheetHeader>
              {origin.data ? <PositionImportForm account={holdingsAccounts.find((record) => record.account.id === accountId)?.account}
                onRecorded={() => { toast.success(t("history.recorded")); setOpen(false); setAccountId(""); }} /> : <div className="flex flex-col gap-4">
                {products.isError ? <ErrorState title={t("portfolio.loadError")} description={t("ui.state.errorDescription")} onRetry={() => void products.refetch()} retryLabel={t("common.retryAction")} /> : !productsReady ? <LoadingState label={t("portfolio.loading")} /> : null}
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
                  <NativeSelect id="holding-instrument" value={productsReady && selectableInstruments.some((item) => item.id === instrumentId) ? instrumentId : ""} disabled={!productsReady} onChange={(event) => setInstrumentId(event.target.value)}>
                    <option value="">{t("history.selectInstrument")}</option>
                    {selectableInstruments.map((instrument) => (
                      <option key={instrument.id} value={instrument.id}>
                        {selectionLabels.get(instrument.id) ?? instrument.name}
                      </option>
                    ))}
                  </NativeSelect>
                </div>
                {creatingInstrument ? <div className="flex flex-col gap-3">
                  <InstrumentForm isSubmitting={createInstrument.isPending}
                    submissionError={createInstrument.isError ? displayError(createInstrument.error, t("portfolio.createError")) : undefined}
                    onSubmit={(request) => createInstrument.mutate(request, { onSuccess: (created) => { setInstrumentId(created.id); setCreatingInstrument(false); } })} />
                  <Button type="button" variant="ghost" className="self-start" onClick={() => setCreatingInstrument(false)}>{t("common.cancel")}</Button>
                </div> : <Button type="button" variant="ghost" className="self-start" onClick={() => setCreatingInstrument(true)}>{t("accounts.createInstrument")}</Button>}
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
                <Button onClick={submit} disabled={createHolding.isPending || !productsReady}>
                  {createHolding.isPending ? t("common.pending") : t("portfolio.addHolding")}
                </Button>
              </div>}
            </SheetContent>
          </Sheet>
        }
      />

      {allHoldings.length === 0 ? (
        <EmptyState title={t("portfolio.noHoldings")} description={t("portfolio.noHoldingsDescription")} />
      ) : (
        <HoldingGroupsList scope="portfolio" active={active} groups={allHoldings} onOpenAccount={onOpenAccount} />
      )}
    </div>
  );
}
