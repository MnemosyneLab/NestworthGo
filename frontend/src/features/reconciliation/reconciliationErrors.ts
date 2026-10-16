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
