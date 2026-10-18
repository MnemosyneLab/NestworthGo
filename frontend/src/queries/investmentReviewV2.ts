import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity";
import type { ReviewAlertFilter, ReviewCoverageFilter } from "../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import type { SaveReviewRuleV2Input, SetReviewSuppressionV2Input } from "../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import { callService } from "@/lib/wails";
import { queryKeys } from "@/queries/keys";

export const reviewV2Key = queryKeys.reviewV2.all;

export function useReviewAlertPage(filter: ReviewAlertFilter, cursor = "", active = true) {
  return useQuery({
    queryKey: [...reviewV2Key, "alerts", filter, cursor],
    queryFn: () => callService(() => Service.ListReviewAlertsV2({ filter, cursor, pageSize: 50 })),
    enabled: active,
    refetchOnMount: "always",
  });
}

export function useReviewDailyEvidencePage(episodeId: string, cursor = "", active = true) {
  return useQuery({
    queryKey: [...reviewV2Key, "evidence", episodeId, cursor],
    queryFn: () => callService(() => Service.ListReviewDailyEvidenceV2({ episodeId, cursor, pageSize: 20 })),
    enabled: active && episodeId !== "",
  });
}

export function useReviewEpisodeHistoryPage(ruleId: string, cursor = "", active = true) {
  return useQuery({
    queryKey: [...reviewV2Key, "episode-history", ruleId, cursor],
    queryFn: () => callService(() => Service.ListReviewEpisodeHistoryV2({ ruleId, cursor, pageSize: 20 })),
    enabled: active && ruleId !== "",
  });
}

export function useReviewEpisodeEventPage(episodeId: string, cursor = "", active = true) {
  return useQuery({
    queryKey: [...reviewV2Key, "episode-events", episodeId, cursor],
    queryFn: () => callService(() => Service.ListReviewEpisodeEventsV2({ episodeId, cursor, pageSize: 20 })),
    enabled: active && episodeId !== "",
  });
}

export function useReviewCoveragePage(filter: ReviewCoverageFilter, cursor = "", active = true) {
  return useQuery({
    queryKey: [...reviewV2Key, "coverage", filter, cursor],
    queryFn: () => callService(() => Service.ListReviewCoverageV2({ filter, cursor, pageSize: 50 })),
    enabled: active,
    refetchOnMount: "always",
  });
}

export function useReviewRulesV2(active = true) {
  const query = useInfiniteQuery({
    queryKey: [...reviewV2Key, "rules"],
    queryFn: ({ pageParam }) => callService(() => Service.ListReviewRulesV2({ cursor: pageParam, pageSize: 100 })),
    initialPageParam: "",
    getNextPageParam: last => last?.length === 100 ? last[last.length - 1].id : undefined,
    enabled: active,
  });
  return { ...query, data: query.data?.pages.flatMap(page => page ?? []) };
}

export function useReviewSuppressionsV2(active = true) {
  const query = useInfiniteQuery({
    queryKey: [...reviewV2Key, "suppressions"],
    queryFn: ({ pageParam }) => callService(() => Service.ListReviewSuppressionsV2({ cursor: pageParam, pageSize: 100 })),
    initialPageParam: "",
    getNextPageParam: last => last?.length === 100 ? last[last.length - 1].ID : undefined,
    enabled: active,
  });
  return { ...query, data: query.data?.pages.flatMap(page => page ?? []) };
}

export function useReviewV2Commands() {
  const client = useQueryClient();
  const invalidate = () => { void client.invalidateQueries({ queryKey: reviewV2Key }); };
  const acknowledge = useMutation({
    mutationFn: (request: { episodeId: string; expectedRevision: number }) => callService(() => Service.AcknowledgeReviewEpisodeV2({ ...request, mutationId: crypto.randomUUID() })),
    onSuccess: invalidate,
  });
  const suppress = useMutation({
    mutationFn: (request: Omit<SetReviewSuppressionV2Input, "mutationId" | "suppressionId" | "expectedRevision">) => callService(() => Service.SetReviewSuppressionV2({ ...request, mutationId: crypto.randomUUID(), suppressionId: crypto.randomUUID(), expectedRevision: 0 })),
    onSuccess: invalidate,
  });
  const clearSuppression = useMutation({
    mutationFn: (request: { suppressionId: string; expectedRevision: number }) => callService(() => Service.ClearReviewSuppressionV2({ ...request, mutationId: crypto.randomUUID() })),
    onSuccess: invalidate,
  });
  const reconcile = useMutation({
    mutationFn: () => callService(() => Service.ReconcileInvestmentReviewsV2()),
    onSuccess: invalidate,
  });
  const createRule = useMutation({
    mutationFn: (request: Omit<SaveReviewRuleV2Input, "mutationId" | "ruleId" | "expectedRevision">) => callService(() => Service.CreateReviewRuleV2({ ...request, ruleId: crypto.randomUUID(), mutationId: crypto.randomUUID(), expectedRevision: 0 })),
    onSuccess: invalidate,
  });
  const updateRule = useMutation({
    mutationFn: (request: Omit<SaveReviewRuleV2Input, "mutationId">) => callService(() => Service.UpdateReviewRuleV2({ ...request, mutationId: crypto.randomUUID() })),
    onSuccess: invalidate,
  });
  const archiveRule = useMutation({
    mutationFn: (request: { ruleId: string; expectedRevision: number }) => callService(() => Service.ArchiveReviewRuleV2({ ...request, mutationId: crypto.randomUUID() })),
    onSuccess: invalidate,
  });
  return { acknowledge, suppress, clearSuppression, reconcile, createRule, updateRule, archiveRule };
}
