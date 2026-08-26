import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service as AccountService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account";
import type {
  AccountFilterRequest,
  CreateAccountRequest,
  UpdateAccountRequest,
} from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account/models";
import { callService } from "@/lib/wails";
import { overviewQueryKey } from "@/queries/portfolio";
import { bootstrapQueryKey } from "@/queries/household";

export const accountsQueryKey = (filter: AccountFilterRequest) => ["accounts", filter] as const;

export function useAccounts(filter: AccountFilterRequest = {}) {
  return useQuery({
    queryKey: accountsQueryKey(filter),
    queryFn: () => callService(() => AccountService.ListAccounts(filter)),
  });
}

/** invalidateAccountData is shared by every Account mutation below: an
 * Account change can move Overview's net worth (frontend stack decision
 * Sec5.1's "UpdateAccountValue() -> invalidate accounts/overview/history"
 * pattern). */
function invalidateAccountData(queryClient: ReturnType<typeof useQueryClient>) {
  void queryClient.invalidateQueries({ queryKey: ["accounts"] });
  void queryClient.invalidateQueries({ queryKey: overviewQueryKey });
  void queryClient.invalidateQueries({ queryKey: bootstrapQueryKey });
}

export function useCreateAccount() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (request: CreateAccountRequest) => callService(() => AccountService.CreateAccount(request)),
    onSuccess: () => invalidateAccountData(queryClient),
  });
}

export function useUpdateAccount() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, request }: { id: string; request: UpdateAccountRequest }) =>
      callService(() => AccountService.UpdateAccount(id, request)),
    onSuccess: () => invalidateAccountData(queryClient),
  });
}

export function useArchiveAccount() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, archived }: { id: string; archived: boolean }) =>
      callService(() => AccountService.ArchiveAccount(id, archived)),
    onSuccess: () => invalidateAccountData(queryClient),
  });
}

export function useAppendAccountValue() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, amount, effectiveAt }: { id: string; amount: string; effectiveAt: string }) =>
      callService(() => AccountService.AppendAccountValue(id, amount, effectiveAt)),
    onSuccess: () => invalidateAccountData(queryClient),
  });
}

export function useSetAccountLogo() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, mediaAssetId }: { id: string; mediaAssetId: string }) =>
      callService(() => AccountService.SetAccountLogo(id, mediaAssetId)),
    onSuccess: () => invalidateAccountData(queryClient),
  });
}

export function useSetAccountIcon() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, iconKey }: { id: string; iconKey: string }) =>
      callService(() => AccountService.SetAccountIcon(id, iconKey)),
    onSuccess: () => invalidateAccountData(queryClient),
  });
}

/** toUpdateAccountRequest maps the create-form's complete state onto the
 * Set-flags UpdateAccountRequest so an edit sheet can reuse AccountForm. */
export function toUpdateAccountRequest(request: CreateAccountRequest): UpdateAccountRequest {
  return {
    name: request.name,
    primaryCategory: request.primaryCategory,
    secondaryCategory: request.secondaryCategory,
    trackingMode: request.trackingMode,
    defaultCurrency: request.defaultCurrency,
    institutionId: request.institutionId ?? "",
    institutionIdSet: true,
    groupId: request.groupId ?? "",
    groupIdSet: true,
    note: request.note,
    noteSet: request.note !== undefined,
    iconKey: request.iconKey ?? "",
    iconKeySet: Boolean(request.iconKey),
    includeInNetWorth: request.includeInNetWorth,
    includeInInvestment: request.includeInInvestment,
    includeInLiquidAssets: request.includeInLiquidAssets,
    ownerIds: request.ownerIds,
    ownershipPercentages: request.ownershipPercentages,
  };
}
