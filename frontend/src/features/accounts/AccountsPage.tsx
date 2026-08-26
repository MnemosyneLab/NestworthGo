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
import {
  useAccounts,
  useArchiveAccount,
  useCreateAccount,
  useSetAccountLogo,
  useUpdateAccount,
  toUpdateAccountRequest,
} from "@/queries/accounts";
import { persistPickedImage } from "@/queries/media";
import { AccountForm } from "@/features/accounts/AccountForm";
import { formatAmount } from "@/lib/money";
import { toast } from "sonner";
import type { AccountRecordDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import type { CreateAccountRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account/models";
import type { AccountFormExtras } from "@/features/accounts/AccountForm";

const columnHelper = createColumnHelper<AccountRecordDTO>();

async function attachAccountLogo(
  id: string,
  pendingImage: string | undefined,
  setLogo: (args: { id: string; mediaAssetId: string }) => Promise<unknown>,
) {
  if (!pendingImage) {
    return;
  }
  await persistPickedImage(pendingImage, (mediaAssetId) => setLogo({ id, mediaAssetId }));
}

/**
 * AccountsPage implements list/filter/create/update/archive/restore end
 * to end through AccountService. Icon/logo pick happens on the create
 * and edit sheets via MediaService.PickImage, persisted after confirm.
 */
export function AccountsPage() {
  const { t } = useTranslation();
  const [showArchived, setShowArchived] = useState(false);
  const accounts = useAccounts({ includeArchived: showArchived });
  const createAccount = useCreateAccount();
  const updateAccount = useUpdateAccount();
  const archiveAccount = useArchiveAccount();
  const setAccountLogo = useSetAccountLogo();
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<AccountRecordDTO | null>(null);

  const saveCreate = (request: CreateAccountRequest, extras: AccountFormExtras) => {
    createAccount.mutate(request, {
      onSuccess: (record) => {
        void attachAccountLogo(record.account?.id ?? "", extras.pendingImage, (args) =>
          setAccountLogo.mutateAsync(args),
        ).then(() => {
          toast.success(t("accounts.create"));
          setCreateOpen(false);
        });
      },
    });
  };

  const saveEdit = (request: CreateAccountRequest, extras: AccountFormExtras) => {
    if (!editing) {
      return;
    }
    const id = editing.account.id;
    updateAccount.mutate(
      { id, request: toUpdateAccountRequest(request) },
      {
        onSuccess: () => {
          void attachAccountLogo(id, extras.pendingImage, (args) => setAccountLogo.mutateAsync(args)).then(() => {
            toast.success(t("common.saved"));
            setEditing(null);
          });
        },
      },
    );
  };

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
          <div className="flex items-center justify-end gap-2">
            <button
              type="button"
              className={cn(buttonVariants({ variant: "outline", size: "sm" }))}
              onClick={() => setEditing(record)}
            >
              {t("common.edit")}
            </button>
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
          </div>
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
                onSubmit={saveCreate}
              />
            </div>
          </SheetContent>
        </Sheet>
      </div>

      <Sheet open={Boolean(editing)} onOpenChange={(open) => !open && setEditing(null)}>
        <SheetContent>
          <SheetHeader>
            <SheetTitle>{t("accounts.edit")}</SheetTitle>
          </SheetHeader>
          <div className="overflow-y-auto">
            {editing && (
              <AccountForm
                key={editing.account.id}
                record={editing}
                submitLabel={t("common.save")}
                isSubmitting={updateAccount.isPending}
                onSubmit={saveEdit}
              />
            )}
          </div>
        </SheetContent>
      </Sheet>

      {accounts.isLoading && <p className="text-sm text-muted-foreground">{t("common.comingSoon")}</p>}
      {accounts.isError && (
        <p role="alert" className="text-sm text-destructive">
          {t("accounts.loadError")}
        </p>
      )}

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
