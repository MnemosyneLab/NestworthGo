import type { ProductPolicyInput, ProductTermsInput } from "../../bindings/github.com/waltwang/nestworth-go/internal/application/models";

export function emptyTerms(kind: "term_deposit" | "locked_product"): ProductTermsInput {
  return {
    kind,
    name: "",
    note: null,
    startOn: "",
    maturityOn: null,
    interestMode: kind === "term_deposit" ? "none" : "none",
    annualRate: null,
    annualRatePercent: null,
    maturityInterest: null,
    interestPaidThroughOn: null,
  };
}

export function defaultProductPolicy(maturityOn: string | null): ProductPolicyInput {
  return {
    accessKind: maturityOn ? "on_date" : "on_request",
    unlockOn: maturityOn,
    settlementDays: 0,
    dayBasis: "calendar",
    receiptOnOverride: null,
    normalExitFee: "0",
    earlyKind: "not_allowed",
    earlySettlementDays: null,
    earlyDayBasis: null,
    earlyFee: null,
    earlyAmountMode: null,
    earlyGrossAmount: null,
    note: null,
  };
}

export function canHoldProducts(account: { trackingMode: string; balanceSheetRole: string; accountType: string; archivedAt?: string | null }): boolean {
  return account.trackingMode === "holdings"
    && account.balanceSheetRole === "asset"
    && account.accountType !== "cash_on_hand"
    && !account.archivedAt;
}
