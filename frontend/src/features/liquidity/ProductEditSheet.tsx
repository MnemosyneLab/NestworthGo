import { DepositFields } from "./DepositFields";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { displayError } from "@/lib/display";
import { formatAmount } from "@/lib/money";
import { useProduct, useUpdateProductTerms } from "@/queries/liquidity";
import { ProductPolicyFields, ProductTermsFields } from "./ProductFields";
import { policyForTerms, policyFromDTO, termsFromProduct } from "./productPolicy";
import type { ProductDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";

export function ProductEditSheet({ productId, open, onOpenChange }: {
  productId: string; open: boolean; onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const detail = useProduct(productId, open);
  const save = useUpdateProductTerms();
  return <Sheet open={open} onOpenChange={(next) => { if (!save.isPending) onOpenChange(next); }}>
    <SheetContent className="overflow-y-auto">
      <SheetHeader><SheetTitle>{t("availableFunds.editTerms")}</SheetTitle></SheetHeader>
      {detail.isLoading && <p>{t("ui.state.loadingPage")}</p>}
      {detail.isError && <p role="alert">{t("availableFunds.loadError")}</p>}
      {detail.data && <ProductEditForm key={`${productId}:${detail.data.product.revision}`} product={detail.data.product} save={save} onSaved={() => onOpenChange(false)} />}
    </SheetContent>
  </Sheet>;
}

function ProductEditForm({ product, save, onSaved }: { product: ProductDTO; save: ReturnType<typeof useUpdateProductTerms>; onSaved: () => void }) {
  const { t } = useTranslation();
  const [terms, setTerms] = useState(() => termsFromProduct(product));
  const [rules, setRules] = useState(() => policyFromDTO(product.policy));
  const [error, setError] = useState<string>();
  const [invalidField, setInvalidField] = useState<string>();
  const formRef = useRef<HTMLFieldSetElement>(null);
  useEffect(() => {
    if (!invalidField) return;
    const field = formRef.current?.querySelector<HTMLElement>('[aria-invalid="true"]');
    field?.closest("details")?.setAttribute("open", "");
    field?.focus();
  }, [invalidField, error]);
  const policy = product.state === "open" ? policyForTerms(terms, rules) : rules;
  return <div className="flex flex-col gap-4">
    <fieldset ref={formRef} disabled={save.isPending} className="flex min-w-0 flex-col gap-4">
    {product.state === "open" ? <>
      <p>{t("availableFunds.principal")}: {formatAmount(product.principal.amount, product.principal.currency)}</p>
      {terms.kind === "term_deposit" ? <DepositFields terms={terms} policy={policy} principal={product.principal.amount} onTermsChange={setTerms} onPolicyChange={setRules} invalidField={invalidField} errorId="product-edit-error" editing /> : <>
        <ProductTermsFields value={terms} onChange={setTerms} invalidField={invalidField} errorId="product-edit-error" editing />
        <ProductPolicyFields value={policy} onChange={setRules} />
      </>}
    </> : <>
      <p>{t("availableFunds.closedMetadataOnly")}</p>
      <Label htmlFor="closed-name">{t("availableFunds.name")}</Label><Input id="closed-name" aria-invalid={invalidField === "name"} aria-describedby={invalidField === "name" ? "product-edit-error" : undefined} value={terms.name} onChange={(e) => setTerms({ ...terms, name:e.target.value })} />
      <Label htmlFor="closed-note">{t("availableFunds.contractNote")}</Label><Input id="closed-note" value={terms.note ?? ""} onChange={(e) => setTerms({ ...terms, note:e.target.value || null })} />
    </>}
    </fieldset>
    {error && <p id="product-edit-error" role="alert" className="text-sm text-destructive">{error}</p>}
    <SheetFooter><Button type="button" variant="ghost" disabled={save.isPending} onClick={onSaved}>{t("availableFunds.cancel")}</Button><Button type="button" disabled={save.isPending} onClick={() => {
      setError(undefined);
      setInvalidField(undefined);
      void save.mutateAsync({ productId: product.id, expectedRevision: product.revision, terms, policy }).then(onSaved).catch((cause) => {
        setInvalidField((cause as { wireError?: { field?: string } })?.wireError?.field);
        setError(displayError(cause, t("availableFunds.loadError")));
      });
    }}>{t("availableFunds.savePolicy")}</Button></SheetFooter>
  </div>;
}
