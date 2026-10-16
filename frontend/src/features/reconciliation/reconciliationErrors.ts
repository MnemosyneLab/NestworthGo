import type { WireError } from "@/lib/wails";
export function knownReconciliationConflict(wire?: WireError): boolean {
  return (
    wire?.code === "conflict" &&
    [
      "financialRevision",
      "expectedRevision",
      "reconciliationLink",
      "revision",
      "preview",
    ].includes(wire.field ?? "")
  );
}
export function completedReconciliation(wire?: WireError): boolean {
  return (
    wire?.code === "conflict" &&
    wire.field === "expectedRevision" &&
    /completed.*read-only/.test(wire.message ?? "")
  );
}

// The wire protocol has no validation-reason enum. Match only known backend
// reasons; other errors retain the normal localized wire-error fallback.
export function adjustmentErrorKey(wire?: WireError): string | undefined {
  const field = wire?.field?.split(".").slice(-1)[0];
  if (!["unitCost", "totalCost"].includes(field ?? "")) return;
  if (
    (wire?.code === "cost_basis_required" &&
      wire.message ===
        "an explicit per-unit cost is required for added quantity") ||
    (wire?.code === "validation" &&
      wire.message === "unitCost and totalCost cannot be supplied together")
  )
    return "reconciliation.costChoice";
  const reasons: Record<string, string> = {
    "must be a canonical non-negative decimal with up to eight fractional digits":
      "costUnitInvalid",
    "must be a canonical non-negative decimal with up to four fractional digits":
      "costTotalInvalid",
    "unit cost cannot be changed by reducing quantity": "costUnitIncreaseOnly",
    "cost-only reconciliation requires totalCost": "costOnlyTotal",
    "a zero-quantity Holding must have zero total cost": "costZeroQuantity",
    "total cost currency must match the Holding's Instrument currency":
      "costCurrency",
    "total cost requires an unsupported per-unit cost": "costPrecision",
    "total cost cannot be represented at the supported cost precision":
      "costPrecision",
  };
  const reason = reasons[wire?.message ?? ""];
  return reason ? `reconciliation.${reason}` : undefined;
}
