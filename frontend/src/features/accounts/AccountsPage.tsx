import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Plus } from "lucide-react";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { useAccounts, useAccountValuations, useCreateAccount, useSetAccountLogo } from "@/queries/accounts";
import { attachPendingImage } from "@/queries/media";
import { AccountCreateWizard } from "@/features/accounts/AccountCreateWizard";
import { AccountDetail } from "@/features/accounts/AccountDetail";
import { formatAmount } from "@/lib/money";
import { displayEnum, displayError } from "@/lib/display";
import { PageHeader } from "@/components/layout/PageHeader";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { toast } from "sonner";
import type { AccountRecordDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import type { CreateAccountRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account/models";
import type { AccountFormExtras } from "@/features/accounts/AccountForm";

function groupAccounts(records: AccountRecordDTO[]): { key: string; label: string; records: AccountRecordDTO[] }[] {
  const groups = new Map<string, { label: string; records: AccountRecordDTO[] }>();
  for (const record of records) {
    const key = record.account.institutionId || "unassigned";
    const label = record.institutionName || "";
    const existing = groups.get(key);
    if (existing) {
      existing.records.push(record);
    } else {
      groups.set(key, { label, records: [record] });
    }
  }
  return [...groups.entries()]
    .map(([key, value]) => ({ key, label: value.label, records: value.records }))
    .sort((left, right) => {
      if (left.key === "unassigned") {
        return 1;
      }
      if (right.key === "unassigned") {
        return -1;
      }
      return left.label.localeCompare(right.label);
    });
}

/**
 * AccountsPage is the real-world account list. Clicking a row opens Account
 * detail in the same workspace. Create uses the institution-first wizard.
 */
export function AccountsPage({
  selectedAccountId,
  onSelectAccount,
}: {
  selectedAccountId?: string | null;
  onSelectAccount?: (id: string | null) => void;
} = {}) {
  const { t } = useTranslation();
  const [internalSelectedId, setInternalSelectedId] = useState<string | null>(null);
  const selectedId = onSelectAccount ? (selectedAccountId ?? null) : internalSelectedId;
  const setSelectedId = (id: string | null) => {
    if (onSelectAccount) {
      onSelectAccount(id);
      return;
    }
    setInternalSelectedId(id);
  };
  const [showArchived, setShowArchived] = useState(false);
  const accounts = useAccounts({ includeArchived: showArchived });
  const valuations = useAccountValuations({ includeArchived: showArchived });
  const createAccount = useCreateAccount();
  const setAccountLogo = useSetAccountLogo();
  const [createOpen, setCreateOpen] = useState(false);
  const [operation, setOperation] = useState<"create" | null>(null);
  const [createError, setCreateError] = useState<string | undefined>();

  const valuationByAccountId = useMemo(
    () => new Map((valuations.data ?? []).map((valuation) => [valuation.account.id, valuation])),
    [valuations.data],
  );
  const selectedRecord = (accounts.data ?? []).find((record) => record.account.id === selectedId) ?? null;
  const grouped = groupAccounts(accounts.data ?? []);

  const saveCreate = async (request: CreateAccountRequest, extras: AccountFormExtras) => {
    setCreateError(undefined);
    setOperation("create");
    try {
      const record = await createAccount.mutateAsync(request);
      try {
        await attachPendingImage(record.account.id, extras.pendingImage, (args) => setAccountLogo.mutateAsync(args));
        toast.success(t("accounts.created"));
      } catch (error) {
        const message = displayError(error, t("accounts.mediaSaveError"));
        toast.error(`${message} ${t("accounts.mediaRetryHint")}`);
      }
      setCreateOpen(false);
      setSelectedId(record.account.id);
    } catch (error) {
      setCreateError(displayError(error, t("accounts.saveError")));
    } finally {
      setOperation(null);
    }
  };

  if (selectedRecord) {
    return (
      <AccountDetail
        record={selectedRecord}
        valuation={valuationByAccountId.get(selectedRecord.account.id)}
        valuationUpdatedAt={valuations.dataUpdatedAt}
        onBack={() => setSelectedId(null)}
      />
    );
  }

  const createSubmitting = createAccount.isPending || operation === "create";

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={t("nav.accounts")}
        description={t("accounts.description")}
        actions={
          <>
            <label className="flex items-center gap-2 text-sm text-muted-foreground">
              <input type="checkbox" checked={showArchived} onChange={(event) => setShowArchived(event.target.checked)} />
              {t("accounts.showArchived")}
            </label>
            <Sheet
              open={createOpen}
              onOpenChange={(open) => {
                setCreateOpen(open);
                if (open) setCreateError(undefined);
              }}
            >
              <SheetTrigger className={cn(buttonVariants(), "gap-2")}>
                <Plus className="size-4" aria-hidden="true" /> {t("accounts.create")}
              </SheetTrigger>
              <SheetContent>
                <SheetHeader>
                  <SheetTitle>{t("accounts.createTitle")}</SheetTitle>
                </SheetHeader>
                <div className="overflow-y-auto">
                  <AccountCreateWizard isSubmitting={createSubmitting} submissionError={createError} onSubmit={saveCreate} />
                </div>
              </SheetContent>
            </Sheet>
          </>
        }
      />

      {accounts.isLoading && <LoadingState label={t("ui.state.loadingPage")} />}
      {accounts.isError && (
        <ErrorState
          title={t("accounts.loadError")}
          description={t("ui.state.errorDescription")}
          onRetry={() => accounts.refetch()}
          retryLabel={t("common.retryAction")}
        />
      )}

      {!accounts.isLoading && !accounts.isError && accounts.data && accounts.data.length === 0 && (
        <EmptyState title={t("accounts.emptyTitle")} description={t("accounts.emptyDescription")} />
      )}

      {!accounts.isLoading && !accounts.isError && accounts.data && accounts.data.length > 0 && (
        <div className="flex flex-col gap-6" data-testid="accounts-table">
          <span className="sr-only">{t("accounts.tableLabel")}</span>
          {grouped.map((group) => (
            <section key={group.key} className="flex flex-col gap-2">
              <h2 className="text-sm font-medium text-muted-foreground">
                {group.key === "unassigned" ? t("accounts.unassignedInstitution") : group.label}
              </h2>
              <ul className="overflow-hidden rounded-lg border border-border">
                {group.records.map((record) => {
                  const valuation = valuationByAccountId.get(record.account.id);
                  const value = valuation?.baseValue
                    ? formatAmount(valuation.baseValue.amount, valuation.baseValue.currency)
                    : t("accounts.noValue");
                  const completeness = valuation
                    ? valuation.complete
                      ? t("accounts.completeValuation")
                      : t("accounts.partialValuation")
                    : valuations.isLoading
                      ? t("common.loading")
                      : t("accounts.noValue");
                  return (
                    <li key={record.account.id} className="border-b border-border last:border-0">
                      <button
                        type="button"
                        className="flex w-full items-start justify-between gap-3 px-3 py-3 text-left hover:bg-muted/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50"
                        onClick={() => setSelectedId(record.account.id)}
                      >
                        <span className="flex min-w-0 flex-col gap-1">
                          <span className="flex flex-wrap items-center gap-2 font-medium">
                            {record.account.name}
                            <Badge variant="outline">{displayEnum(t, "enum", record.account.accountType)}</Badge>
                            {record.account.archivedAt && <Badge variant="secondary">{t("common.archived")}</Badge>}
                            {record.account.balanceSheetRole === "liability" && (
                              <Badge variant="outline">{t("accounts.liability")}</Badge>
                            )}
                          </span>
                          <span className="text-xs text-muted-foreground">{completeness}</span>
                        </span>
                        <span className="shrink-0 text-sm font-medium">{value}</span>
                      </button>
                    </li>
                  );
                })}
              </ul>
            </section>
          ))}
        </div>
      )}
    </div>
  );
}
