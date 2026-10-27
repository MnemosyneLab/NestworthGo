import { createContext } from "react";

/** null means role metadata is unavailable; never assume that an unknown balance is an asset. */
export const ReconciliationLedgerContext = createContext<(accountId: string) => boolean | null>(() => false);
