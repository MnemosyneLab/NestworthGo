import { describe, expect, it, vi } from "vitest";
import { createElement, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { act, renderHook } from "@testing-library/react";
import { queryKeys } from "@/queries/keys";
import {
  invalidateCurrentValuation,
  invalidateHistoryReads,
  invalidateHoldingChange,
  invalidateInstrumentReads,
  invalidateRefreshAll,
  invalidateRequiredFX,
} from "@/queries/invalidation";
import { useArchiveAccount, useCreateAccount, useUpdateAccount } from "@/queries/accounts";
import { useArchiveInstrument, useCreateHolding, useCreateInstrument, useSaveManualInstrumentQuote } from "@/queries/investments";
import { useFixChange, useRecordChange, useStartHistory, useUndoChange } from "@/queries/history";
import { useRefreshAll, useRefreshRequiredFX } from "@/queries/marketdata";

vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: {
    CreateAccount: vi.fn(async () => ({ account: { id: "account-1" } })),
    UpdateAccount: vi.fn(async () => ({ account: { id: "account-1" } })),
    ArchiveAccount: vi.fn(async () => undefined),
  },
}));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding", () => ({
  Service: {
    CreateHolding: vi.fn(async () => ({ id: "holding-1", accountId: "account-1" })),
  },
}));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({
  Service: {
    CreateInstrument: vi.fn(async () => ({ id: "instrument-1" })),
    ArchiveInstrument: vi.fn(async () => undefined),
  },
}));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote", () => ({
  Service: {
    SaveManualInstrumentQuote: vi.fn(async () => undefined),
  },
}));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({
  Service: {
    StartHistory: vi.fn(async () => ({ id: "origin-1" })),
    RecordChange: vi.fn(async () => ({ activity: { id: "a1" }, effects: [], resulting: [] })),
    UndoChange: vi.fn(async () => ({ activity: { id: "a2" }, effects: [], resulting: [] })),
    FixChange: vi.fn(async () => ({ activity: { id: "a1" }, effects: [], resulting: [] })),
  },
}));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata", () => ({
  Service: {
    RefreshAll: vi.fn(async () => ({})),
    RefreshRequiredFX: vi.fn(async () => ({})),
  },
}));

function clientWith(...keys: readonly (readonly unknown[])[]) {
  const queryClient = createTestQueryClient();
  for (const key of keys) {
    queryClient.setQueryData(key, "stale");
  }
  return queryClient;
}

function isInvalidated(queryClient: QueryClient, key: readonly unknown[]) {
  return queryClient.getQueryState(key)?.isInvalidated ?? false;
}

