import { createContext } from "react";

/** Roles come from authoritative account metadata; immutable evidence stays raw. */
export const ReconciliationLedgerContext = createContext<ReadonlySet<string>>(new Set());
