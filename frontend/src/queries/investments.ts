import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service as InstrumentService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument";
import type { InstrumentRequest } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument/models";
import { Service as HoldingService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding";
import type { CreateHoldingRequest } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding/models";
import { Service as QuoteService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote";
import { callService } from "@/lib/wails";
import { overviewQueryKey } from "@/queries/portfolio";

export function useInstruments(includeArchived = false) {
  return useQuery({
    queryKey: ["instruments", includeArchived],
    queryFn: () => callService(() => InstrumentService.ListInstruments(includeArchived)),
  });
}

export function useCreateInstrument() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (request: InstrumentRequest) => callService(() => InstrumentService.CreateInstrument(request)),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["instruments"] }),
  });
}

export function useArchiveInstrument() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, archived }: { id: string; archived: boolean }) => callService(() => InstrumentService.ArchiveInstrument(id, archived)),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["instruments"] }),
  });
}

export function useHoldingsByAccounts(accountIds: string[]) {
  return useQuery({
    queryKey: ["holdings", "byAccounts", accountIds],
    queryFn: () => callService(() => HoldingService.HoldingsByAccounts(accountIds)),
    enabled: accountIds.length > 0,
  });
}

function invalidateHoldingData(queryClient: ReturnType<typeof useQueryClient>) {
  void queryClient.invalidateQueries({ queryKey: ["holdings"] });
  void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
}

export function useCreateHolding() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (request: CreateHoldingRequest) => callService(() => HoldingService.CreateHolding(request)),
    onSuccess: () => invalidateHoldingData(queryClient),
  });
}

export function useArchiveHolding() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, archived }: { id: string; archived: boolean }) => callService(() => HoldingService.ArchiveHolding(id, archived)),
    onSuccess: () => invalidateHoldingData(queryClient),
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
    queryKey: ["quote", "instrument", instrumentId],
    queryFn: () => callService(() => QuoteService.CurrentInstrumentQuote(instrumentId)),
    enabled: Boolean(instrumentId),
  });
}

export function useSaveManualInstrumentQuote() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ instrumentId, unitPrice, quotedAt }: { instrumentId: string; unitPrice: string; quotedAt: string }) =>
      callService(() => QuoteService.SaveManualInstrumentQuote(instrumentId, unitPrice, quotedAt)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["quote"] });
      invalidateHoldingData(queryClient);
    },
  });
}
