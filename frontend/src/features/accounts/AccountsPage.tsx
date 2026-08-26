import { useState } from "react";
import { flexRender, getCoreRowModel, useReactTable, createColumnHelper } from "@tanstack/react-table";
import { useTranslation } from "react-i18next";
import { Plus } from "lucide-react";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { useAccounts, useArchiveAccount, useCreateAccount } from "@/queries/accounts";
import { AccountForm } from "@/features/accounts/AccountForm";
import { formatAmount } from "@/lib/money";
import { toast } from "sonner";
import type { AccountRecordDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";

const columnHelper = createColumnHelper<AccountRecordDTO>();

/**
 * AccountsPage implements list/filter/create/archive/restore end to end
 * through AccountService (implementation plan Phase 4). Appending a
 * current value re-uses AccountService.AppendAccountValue via the same
 * form pattern once History (Phase 5) exists; for this phase, archive is
 * the one high-risk action, guarded by an AlertDialog + Toast per the
 * frontend stack decision Sec11.
 */
export function AccountsPage() {
  const { t } = useTranslation();
  const [showArchived, setShowArchived] = useState(false);
  const accounts = useAccounts({ includeArchived: showArchived });
  const createAccount = useCreateAccount();
  const archiveAccount = useArchiveAccount();
  const [createOpen, setCreateOpen] = useState(false);

  const columns = [
    columnHelper.accessor((row) => row.account.name, {
      id: "name",
      header: t("accounts.name"),
      cell: (info) => (
        <span className="flex items-center gap-2">
          {info.getValue()}
          {info.row.original.account.archivedAt && <Badge variant="secondary">{t("common.archived")}</Badge>}
        </span>
      ),
    }),
    columnHelper.accessor((row) => row.account.primaryCategory, { id: "category", header: t("accounts.category") }),
    columnHelper.accessor((row) => row.account.defaultCurrency, { id: "currency", header: t("accounts.currency") }),
    columnHelper.accessor((row) => row.latestValue?.amount.amount, {
      id: "value",
      header: t("overview.netWorth", { defaultValue: "Value" }),
      cell: (info) => {
        const value = info.row.original.latestValue;
        return value ? formatAmount(value.amount.amount, value.amount.currency) : t("accounts.noValue");
      },
    }),
    columnHelper.display({
      id: "actions",
      header: "",
      cell: (info) => {
        const record = info.row.original;
        const archived = Boolean(record.account.archivedAt);
        return (
          <AlertDialog>
            <AlertDialogTrigger className={cn(buttonVariants({ variant: "outline", size: "sm" }))}>
              {archived ? t("common.active") : t("accounts.archive")}
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>{archived ? t("common.active") : t("accounts.archive")}</AlertDialogTitle>
                <AlertDialogDescription>{record.account.name}</AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
                <AlertDialogAction
                  onClick={() =>
                    archiveAccount.mutate(
                      { id: record.account.id, archived: !archived },
                      { onSuccess: () => toast.success(archived ? t("common.active") : t("accounts.archive")) },
                    )
                  }
                >
                  {archived ? t("common.active") : t("accounts.archive")}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        );
      },
    }),
  ];

  const table = useReactTable({
    data: accounts.data ?? [],
    columns,
    getCoreRowModel: getCoreRowModel(),
  });

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <label className="flex items-center gap-2 text-sm text-muted-foreground">
          <input type="checkbox" checked={showArchived} onChange={(event) => setShowArchived(event.target.checked)} />
          {t("accounts.showArchived")}
        </label>
        <Sheet open={createOpen} onOpenChange={setCreateOpen}>
          <SheetTrigger className={cn(buttonVariants(), "gap-2")}>
            <Plus className="size-4" /> {t("accounts.create")}
          </SheetTrigger>
          <SheetContent>
            <SheetHeader>
              <SheetTitle>{t("accounts.createTitle")}</SheetTitle>
            </SheetHeader>
            <div className="overflow-y-auto">
              <AccountForm
                submitLabel={t("accounts.create")}
                isSubmitting={createAccount.isPending}
                onSubmit={(request) =>
                  createAccount.mutate(request, {
                    onSuccess: () => {
                      toast.success(t("accounts.create"));
                      setCreateOpen(false);
                    },
                  })
                }
              />
            </div>
          </SheetContent>
        </Sheet>
      </div>

      {accounts.isLoading && <p className="text-sm text-muted-foreground">{t("common.comingSoon")}</p>}
      {accounts.isError && <p role="alert" className="text-sm text-destructive">{t("accounts.loadError")}</p>}

      {accounts.data && accounts.data.length === 0 && (
        <div className="flex flex-col items-center gap-1 py-12 text-center">
          <p className="font-medium text-foreground">{t("accounts.emptyTitle")}</p>
          <p className="text-sm text-muted-foreground">{t("accounts.emptyDescription")}</p>
        </div>
      )}

      {accounts.data && accounts.data.length > 0 && (
        <table className="w-full text-left text-sm" data-testid="accounts-table">
          <thead>
            {table.getHeaderGroups().map((headerGroup) => (
              <tr key={headerGroup.id} className="border-b border-border">
                {headerGroup.headers.map((header) => (
                  <th key={header.id} className="py-2 font-medium text-muted-foreground">
                    {flexRender(header.column.columnDef.header, header.getContext())}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {table.getRowModel().rows.map((row) => (
              <tr key={row.id} className="border-b border-border">
                {row.getVisibleCells().map((cell) => (
                  <td key={cell.id} className="py-2">
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
