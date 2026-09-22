import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { displayError } from "@/lib/display";
import { useProduct, useUpdateProductTerms } from "@/queries/liquidity";
import { ProductPolicyFields, ProductTermsFields } from "./ProductFields";
import { policyForTerms, policyFromDTO, termsFromProduct } from "./productPolicy";
import type { ProductDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

export function ProductEditSheet({ productId, open, onOpenChange }: {
  productId: string; open: boolean; onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const detail = useProduct(productId, open);
  return <Sheet open={open} onOpenChange={onOpenChange}>
    <SheetContent className="overflow-y-auto">
      <SheetHeader><SheetTitle>{t("availableFunds.editTerms")}</SheetTitle></SheetHeader>
      {detail.isLoading && <p>{t("ui.state.loadingPage")}</p>}
      {detail.isError && <p role="alert">{t("availableFunds.loadError")}</p>}
      {detail.data && <ProductEditForm key={`${productId}:${detail.data.product.revision}`} product={detail.data.product} onSaved={() => onOpenChange(false)} />}
    </SheetContent>
  </Sheet>;
}

function ProductEditForm({ product, onSaved }: { product: ProductDTO; onSaved: () => void }) {
  const { t } = useTranslation();
  const save = useUpdateProductTerms();
  const [terms, setTerms] = useState(() => termsFromProduct(product));
  const [rules, setRules] = useState(() => policyFromDTO(product.policy));
  const [error, setError] = useState<string>();
  const policy = product.state === "open" ? policyForTerms(terms, rules) : rules;
  return <div className="flex flex-col gap-4">
    {product.state === "open" ? <>
      <ProductTermsFields value={terms} onChange={setTerms} editing />
      <ProductPolicyFields value={policy} onChange={setRules} termDeposit={terms.kind === "term_deposit"} />
    </> : <>
      <p>{t("availableFunds.closedMetadataOnly")}</p>
      <Label htmlFor="closed-name">{t("availableFunds.name")}</Label><Input id="closed-name" value={terms.name} onChange={(e) => setTerms({ ...terms, name:e.target.value })} />
      <Label htmlFor="closed-note">{t("availableFunds.contractNote")}</Label><Input id="closed-note" value={terms.note ?? ""} onChange={(e) => setTerms({ ...terms, note:e.target.value || null })} />
    </>}
    {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    <SheetFooter><Button type="button" disabled={save.isPending} onClick={() => {
      setError(undefined);
      void save.mutateAsync({ productId: product.id, expectedRevision: product.revision, terms, policy }).then(onSaved).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
    }}>{t("availableFunds.savePolicy")}</Button></SheetFooter>
  </div>;
}
