export type Permissions = {
  read: boolean;
  record: boolean;
  market_refresh: boolean;
  manual_quote: boolean;
};
export type Session = {
  device: { id: string; name: string; permissions: Permissions };
  csrfToken: string;
  instance: string;
};
export type Envelope<T> = {
  data: T;
  ledgerTimezone: string;
  asOf: string;
  dataVersion: string;
};
export type Money = { amount: string; currency: string };
export type Metric = {
  total: Money | null;
  knownSubtotal: Money | null;
  currency: string;
};
export type Account = {
  id: string;
  name: string;
  institution: string;
  currency: string;
  mode: string;
  type: string;
  role: string;
  value: Money | null;
  complete: boolean;
};
export type Activity = {
  id: string;
  kind: string;
  reason: string;
  effectiveAt: string;
  effectiveLocalDate: string;
  note?: string;
  resulting: {
    name: string;
    amount: string;
    quantity?: string;
    currency: string;
  }[];
  effects: { classification: string; money?: Money; direction: string }[];
};
export type Issue = {
  ID: string;
  Kind: string;
  Severity: string;
  Label: string;
  Reason: string;
  Action: string;
  InstrumentID: string;
  AccountID: string;
  CurrencyA: string;
  CurrencyB: string;
  RangeStart: string;
  RangeEnd: string;
  Executable: boolean;
  Collapsed: boolean;
};
export type MarketRequest = {
  mode: "latest" | "history";
  scope: string;
  instrumentId?: string;
  currencyA?: string;
  currencyB?: string;
};
export type Job = {
  current?: string;
  items?: { label: string; status: string }[];
  jobId: string;
  outcome: string;
  phase: string;
  completed: number;
  total: number;
  request: MarketRequest;
  result: {
    Items:
      | { TargetKey: string; Kind: string; Status: string; ErrorCode: string }[]
      | null;
  };
};
export type RecordInput = {
  operationId: string;
  kind: string;
  accountId: string;
  otherAccountId?: string;
  amount: string;
  currency: string;
  received?: string;
  receivedCurrency?: string;
  fee?: string;
  effectiveLocalDate?: string;
  effectiveLocalTime?: string;
  note?: string;
};
export type Review = {
  targetLabel?: string;
  before?: Activity["resulting"];
  planId: string;
  operationId?: string;
  kind?: string;
  timezone?: string;
  preview?: {
    activity: Activity;
    resulting: Activity["resulting"];
    effects: Activity["effects"];
  };
  receipt?: {
    value: string;
    currency: string;
    quoteCurrency?: string;
    effectiveDate?: string;
    quotedAt: string;
  };
  request?: MarketRequest;
  historicalPlan?: {
    EstimatedRequestCount: number;
    SnapshotWorkEstimate: number;
    Items: { Label: string; StartDate: string; EndDate: string }[];
    Unresolved?: { Code: string; Reason: string }[];
  };
};
export class MobileError extends Error {
  constructor(
    public status: number,
    public code: string,
    public field = "",
    public operationState = "unknown",
  ) {
    super(code);
  }
}
let csrf = "";
let sessionGeneration = 0;
let expired: (() => void) | undefined;
export function setMobileSession(session: Session | null) {
  sessionGeneration++;
  csrf = session?.csrfToken ?? "";
}
export function onSessionExpired(callback: () => void) {
  expired = callback;
  return () => {
    expired = undefined;
  };
}
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const requestGeneration = sessionGeneration;
  const response = await fetch(`/mobile-api/v1${path}`, {
    method,
    credentials: "same-origin",
    cache: "no-store",
    headers: {
      "Content-Type": "application/json",
      "X-Nestworth-Mobile": "1",
      ...(csrf ? { "X-CSRF-Token": csrf } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const payload = await response.json();
  if (requestGeneration !== sessionGeneration && path !== "/session")
    throw new MobileError(409, "session_changed");
  if (!response.ok) {
    if (response.status === 401 && requestGeneration === sessionGeneration)
      expired?.();
    throw new MobileError(
      response.status,
      payload.error?.code ?? "unavailable",
      payload.error?.field,
      payload.error?.operationState,
    );
  }
  return payload as T;
}
const pendingKey = "nestworth.mobile.pending.v1";
export type Pending = { operationId: string; householdId: string };
export function pendingOperation(): Pending | null {
  try {
    const p = JSON.parse(sessionStorage.getItem(pendingKey) ?? "null");
    return p &&
      typeof p.operationId === "string" &&
      typeof p.householdId === "string"
      ? p
      : null;
  } catch {
    return null;
  }
}
export function savePending(p: Pending | null) {
  if (p) sessionStorage.setItem(pendingKey, JSON.stringify(p));
  else sessionStorage.removeItem(pendingKey);
}
