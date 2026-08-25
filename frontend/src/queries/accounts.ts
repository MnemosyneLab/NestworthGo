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