describe("query keys and dependency invalidation", () => {
  it("normalizes account filters and account ID lists before key construction", () => {
    expect(queryKeys.accounts.list()).toEqual(queryKeys.accounts.list({ includeArchived: false }));
    expect(queryKeys.holdings.byAccounts(["account-b", "account-a", "account-a"])).toEqual(
      queryKeys.holdings.byAccounts(["account-a", "account-b"]),
    );
  });

  it("keeps Start History inside the History namespace", () => {
    const queryClient = clientWith(
      queryKeys.history.origin,
      queryKeys.history.activities(50),
      queryKeys.overview.all,
      queryKeys.accounts.list(),
    );

    invalidateHistoryReads(queryClient);

    expect(isInvalidated(queryClient, queryKeys.history.origin)).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.history.activities(50))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.overview.all)).toBe(false);
    expect(isInvalidated(queryClient, queryKeys.accounts.list())).toBe(false);
  });

  it("does not refetch valuation when an Instrument is created or archived", () => {
    const queryClient = clientWith(
      queryKeys.instruments.list(),
      queryKeys.instruments.list(true),
      queryKeys.overview.all,
      queryKeys.quote.instrument.current("instrument-1"),
    );

    invalidateInstrumentReads(queryClient);

    expect(isInvalidated(queryClient, queryKeys.instruments.list())).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.instruments.list(true))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.overview.all)).toBe(false);
    expect(isInvalidated(queryClient, queryKeys.quote.instrument.current("instrument-1"))).toBe(false);
  });

  it("refreshes Overview and current gains for FX-only refresh, not raw quote or holding lists", () => {
    const queryClient = clientWith(
      queryKeys.overview.all,
      queryKeys.analytics.accountGain.current("account-1"),
      queryKeys.analytics.accountGain.current("account-2"),
      queryKeys.quote.instrument.current("instrument-1"),
      queryKeys.holdings.byAccounts(["account-1"]),
    );

    invalidateRequiredFX(queryClient);

    expect(isInvalidated(queryClient, queryKeys.overview.all)).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-1"))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-2"))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.quote.instrument.current("instrument-1"))).toBe(false);
    expect(isInvalidated(queryClient, queryKeys.holdings.byAccounts(["account-1"]))).toBe(false);
  });

  it("refreshes quote and valuation for Refresh All, not raw holdings", () => {
    const queryClient = clientWith(
      queryKeys.overview.all,
      queryKeys.quote.instrument.current("instrument-1"),
      queryKeys.analytics.accountGain.current("account-1"),
      queryKeys.holdings.byAccounts(["account-1"]),
    );

    invalidateRefreshAll(queryClient);

    expect(isInvalidated(queryClient, queryKeys.overview.all)).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.quote.instrument.current("instrument-1"))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-1"))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.holdings.byAccounts(["account-1"]))).toBe(false);
  });

  it("invalidates only the affected account gain for a holding change when its account is known", () => {
    const queryClient = clientWith(
      queryKeys.holdings.byAccounts(["account-1"]),
      queryKeys.overview.all,
      queryKeys.analytics.accountGain.current("account-1"),
      queryKeys.analytics.accountGain.current("account-2"),
    );

    invalidateHoldingChange(queryClient, "account-1");

    expect(isInvalidated(queryClient, queryKeys.holdings.byAccounts(["account-1"]))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.overview.all)).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-1"))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-2"))).toBe(false);
  });

  it("falls back to the account-gain prefix when affected accounts are unknown", () => {
    const queryClient = clientWith(
      queryKeys.overview.all,
      queryKeys.analytics.accountGain.current("account-1"),
      queryKeys.analytics.accountGain.current("account-2"),
    );

    invalidateCurrentValuation(queryClient);

    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-1"))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-2"))).toBe(true);
  });
});

function seededClient() {
  return clientWith(
    queryKeys.accounts.list(),
    queryKeys.overview.all,
    queryKeys.analytics.accountGain.current("account-1"),
    queryKeys.analytics.accountGain.current("account-2"),
    queryKeys.holdings.byAccounts(["account-1"]),
    queryKeys.instruments.list(),
    queryKeys.quote.instrument.current("instrument-1"),
    queryKeys.history.origin,
    queryKeys.history.activities(50),
  );
}

function renderMutation<T>(queryClient: QueryClient, hook: () => T) {
  return renderHook(hook, {
    wrapper: ({ children }: { children: ReactNode }) => createElement(QueryClientProvider, { client: queryClient }, children),
  });
}

