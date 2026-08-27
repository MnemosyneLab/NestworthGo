import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service as AccountService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account";
import type {
  AccountFilterRequest,
  CreateAccountRequest,
  UpdateAccountRequest,
} from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account/models";
import { callService } from "@/lib/wails";
import { queryKeys, normalizeAccountFilter } from "@/queries/keys";
import { invalidateAccountChange } from "@/queries/invalidation";

export const accountsQueryKey = queryKeys.accounts.list;

export function useAccounts(filter: AccountFilterRequest = {}) {
  const normalizedFilter = normalizeAccountFilter(filter);
  return useQuery({
    queryKey: accountsQueryKey(normalizedFilter),
    queryFn: () => callService(() => AccountService.ListAccounts(normalizedFilter)),
  });
}

export function useAccountValuations(filter: AccountFilterRequest = {}) {
  const normalizedFilter = normalizeAccountFilter(filter);
  return useQuery({
    queryKey: queryKeys.accounts.valuations(normalizedFilter),
    queryFn: () => callService(() => AccountService.AccountValuations(normalizedFilter)),
  });
}

export function useCreateAccount() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (request: CreateAccountRequest) => callService(() => AccountService.CreateAccount(request)),
    onSuccess: () => invalidateAccountChange(queryClient),
  });
}

export function useUpdateAccount() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, request }: { id: string; request: UpdateAccountRequest }) =>
      callService(() => AccountService.UpdateAccount(id, request)),
    onSuccess: (_data, variables) => invalidateAccountChange(queryClient, variables.id),
  });
}

export function useArchiveAccount() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, archived }: { id: string; archived: boolean }) =>
      callService(() => AccountService.ArchiveAccount(id, archived)),
    onSuccess: (_data, variables) => invalidateAccountChange(queryClient, variables.id),
  });
}

export function useSetAccountLogo() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, mediaAssetId }: { id: string; mediaAssetId: string }) =>
      callService(() => AccountService.SetAccountLogo(id, mediaAssetId)),
    onSuccess: (_data, variables) => invalidateAccountChange(queryClient, variables.id),
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
    ownership: request.ownership,
  };
}
