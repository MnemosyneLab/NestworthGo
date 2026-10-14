/** Ownership, including closed products, is supplied by the authoritative inventory read. */
export function ordinaryInstrument(instrument: { managedProductId?: string; archivedAt?: string | null }) {
  return !instrument.managedProductId && !instrument.archivedAt;
}
