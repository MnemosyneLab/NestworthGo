import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Service as AccountService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account";
import type {
  AccountFilterRequest,
  CreateAccountRequest,
  UpdateAccountRequest,
} from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account/models";
import { Service as HoldingService } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/holding";
import { callService } from "@/lib/wails";
import { queryKeys, normalizeAccountFilter } from "@/queries/keys";
import { invalidateAccountChange, invalidateActivityChange } from "@/queries/invalidation";

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

export function useAccountCashValues(accountId: string, enabled = true) {
  return useQuery({
    queryKey: [...queryKeys.accounts.all, "cashValues", accountId] as const,
    queryFn: () => callService(() => HoldingService.ListAccountCashValues(accountId)),
    enabled: enabled && Boolean(accountId),
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

export function useSetAccountIcon() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, iconKey }: { id: string; iconKey: string }) =>
      callService(() => AccountService.SetAccountIcon(id, iconKey)),
    onSuccess: (_data, variables) => invalidateAccountChange(queryClient, variables.id),
  });
}

export function useAppendAccountValue() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, amount, effectiveAt }: { id: string; amount: string; effectiveAt?: string }) =>
      callService(() => AccountService.AppendAccountValue(id, amount, effectiveAt ?? "")),
    onSuccess: (_data, variables) => invalidateActivityChange(queryClient, [variables.id]),
  });
}

export function useAppendAccountCashValue() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      accountId,
      amount,
      currency,
      effectiveAt,
    }: {
      accountId: string;
      amount: string;
      currency: string;
      effectiveAt?: string;
    }) => callService(() => HoldingService.AppendAccountCashValue(accountId, amount, currency, effectiveAt ?? "")),
    onSuccess: (_data, variables) => invalidateActivityChange(queryClient, [variables.accountId]),
  });
}

/** toUpdateAccountRequest maps the create-form's complete state onto the
 * Set-flags UpdateAccountRequest so an edit sheet can reuse AccountForm. */
export function toUpdateAccountRequest(request: CreateAccountRequest): UpdateAccountRequest {
  return {
    name: request.name,
    accountType: request.accountType,
    balanceSheetRole: request.balanceSheetRole,
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
    includeInPortfolio: request.includeInPortfolio,
    includeInLiquidAssets: request.includeInLiquidAssets,
    ownerIds: request.ownerIds,
    ownershipPercentages: request.ownershipPercentages,
    ownership: request.ownership,
  };
}
