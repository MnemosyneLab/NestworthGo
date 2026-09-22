import { multiplyCanonical } from "@/lib/money";
import type { ProductDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import type { ProductPolicyInput, ProductTermsInput } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";

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
    accessibleAmountCap: null,
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
  return canViewProducts(account) && !account.archivedAt;
}

export function canViewProducts(account: { trackingMode: string; balanceSheetRole: string; accountType: string }): boolean {
  return account.trackingMode === "holdings"
    && account.balanceSheetRole === "asset"
    && account.accountType !== "cash_on_hand";
}

export function policyForTerms(terms: ProductTermsInput, policy: ProductPolicyInput): ProductPolicyInput {
  return terms.kind === "term_deposit" ? { ...policy, accessKind: policy.accessKind === "unknown" || policy.accessKind === "excluded" ? policy.accessKind : "on_date", unlockOn: terms.maturityOn } : policy;
}

export function policyFromDTO(policy: import("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models").PolicyDTO): ProductPolicyInput {
  return {
    accessibleAmountCap: policy.accessibleAmountCap?.amount ?? null,
    accessKind: policy.accessKind, unlockOn: policy.unlockOn, settlementDays: policy.settlementDays,
    dayBasis: policy.dayBasis, receiptOnOverride: policy.receiptOnOverride, normalExitFee: policy.normalExitFee?.amount ?? null,
    earlyKind: policy.earlyKind, earlySettlementDays: policy.earlySettlementDays, earlyDayBasis: policy.earlyDayBasis,
    earlyFee: policy.earlyFee?.amount ?? null, earlyAmountMode: policy.earlyAmountMode,
    earlyGrossAmount: policy.earlyGrossAmount?.amount ?? null, note: policy.note,
  };
}

export function termsFromProduct(product: ProductDTO): ProductTermsInput {
  return {
    kind: product.kind, name: product.name, note: product.note, startOn: product.startOn, maturityOn: product.maturityOn,
    interestMode: product.interestMode, annualRate: null,
    annualRatePercent: product.annualRate == null ? null : multiplyCanonical(product.annualRate, "100"),
    maturityInterest: product.maturityInterest?.amount ?? null, interestPaidThroughOn: product.interestPaidThroughOn,
  };
}
