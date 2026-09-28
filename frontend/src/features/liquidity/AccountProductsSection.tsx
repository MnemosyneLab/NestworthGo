import { productStateLabel } from "./liquidityDisplay";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { formatAmount } from "@/lib/money";
import { displayEnum } from "@/lib/display";
import { useProducts } from "@/queries/liquidity";
import { canViewProducts, canHoldProducts } from "@/features/liquidity/productPolicy";
import { ProductDetailSheet, ProductFormSheet, moneyText } from "@/features/liquidity/ProductSheets";
import { useObjectNavigation } from "@/app/NavigationContext";
import type { AccountRecordDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

export function AccountProductsSection({
  record,
  cashAmount,
}: {
  record: AccountRecordDTO;
  cashAmount?: string;
}) {
  const { t } = useTranslation();
  const navigation = useObjectNavigation();
  const products = useProducts({ accountId: record.account.id, includeClosed: true });
  const [openForm, setOpenForm] = useState(false);
  const [productId, setProductId] = useState<string | null>(null);
  const eligible = canHoldProducts(record.account);

  if (!canViewProducts(record.account)) {
    return (
      <Card data-testid="account-products">
        <CardHeader>
          <CardTitle>{t("availableFunds.products")}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <p className="text-sm text-muted-foreground">{t("availableFunds.simpleAccountHint")}</p>
          {navigation && (
            <Button type="button" variant="outline" size="sm" className="self-start" onClick={() => navigation.open({ page: "accounts" })}>
              {t("availableFunds.createHoldingsAccount")}
            </Button>
          )}
        </CardContent>
      </Card>
    );
  }

  if (products.isLoading) {
    return <LoadingState label={t("ui.state.loadingPage")} />;
  }

  if (products.isError) return <ErrorState title={t("availableFunds.loadError")} description={t("ui.state.errorDescription")} onRetry={() => void products.refetch()} retryLabel={t("common.retryAction")} />;

  const items = products.data ?? [];
  return (
    <Card data-testid="account-products">
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <CardTitle>{t("availableFunds.products")}</CardTitle>
        {eligible && (
          <Button type="button" size="sm" onClick={() => setOpenForm(true)}>{t("availableFunds.addProduct")}</Button>
        )}
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {items.length === 0 ? (
          <EmptyState title={t("availableFunds.products")} description={t("availableFunds.unsupportedPartial")} />
        ) : (
          <table className="w-full text-left text-sm">
            <caption className="sr-only">{t("availableFunds.products")}</caption>
            <thead>
              <tr className="border-b border-border text-muted-foreground">
                <th className="py-2 font-medium">{t("availableFunds.name")}</th>
                <th className="py-2 font-medium">{t("availableFunds.asset")}</th>
                <th className="py-2 font-medium">{t("availableFunds.principal")}</th>
                <th className="py-2 font-medium">{t("availableFunds.currentContractValue")}</th>
                <th className="py-2 font-medium">{t("availableFunds.maturityOn")}</th>
                <th className="py-2 font-medium">{t("availableFunds.nextAction")}</th>
              </tr>
            </thead>
            <tbody>
              {items.map((detail) => {
                const product = detail.product;
                return (
                  <tr key={product.id} className="border-b border-border last:border-0">
                    <td className="py-2">
                      <Button type="button" variant="link" className="h-auto p-0" onClick={() => setProductId(product.id)}>
                        {product.name}
                      </Button>
                    </td>
                    <td className="py-2">{displayEnum(t, "availableFunds", product.kind === "term_deposit" ? "termDeposit" : "lockedProduct")}</td>
                    <td className="py-2">{formatAmount(product.principal.amount, product.principal.currency)}</td>
                    <td className="py-2">{moneyText(product.currentValue, t("availableFunds.unknownAmount"))}</td>
                    <td className="py-2">{product.maturityOn ?? product.policy.unlockOn ?? t("availableFunds.unknownAmount")}</td>
                    <td className="py-2">
                      <Badge variant={product.displayState === "due_unconfirmed" ? "warning" : "secondary"}>
                        {productStateLabel(t, product.displayState)}
                      </Badge>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
        {openForm && <ProductFormSheet
          accountId={record.account.id}
          accountName={record.account.name}
          currency={record.account.defaultCurrency}
          cashAmount={cashAmount}
          open={openForm}
          onOpenChange={setOpenForm}
        />}
        <ProductDetailSheet key={productId ?? "product"} productId={productId} open={Boolean(productId)} onOpenChange={(open) => { if (!open) setProductId(null); }} />
      </CardContent>
    </Card>
  );
}
