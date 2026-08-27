import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service as InstrumentService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument";
import type { InstrumentRequest } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument/models";
import { Service as HoldingService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding";
import type { CreateHoldingRequest } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding/models";
import { Service as QuoteService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote";
import { callService } from "@/lib/wails";
import { normalizeAccountIds, queryKeys } from "@/queries/keys";
import {
  invalidateHoldingChange,
  invalidateInstrumentQuoteChange,
  invalidateInstrumentReads,
  invalidateRequiredFX,
} from "@/queries/invalidation";

export function useInstruments(includeArchived = false) {
  const normalizedIncludeArchived = Boolean(includeArchived);
  return useQuery({
    queryKey: queryKeys.instruments.list(normalizedIncludeArchived),
    queryFn: () => callService(() => InstrumentService.ListInstruments(normalizedIncludeArchived)),
  });
}

export function useCreateInstrument() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (request: InstrumentRequest) => callService(() => InstrumentService.CreateInstrument(request)),
    onSuccess: () => invalidateInstrumentReads(queryClient),
  });
}

export function useArchiveInstrument() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, archived }: { id: string; archived: boolean }) => callService(() => InstrumentService.ArchiveInstrument(id, archived)),
    onSuccess: () => invalidateInstrumentReads(queryClient),
  });
}

export function useHoldingsByAccounts(accountIds: string[]) {
  const normalizedAccountIds = normalizeAccountIds(accountIds);
  return useQuery({
    queryKey: queryKeys.holdings.byAccounts(normalizedAccountIds),
    queryFn: () => callService(() => HoldingService.HoldingsByAccounts(normalizedAccountIds)),
    enabled: normalizedAccountIds.length > 0,
  });
}

export function useCreateHolding() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (request: CreateHoldingRequest) => callService(() => HoldingService.CreateHolding(request)),
    onSuccess: (_data, variables) => invalidateHoldingChange(queryClient, variables.accountId),
  });
}

/**
 * useAllHoldingsFlat denormalizes every Holding across every Holdings-mode
 * Account into one flat list with instrument/account names attached, for
 * simple <select> pickers (Record change's position_transfer/
 * position_adjustment/trade forms). It composes useAccounts +
 * useHoldingsByAccounts + useInstruments rather than adding a new Go
 * binding, since all three are already loaded elsewhere in the app.
 */
export function useAllHoldingsFlat(accountIds: string[]) {
  const holdingsByAccount = useHoldingsByAccounts(accountIds);
  const instruments = useInstruments();
  const instrumentNameById = new Map((instruments.data ?? []).map((instrument) => [instrument.id, instrument.name]));
  const flat = Object.entries(holdingsByAccount.data ?? {}).flatMap(([accId, holdings]) =>
    (holdings ?? [])
      .filter((holding) => !holding.archivedAt)
      .map((holding) => ({ ...holding, accountId: accId, instrumentName: instrumentNameById.get(holding.instrumentId) })),
  );
  return {
    data: flat,
    isLoading: holdingsByAccount.isLoading || instruments.isLoading,
    isError: holdingsByAccount.isError || instruments.isError,
    refetch: () => Promise.all([holdingsByAccount.refetch(), instruments.refetch()]),
  };
}

export function useCurrentInstrumentQuote(instrumentId: string) {
  return useQuery({
    queryKey: queryKeys.quote.instrument.current(instrumentId),
    queryFn: () => callService(() => QuoteService.CurrentInstrumentQuote(instrumentId)),
    enabled: Boolean(instrumentId),
  });
}

export function useCurrentFXQuote(currencyA: string, currencyB: string) {
  return useQuery({
    queryKey: queryKeys.quote.fx.current(currencyA, currencyB),
    queryFn: () => callService(() => QuoteService.CurrentFXQuote(currencyA, currencyB)),
    enabled: Boolean(currencyA) && Boolean(currencyB),
  });
}

export function useFXPreferences() {
  return useQuery({
    queryKey: queryKeys.quote.fx.preferences,
    queryFn: () => callService(() => QuoteService.ListFXPreferences()),
  });
}

export function useSetFXPreference() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ currencyA, currencyB, source }: { currencyA: string; currencyB: string; source: string }) =>
      callService(() => QuoteService.SetFXPreference(currencyA, currencyB, source)),
    onSuccess: () => {
      invalidateRequiredFX(queryClient);
    },
  });
}

export function useSaveManualInstrumentQuote() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ instrumentId, unitPrice, quotedAt }: { instrumentId: string; unitPrice: string; quotedAt: string }) =>
      callService(() => QuoteService.SaveManualInstrumentQuote(instrumentId, unitPrice, quotedAt)),
    onSuccess: (_data, variables) => {
      invalidateInstrumentQuoteChange(queryClient, variables.instrumentId);
    },
  });
}
