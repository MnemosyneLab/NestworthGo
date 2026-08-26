import { useState, Fragment } from "react";
import { flexRender, getCoreRowModel, useReactTable, createColumnHelper } from "@tanstack/react-table";
import { useTranslation } from "react-i18next";
import { Plus } from "lucide-react";
import { Button, buttonVariants } from "@/components/ui/button";
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
import { attachPendingImage } from "@/queries/media";
import { AccountForm } from "@/features/accounts/AccountForm";
import { ImagePicker } from "@/components/forms/ImagePicker";
import { formatAmount } from "@/lib/money";
import { displayEnum, displayError } from "@/lib/display";
import { PageHeader } from "@/components/layout/PageHeader";
import { EmptyState, ErrorState, LoadingState } from "@/components/layout/PageState";
import { toast } from "sonner";
import type { AccountRecordDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/wire/models";
import type { CreateAccountRequest } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account/models";
import type { AccountFormExtras } from "@/features/accounts/AccountForm";

const columnHelper = createColumnHelper<AccountRecordDTO>();

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
  const [operation, setOperation] = useState<"create" | "edit" | null>(null);
  const [createError, setCreateError] = useState<string | undefined>();
  const [editError, setEditError] = useState<string | undefined>();
  const [imageTargetId, setImageTargetId] = useState<string | null>(null);
  const [rowPendingImage, setRowPendingImage] = useState<string | undefined>(undefined);
  const [imageError, setImageError] = useState<{ id: string; message: string } | undefined>();
  const [imageSavingId, setImageSavingId] = useState<string | null>(null);

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
        setImageError({ id: record.account.id, message });
        setImageTargetId(record.account.id);
        setRowPendingImage(undefined);
      }
      setCreateOpen(false);
    } catch (error) {
      setCreateError(displayError(error, t("accounts.saveError")));
    } finally {
      setOperation(null);
    }
  };

  const saveEdit = async (request: CreateAccountRequest, extras: AccountFormExtras) => {
    if (!editing) {
      return;
    }
    const id = editing.account.id;
    setEditError(undefined);
    setOperation("edit");
    try {
      await updateAccount.mutateAsync({ id, request: toUpdateAccountRequest(request) });
      try {
        await attachPendingImage(id, extras.pendingImage, (args) => setAccountLogo.mutateAsync(args));
        toast.success(t("common.saved"));
      } catch (error) {
        const message = displayError(error, t("accounts.mediaSaveError"));
        toast.error(`${message} ${t("accounts.mediaRetryHint")}`);
        setImageError({ id, message });
        setImageTargetId(id);
        setRowPendingImage(undefined);
      }
      setEditing(null);
    } catch (error) {
      setEditError(displayError(error, t("accounts.saveError")));
    } finally {
      setOperation(null);
    }
  };

  const saveRowImage = async (id: string) => {
    if (!rowPendingImage) {
      return;
    }
    setImageError(undefined);
    setImageSavingId(id);
    try {
      await attachPendingImage(id, rowPendingImage, (args) => setAccountLogo.mutateAsync(args));
      setImageTargetId(null);
      setRowPendingImage(undefined);
      toast.success(t("common.saved"));
    } catch (error) {
      setImageError({ id, message: displayError(error, t("accounts.imageSaveError")) });
    } finally {
      setImageSavingId(null);
    }
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
    columnHelper.accessor((row) => row.account.primaryCategory, {
      id: "category",
      header: t("accounts.category"),
      cell: (info) => displayEnum(t, "enum", info.getValue()),
    }),
    columnHelper.accessor((row) => row.account.defaultCurrency, { id: "currency", header: t("accounts.currency") }),
    columnHelper.accessor((row) => row.latestValue?.amount.amount, {
      id: "value",
      header: t("accounts.currentValue"),
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
            <button
              type="button"
              className={cn(buttonVariants({ variant: "outline", size: "sm" }))}
              onClick={() => {
                setImageTargetId(record.account.id);
                setRowPendingImage(undefined);
                setImageError(undefined);
              }}
            >
              {t("accounts.setImage")}
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
                        {
                          onSuccess: () => toast.success(archived ? t("common.active") : t("accounts.archive")),
                          onError: (error) => toast.error(displayError(error, t("accounts.saveError"))),
                        },
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

  const createSubmitting = createAccount.isPending || operation === "create";
  const editSubmitting = updateAccount.isPending || operation === "edit";

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
                  <AccountForm
                    submitLabel={t("accounts.create")}
                    isSubmitting={createSubmitting}
                    submissionError={createError}
                    onSubmit={saveCreate}
                  />
                </div>
              </SheetContent>
            </Sheet>
          </>
        }
      />

      <Sheet
        open={Boolean(editing)}
        onOpenChange={(open) => {
          if (!open) setEditing(null);
          if (open) setEditError(undefined);
        }}
      >
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
                isSubmitting={editSubmitting}
                submissionError={editError}
                onSubmit={saveEdit}
              />
            )}
          </div>
        </SheetContent>
      </Sheet>

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
        <EmptyState
          title={t("accounts.emptyTitle")}
          description={t("accounts.emptyDescription")}
        />
      )}

      {!accounts.isLoading && !accounts.isError && accounts.data && accounts.data.length > 0 && (
        <div className="overflow-x-auto rounded-lg border border-border">
          <table className="w-full min-w-[44rem] text-left text-sm" data-testid="accounts-table">
            <caption className="sr-only">{t("accounts.tableLabel")}</caption>
            <thead>
              {table.getHeaderGroups().map((headerGroup) => (
                <tr key={headerGroup.id} className="border-b border-border bg-muted/40">
                  {headerGroup.headers.map((header) => (
                    <th key={header.id} className="px-3 py-2 font-medium text-muted-foreground">
                      {flexRender(header.column.columnDef.header, header.getContext())}
                    </th>
                  ))}
                </tr>
              ))}
            </thead>
            <tbody>
              {table.getRowModel().rows.map((row) => {
                const record = row.original;
                const imageOpen = imageTargetId === record.account.id;
                return (
                  <Fragment key={row.id}>
                    <tr className="border-b border-border last:border-0">
                      {row.getVisibleCells().map((cell) => (
                        <td key={cell.id} className="px-3 py-3 align-top">
                          {flexRender(cell.column.columnDef.cell, cell.getContext())}
                        </td>
                      ))}
                    </tr>
                    {imageOpen && (
                      <tr className="border-b border-border bg-muted/40 last:border-0">
                        <td colSpan={row.getVisibleCells().length} className="px-3 py-3">
                          <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
                            <ImagePicker
                              label={t("accounts.setImage")}
                              value={rowPendingImage}
                              existingAssetId={record.account.logoAssetId}
                              onChange={setRowPendingImage}
                            />
                            <Button
                              type="button"
                              size="sm"
                              disabled={!rowPendingImage || imageSavingId === record.account.id}
                              onClick={() => void saveRowImage(record.account.id)}
                            >
                              {imageSavingId === record.account.id ? t("common.pending") : t("common.save")}
                            </Button>
                          </div>
                          {imageError?.id === record.account.id && (
                            <p role="alert" className="mt-2 text-sm text-destructive">
                              {imageError.message} {t("accounts.imageRetry")}
                            </p>
                          )}
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
