import { keepPreviousData, useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useId, useMemo, useRef, useState } from "react";
import { Service as LiquidityService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity";
import type {
  AppendProductValuationRequest,
  ListProductsRequest,
  OverviewRequest,
  ProductCommandRequest,
  ProductOperationPageDTO,
  RecordProductOperationRequest,
  ReleaseReservationRequest,
  ResetPolicyRequest,
  SavePolicyRequest,
  SaveReservationRequest,
  UpdateProductTermsRequest,
} from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import { callService } from "@/lib/wails";
import { callCancellableService } from "@/lib/wailsCancellation";
import { queryKeys } from "@/queries/keys";
import { invalidateActivityChange, invalidateLiquidityReads, invalidateInstrumentReads, invalidateInstrumentChange } from "@/queries/invalidation";

export function useLiquidityOverview(request: OverviewRequest, enabled = true) {
  const customHorizonOn = request.customHorizonOn ?? "";
  return useQuery({
    queryKey: queryKeys.liquidity.overview(customHorizonOn, request.includeEarlyWithdrawal),
    queryFn: () => callService(() => LiquidityService.Overview({
      customHorizonOn: request.customHorizonOn ?? undefined,
      includeEarlyWithdrawal: request.includeEarlyWithdrawal,
    })),
    enabled,
    placeholderData: keepPreviousData,
    refetchOnWindowFocus: true,
    refetchInterval: enabled ? 60_000 : false,
    refetchIntervalInBackground: false,
  });
}

export function useUpcomingActions(isActive = true) {
  return useQuery({
    queryKey: queryKeys.liquidity.upcomingActions,
    queryFn: () => callService(() => LiquidityService.UpcomingActions()),
    enabled: isActive,
    refetchOnMount: "always",
    refetchOnWindowFocus: true,
    refetchInterval: isActive ? 60_000 : false,
    refetchIntervalInBackground: false,
  });
}

export function useProducts(request: ListProductsRequest, enabled = true) {
  return useQuery({
    queryKey: queryKeys.liquidity.products(request.accountId ?? "", request.includeClosed),
    queryFn: () => callService(() => LiquidityService.ListProducts(request)),
    enabled,
  });
}

export function useProduct(productId: string, enabled = true) {
  return useQuery({
    queryKey: queryKeys.liquidity.product(productId),
    queryFn: () => callService(() => LiquidityService.Product(productId)),
    enabled: enabled && Boolean(productId),
  });
}

// Each observer owns its page chain. Invalidation and reopening begin at one page;
// static queries prevent TanStack from automatically refetching every loaded page.
export function useProductOperations(productId: string, enabled = true) {
  const client = useQueryClient();
  const owner = useId();
  const [session, setSession] = useState({ productId, enabled, revision: 0 });
  if (session.productId !== productId || session.enabled !== enabled) {
    setSession({ productId, enabled, revision: session.revision + 1 });
  }
  const queryKey = useMemo(() => queryKeys.liquidity.operationSession(productId, owner, session.revision), [productId, owner, session.revision]);
  const refresh = useCallback(() => setSession(current => ({ ...current, revision: current.revision + 1 })), []);
  useEffect(() => {
    const unsubscribe = client.getQueryCache().subscribe(event => {
      if (event.type === "updated" && event.action.type === "invalidate" && event.query === client.getQueryCache().find({ queryKey, exact: true })) refresh();
    });
    if (!enabled) void client.cancelQueries({ queryKey, exact: true });
    return () => { unsubscribe(); void client.cancelQueries({ queryKey, exact: true }); };
  }, [client, enabled, queryKey, refresh]);
  const query = useInfiniteQuery({
    queryKey,
    queryFn: async ({ pageParam, signal }) => {
      const page = await callCancellableService(signal, () => LiquidityService.ListOperations({ productId, limit: 25, ...(pageParam ? { cursor: pageParam } : {}) }));
      const first = client.getQueryData<{ pages: ProductOperationPageDTO[] }>(queryKey)?.pages[0];
      if (pageParam && first && page.latestRecordedOperationId !== first.latestRecordedOperationId) {
        refresh();
        throw new Error("Product history changed; refresh the first page");
      }
      return page;
    },
    initialPageParam: undefined as string | undefined,
    getNextPageParam: page => page.next ?? undefined,
    enabled: enabled && Boolean(productId),
    staleTime: "static",
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const nextPending = useRef<typeof queryKey | null>(null);
  const loadMore = async () => {
    if (nextPending.current === queryKey || query.isFetching || !query.hasNextPage) return;
    nextPending.current = queryKey;
    try { await query.fetchNextPage({ cancelRefetch: false }); }
    finally { if (nextPending.current === queryKey) nextPending.current = null; }
  };
  return { ...query, refresh, loadMore, sessionKey: queryKey };
}

export function useSavePolicy() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (request: SavePolicyRequest) => callService(() => LiquidityService.SavePolicy(request)),
    onSuccess: () => invalidateLiquidityReads(queryClient),
  });
}

