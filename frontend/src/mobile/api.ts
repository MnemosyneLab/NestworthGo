import { backendBase, ConnectionError } from "./connection";
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
  accessToken?: string;
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
  items?: {
    label: string;
    status: string;
    kind?: string;
    errorCode?: string;
    startDate?: string;
    endDate?: string;
  }[];
  source?: string;
  retryable?: boolean;
  cancellable?: boolean;
  blockers?: string[];
  errorCode?: string;
  nextEligibilityAt?: string;
  startedAt?: string;
  finishedAt?: string;
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
let accessToken = "";
const requests = new Set<AbortController>();
let sessionGeneration = 0;
let expired: (() => void) | undefined;
export function setMobileSession(session: Session | null) {
  sessionGeneration++;
  csrf = session?.csrfToken ?? "";
  accessToken = session ? (session.accessToken ?? accessToken) : "";
  for (const request of requests) request.abort();
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
  options: { signal?: AbortSignal; timeout?: number; anonymous?: boolean } = {},
): Promise<T> {
  const requestGeneration = sessionGeneration;
  const controller = new AbortController();
  requests.add(controller);
  const abort = () => controller.abort();
  options.signal?.addEventListener("abort", abort, { once: true });
  if (options.signal?.aborted) controller.abort();
  let timedOut = false;
  const timer = setTimeout(
    () => {
      timedOut = true;
      controller.abort();
    },
    options.timeout ?? (method === "GET" ? 15000 : 35000),
  );
  try {
    const response = await fetch(`${backendBase()}/mobile-api/v1${path}`, {
      method,
      credentials: backendBase() ? "omit" : "same-origin",
      cache: "no-store",
      redirect: "error",
      referrerPolicy: "no-referrer",
      signal: controller.signal,
      headers: {
        "Content-Type": "application/json",
        "X-Nestworth-Mobile": "1",
        ...(!options.anonymous && csrf ? { "X-CSRF-Token": csrf } : {}),
        ...(!options.anonymous && accessToken
          ? { Authorization: `Bearer ${accessToken}` }
          : {}),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    let payload;
    try {
      payload = await response.json();
    } catch {
      throw new ConnectionError("invalid_response");
    }
    if (controller.signal.aborted)
      throw new ConnectionError(timedOut ? "timeout" : "cancelled");
    if (requestGeneration !== sessionGeneration)
      throw new MobileError(409, "session_changed");
    if (!response.ok) {
      if (response.status === 401 && !options.anonymous && path !== "/session")
        expired?.();
      throw new MobileError(
        response.status,
        payload.error?.code ?? "unavailable",
        payload.error?.field,
        payload.error?.operationState,
      );
    }
    return payload as T;
  } catch (error) {
    if (requestGeneration !== sessionGeneration)
      throw new MobileError(409, "session_changed");
    if (controller.signal.aborted)
      throw new ConnectionError(timedOut ? "timeout" : "cancelled");
    if (error instanceof MobileError || error instanceof ConnectionError)
      throw error;
    throw new ConnectionError("transport");
  } finally {
    clearTimeout(timer);
    requests.delete(controller);
    options.signal?.removeEventListener("abort", abort);
  }
}
const pendingKey = "nestworth.mobile.pending.v1";
const recoveryKey = () =>
  backendBase() ? `${pendingKey}:${backendBase()}` : pendingKey;
export type Pending = { operationId: string; householdId: string };
export function pendingOperation(): Pending | null {
  try {
    const p = JSON.parse(sessionStorage.getItem(recoveryKey()) ?? "null");
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
  try {
    if (p) sessionStorage.setItem(recoveryKey(), JSON.stringify(p));
    else sessionStorage.removeItem(recoveryKey());
    return true;
  } catch {
    return false;
  }
}
