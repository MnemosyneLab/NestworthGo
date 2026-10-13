import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity";
import type { SaveInvestmentReviewPolicyRequest, SetInvestmentReviewEnrollmentRequest } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity/models";
import { callService } from "@/lib/wails";
import { queryKeys } from "./keys";

export const investmentReviewKey = [...queryKeys.liquidity.all, "investment-review", "v1"] as const;
export function useInvestmentReviews(active = true) {
  return useQuery({
    queryKey: investmentReviewKey,
    queryFn: () => callService(() => Service.InvestmentReviews()),
    enabled: active,
    refetchOnMount: "always",
    refetchOnWindowFocus: true,
    // Evaluation is local and foreground-only. Valuation/quote invalidation
    // shares the liquidity prefix, so refresh/import re-evaluates this view.
    refetchInterval: active ? 60_000 : false,
    refetchIntervalInBackground: false,
  });
}
export function useInvestmentReviewActions() {
  const client = useQueryClient();
  const updated = () => { void client.invalidateQueries({ queryKey: investmentReviewKey }); };
  const save = useMutation({
    mutationFn: (request: SaveInvestmentReviewPolicyRequest) => callService(() => Service.SaveInvestmentReviewPolicy(request)),
    onSuccess: updated,
  });
  const acknowledge = useMutation({
    mutationFn: (episodeId: string) => callService(() => Service.AcknowledgeInvestmentReview(episodeId)),
    onSuccess: updated,
  });
  const enroll = useMutation({
    mutationFn: (request: SetInvestmentReviewEnrollmentRequest) => callService(() => Service.SetInvestmentReviewEnrollment(request)),
    onSuccess: updated,
  });
  return { save, acknowledge, enroll };
}
