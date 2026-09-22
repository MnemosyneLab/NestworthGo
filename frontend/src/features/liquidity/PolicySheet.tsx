import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { defaultProductPolicy, policyFromDTO } from "@/features/liquidity/productPolicy";
import { displayError } from "@/lib/display";
import {
  useResetPolicy,
  useSavePolicy
} from "@/queries/liquidity";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { ProductPolicyInput } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import type { LiquiditySourceDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import { ProductEditSheet } from "./ProductEditSheet";
import { SimplePolicyFields } from "./SimplePolicyFields";
import { sourceName } from "./sourceName";


export function PolicySheet({
  source,
  open,
  onOpenChange,
}: {
  source: LiquiditySourceDTO | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  if (!source) return null;
  if (source.productId) return <ProductEditSheet productId={source.productId} open={open} onOpenChange={onOpenChange} />;
  return <OrdinaryPolicySheet source={source} open={open} onOpenChange={onOpenChange} />;
}

function OrdinaryPolicySheet({ source, open, onOpenChange }: {
  source: LiquiditySourceDTO; open: boolean; onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const save = useSavePolicy();
  const reset = useResetPolicy();
  const [policy, setPolicy] = useState<ProductPolicyInput>(() => source.policy ? policyFromDTO(source.policy) : { ...defaultProductPolicy(null), accessKind: "unknown", settlementDays: null, normalExitFee: null });
  const [error, setError] = useState<string>();
  const expectedRevision = source.policyOrigin === "explicit" ? source.policy?.revision ?? 0 : 0;
  return <Sheet open={open} onOpenChange={onOpenChange}>
    <SheetContent className="overflow-y-auto">
      <SheetHeader><SheetTitle>{t("availableFunds.policy")}</SheetTitle></SheetHeader>
      <p className="text-sm text-muted-foreground">{sourceName(t, source)} · {source.nativeCurrency}</p>
      <SimplePolicyFields value={policy} onChange={setPolicy} />
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <SheetFooter>
        {source.policyOrigin === "explicit" && source.policy && source.policy.revision > 0 && <Button type="button" variant="outline" disabled={save.isPending || reset.isPending} onClick={() => {
          setError(undefined);
          void reset.mutateAsync({ sourceRef: source.sourceRef, expectedRevision }).then(() => onOpenChange(false)).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
        }}>{t("availableFunds.resetPolicy")}</Button>}
        <Button type="button" disabled={save.isPending || reset.isPending} onClick={() => {
          setError(undefined);
          void save.mutateAsync({ sourceRef: source.sourceRef, expectedRevision, policy }).then(() => onOpenChange(false)).catch((cause) => setError(displayError(cause, t("availableFunds.loadError"))));
        }}>{t("availableFunds.savePolicy")}</Button>
      </SheetFooter>
    </SheetContent>
  </Sheet>;
}