describe("mutation-hook invalidation", () => {
  it("invalidates accounts and valuation after account create/update/archive", async () => {
    const queryClient = seededClient();
    const create = renderMutation(queryClient, useCreateAccount);
    await act(async () => {
      await create.result.current.mutateAsync({ name: "Cash" } as never);
    });
    expect(isInvalidated(queryClient, queryKeys.accounts.list())).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.overview.all)).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.history.origin)).toBe(false);

    const updateClient = seededClient();
    const update = renderMutation(updateClient, useUpdateAccount);
    await act(async () => {
      await update.result.current.mutateAsync({ id: "account-1", request: { name: "Cash" } as never });
    });
    expect(isInvalidated(updateClient, queryKeys.analytics.accountGain.current("account-1"))).toBe(true);
    expect(isInvalidated(updateClient, queryKeys.analytics.accountGain.current("account-2"))).toBe(false);
    expect(isInvalidated(updateClient, queryKeys.history.origin)).toBe(false);

    const archiveClient = seededClient();
    const archive = renderMutation(archiveClient, useArchiveAccount);
    await act(async () => {
      await archive.result.current.mutateAsync({ id: "account-1", archived: true });
    });
    expect(isInvalidated(archiveClient, queryKeys.accounts.list())).toBe(true);
    expect(isInvalidated(archiveClient, queryKeys.history.origin)).toBe(false);
  });

  it("invalidates holdings and valuation after holding create, not History", async () => {
    const queryClient = seededClient();
    const create = renderMutation(queryClient, useCreateHolding);
    await act(async () => {
      await create.result.current.mutateAsync({ accountId: "account-1", instrumentId: "instrument-1", quantity: "1" } as never);
    });
    expect(isInvalidated(queryClient, queryKeys.holdings.byAccounts(["account-1"]))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.overview.all)).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-1"))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.history.origin)).toBe(false);
  });

  it("invalidates History, holdings, and valuation after record/fix/undo", async () => {
    const queryClient = seededClient();
    const record = renderMutation(queryClient, useRecordChange);
    await act(async () => {
      await record.result.current.mutateAsync({ kind: "money_added", accountId: "account-1" } as never);
    });
    expect(isInvalidated(queryClient, queryKeys.history.origin)).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.overview.all)).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-1"))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.analytics.accountGain.current("account-2"))).toBe(false);

    const undoClient = seededClient();
    const undo = renderMutation(undoClient, useUndoChange);
    await act(async () => {
      await undo.result.current.mutateAsync("activity-1");
    });
    expect(isInvalidated(undoClient, queryKeys.history.origin)).toBe(true);
    expect(isInvalidated(undoClient, queryKeys.analytics.accountGain.current("account-1"))).toBe(true);

    const fixClient = seededClient();
    const fix = renderMutation(fixClient, useFixChange);
    await act(async () => {
      await fix.result.current.mutateAsync({ activityId: "activity-1", replacement: { kind: "money_added", accountId: "account-1" } as never });
    });
    expect(isInvalidated(fixClient, queryKeys.history.origin)).toBe(true);
    expect(isInvalidated(fixClient, queryKeys.analytics.accountGain.current("account-1"))).toBe(true);
  });

  it("invalidates quote and valuation after a manual quote, not raw holdings or History", async () => {
    const queryClient = seededClient();
    const save = renderMutation(queryClient, useSaveManualInstrumentQuote);
    await act(async () => {
      await save.result.current.mutateAsync({ instrumentId: "instrument-1", unitPrice: "1", quotedAt: "" });
    });
    expect(isInvalidated(queryClient, queryKeys.quote.instrument.current("instrument-1"))).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.overview.all)).toBe(true);
    expect(isInvalidated(queryClient, queryKeys.holdings.byAccounts(["account-1"]))).toBe(false);
    expect(isInvalidated(queryClient, queryKeys.history.origin)).toBe(false);
  });

  it("keeps Start History inside History and identity-only instrument writes off valuation", async () => {
    const historyClient = seededClient();
    const start = renderMutation(historyClient, useStartHistory);
    await act(async () => {
      await start.result.current.mutateAsync("UTC");
    });
    expect(isInvalidated(historyClient, queryKeys.history.origin)).toBe(true);
    expect(isInvalidated(historyClient, queryKeys.accounts.list())).toBe(false);
    expect(isInvalidated(historyClient, queryKeys.overview.all)).toBe(false);

    const instrumentClient = seededClient();
    const create = renderMutation(instrumentClient, useCreateInstrument);
    await act(async () => {
      await create.result.current.mutateAsync({ name: "ETF" } as never);
    });
    expect(isInvalidated(instrumentClient, queryKeys.instruments.list())).toBe(true);
    expect(isInvalidated(instrumentClient, queryKeys.overview.all)).toBe(false);

    const archiveClient = seededClient();
    const archive = renderMutation(archiveClient, useArchiveInstrument);
    await act(async () => {
      await archive.result.current.mutateAsync({ id: "instrument-1", archived: true });
    });
    expect(isInvalidated(archiveClient, queryKeys.instruments.list())).toBe(true);
    expect(isInvalidated(archiveClient, queryKeys.overview.all)).toBe(false);
  });

  it("refreshes FX-only and Refresh All through their mutation hooks", async () => {
    const fxClient = seededClient();
    const fx = renderMutation(fxClient, useRefreshRequiredFX);
    await act(async () => {
      await fx.result.current.mutateAsync();
    });
    expect(isInvalidated(fxClient, queryKeys.overview.all)).toBe(true);
    expect(isInvalidated(fxClient, queryKeys.quote.instrument.current("instrument-1"))).toBe(false);
    expect(isInvalidated(fxClient, queryKeys.holdings.byAccounts(["account-1"]))).toBe(false);

    const refreshClient = seededClient();
    const refresh = renderMutation(refreshClient, useRefreshAll);
    await act(async () => {
      await refresh.result.current.mutateAsync();
    });
    expect(isInvalidated(refreshClient, queryKeys.overview.all)).toBe(true);
    expect(isInvalidated(refreshClient, queryKeys.quote.instrument.current("instrument-1"))).toBe(true);
    expect(isInvalidated(refreshClient, queryKeys.holdings.byAccounts(["account-1"]))).toBe(false);
  });
});