export function useResetPolicy() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (request: ResetPolicyRequest) => callService(() => LiquidityService.ResetPolicy(request)),
    onSuccess: () => invalidateLiquidityReads(queryClient),
  });
}

export function useSaveReservation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (request: SaveReservationRequest) => callService(() => LiquidityService.SaveReservation(request)),
    onSuccess: () => invalidateLiquidityReads(queryClient),
  });
}

export function useReleaseReservation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (request: ReleaseReservationRequest) => callService(() => LiquidityService.ReleaseReservation(request)),
    onSuccess: () => invalidateLiquidityReads(queryClient),
  });
}

export function useUpdateProductTerms() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (request: UpdateProductTermsRequest) => callService(() => LiquidityService.UpdateProductTerms(request)),
    onSuccess: (detail) => { invalidateLiquidityReads(queryClient); invalidateInstrumentChange(queryClient, detail.product.instrumentId); },
  });
}

export function useAppendProductValuation() {
  const queryClient = useQueryClient();
  const slot = useRef<{ payload: string; id: string } | null>(null);
  return useMutation({
    mutationFn: (request: AppendProductValuationRequest) => {
      const payload = JSON.stringify({ ...request, mutationId: "" });
      let current = slot.current;
      if (!current || current.payload !== payload) {
        current = { payload, id: request.mutationId.trim() || crypto.randomUUID() };
        slot.current = current;
      }
      return callService(() => LiquidityService.AppendProductValuation({ ...request, mutationId: current.id }));
    },
    onSuccess: () => {
      slot.current = null;
      invalidateActivityChange(queryClient);
    },
  });
}

export function usePreviewProductOperation() {
  return useMutation({
    mutationFn: (request: ProductCommandRequest) => callService(() => LiquidityService.PreviewProductOperation(request)),
  });
}

export function useRecordProductOperation() {
  const queryClient = useQueryClient();
  const slot = useRef<{ payload: string; id: string } | null>(null);
  return useMutation({
    mutationFn: (request: RecordProductOperationRequest) => {
      const payload = JSON.stringify({ ...request, mutationId: "" });
      let current = slot.current;
      if (!current || current.payload !== payload) {
        current = { payload, id: request.mutationId.trim() || crypto.randomUUID() };
        slot.current = current;
      }
      return callService(() => LiquidityService.RecordProductOperation({ ...request, mutationId: current.id }));
    },
    onSuccess: () => {
      slot.current = null;
      invalidateActivityChange(queryClient);
      invalidateLiquidityReads(queryClient);
      invalidateInstrumentReads(queryClient);
    },
  });
}

export function useReservations(enabled = true) {
  return useQuery({ queryKey: [...queryKeys.liquidity.all,"reservations"], queryFn: () => callService(() => LiquidityService.ListReservations()), enabled });
}
