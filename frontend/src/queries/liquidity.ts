import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef } from "react";
import { Service as LiquidityService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity";
import type {
  AppendProductValuationRequest,
  ListProductsRequest,
  OverviewRequest,
  ProductCommandRequest,
  RecordProductOperationRequest,
  ReleaseReservationRequest,
  ResetPolicyRequest,
  SavePolicyRequest,
  SaveReservationRequest,
  UpdateProductTermsRequest,
} from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";
import { invalidateActivityChange, invalidateLiquidityReads } from "@/queries/invalidation";

export function useLiquidityOverview(request: OverviewRequest, enabled = true) {
  const customHorizonOn = request.customHorizonOn ?? "";
  return useQuery({
    queryKey: queryKeys.liquidity.overview(customHorizonOn, request.includeEarlyWithdrawal),
    queryFn: () => callService(() => LiquidityService.Overview({
      customHorizonOn: request.customHorizonOn ?? undefined,
      includeEarlyWithdrawal: request.includeEarlyWithdrawal,
    })),
    enabled,
    refetchOnWindowFocus: true,
    refetchInterval: 60_000,
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

export function useProductOperations(productId: string, enabled = true) {
  return useQuery({
    queryKey: queryKeys.liquidity.operations(productId),
    queryFn: async () => {
      const first = await callService(() => LiquidityService.ListOperations({ productId, limit: 25 }));
      const operations = [...(first.operations ?? [])];
      let cursor = first.next;
      while (cursor) {
        const nextCursor = cursor;
        const page = await callService(() => LiquidityService.ListOperations({ productId, limit: 25, cursor: nextCursor }));
        operations.push(...(page.operations ?? []));
        cursor = page.next;
      }
      return { ...first, operations, next: null };
    },
    enabled: enabled && Boolean(productId),
  });
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
    onSuccess: () => invalidateLiquidityReads(queryClient),
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
    },
  });
}

export function useReservations(enabled = true) {
  return useQuery({ queryKey: [...queryKeys.liquidity.all,"reservations"], queryFn: () => callService(() => LiquidityService.ListReservations()), enabled });
}
