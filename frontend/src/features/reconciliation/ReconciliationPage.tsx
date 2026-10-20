import { ReconciliationCoverageNotice } from "./ReconciliationCoverageNotice";
import { useHistoryOrigin } from "@/queries/history";
import { localDateTimeInTimeZone } from "@/lib/time";
import {
  adjustmentErrorKey,
  knownReconciliationConflict,
  completedReconciliation,
} from "./reconciliationErrors";
import { ArrowLeft, ChevronRight } from "lucide-react";
import { Card } from "@/components/ui/card";
import {
  Counts,
  FormField,
  Notice,
  PAGE_SIZE,
  Pager,
  StatusBadge,
  outcomeVariant,
  sessionStatusVariant,
} from "./ReconciliationParts";
import { cn } from "@/lib/utils";
import { checkpointReference, checkpointValue, formatRecordedAmount, formatRecordedDate, formatRecordedRate, sourceText } from "./reconciliationDisplay";
import { FieldError } from "@/components/ui/field-error";
import { ProductPreview as ProductOperationReview } from "@/features/liquidity/ProductPreview";
import {
  ProductTermsFields,
  ProductPolicyFields,
} from "@/features/liquidity/ProductFields";
import {
  termsFromProduct,
  policyFromDTO,
} from "@/features/liquidity/productPolicy";
import { ProductFormSheet } from "@/features/liquidity/ProductFormSheet";
import { ProductCorrection } from "./ProductCorrection";
import {
  useEffect,
  useId,
  useRef,
  useState,
  type MouseEvent as ReactMouseEvent,
} from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { utf8ByteLength } from "@/lib/utf8";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/select";
import { DatePicker } from "@/components/ui/date-picker";
import { TimePicker } from "@/components/ui/time-picker";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  useSheetConfirmation,
  hasUnresolvedSheetConfirmation,
} from "@/components/ui/sheet-confirmation";
import {
  LoadingState,
  ErrorState,
  EmptyState,
} from "@/components/layout/PageState";
import { useAccounts } from "@/queries/accounts";
import {
  useReconciliationSessions,
  useReconciliationSession,
  useReconciliationLinks,
  useReconciliationChanges,
  ReconciliationService,
} from "@/queries/reconciliation";
import { invalidateActivityChange } from "@/queries/invalidation";
import { callService, type WireError } from "@/lib/wails";
import { displayError } from "@/lib/display";
import {
  formatQuotePrice,
  compareCanonical,
} from "@/lib/money";
import { useReconciliationTimestamp } from "./useReconciliationTimestamp";
import { RecordChangeForm } from "@/features/history/RecordChangeForm";
import { emptyChangeRequest } from "@/features/history/activityToCommand";
import { ChangeCommandKind } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history/models";
import type { ReconciliationAction } from "../../../bindings/github.com/waltwang/nestworth-go/internal/application/models";
import type {
  ReconciliationSession,
  ReconciliationItem,
  ReconciliationAccount,
} from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
import type {
  AdjustmentPreview,
  ProductPreview,
} from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation/models";

const secondaryAction =
  "h-auto whitespace-normal py-1.5 text-left text-primary hover:text-primary";
const detailsClass =
  "group rounded-xl border border-border bg-card text-sm open:shadow-sm";
const summaryClass =
  "cursor-pointer select-none rounded-xl px-3 py-2.5 font-medium text-foreground marker:text-muted-foreground hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50";

const validationFields = new Set([
  "source",
  "asOfDate",
  "asOfTime",
  "allowPartial",
  "note",
  "outcome",
  "referenceValue",
  "referenceComparable",
]);

const reconciliationAction = (
  action: string,
  session?: ReconciliationSession,
): ReconciliationAction => ({
  action,
  sessionId: session?.id,
  expectedRevision: session?.revision ?? 0,
  financialRevision: 0,
  source: { reference: "", asOfDate: "" },
  note: "",
  allowPartial: false,
});
type Review = {
  action?: string;
  planId: string;
  result?: ReconciliationSession;
  adjustment?: AdjustmentPreview;
  product?: ProductPreview;
};
function archivedTarget(
  account: ReconciliationAccount,
  item: ReconciliationItem,
): boolean {
  return Boolean(
    item.archivedNonzero ||
      (item.holdingId && account.archivedHoldingIds?.includes(item.holdingId)),
  );
}

export function ReconciliationPage({
  accountId,
  sessionId,
  navigationRequestId = 0,
  isActive = true,
}: {
  accountId?: string;
  sessionId?: string;
  navigationRequestId?: number;
  isActive?: boolean;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const formatTimestamp = useReconciliationTimestamp();
  const filterId = useId();
  const accounts = useAccounts();
  const sessions = useReconciliationSessions(accountId, isActive);
  const [selected, setSelected] = useState<string[] | null>(
    accountId ? [accountId] : null,
  );
  const [active, setActive] = useState(sessionId ?? "");
  const detail = useReconciliationSession(active);
  const [filter, setFilter] = useState("");
  const [page, setPage] = useState(0);
  const [review, setReview] = useState<Review>();
  const [error, setError] = useState<string>();
  const [errorField, setErrorField] = useState<string>();
  const clearError = () => {
    setError(undefined);
    setErrorField(undefined);
  };
  const [busy, setBusy] = useState(false);
  type EditingIntent = {
    account: ReconciliationAccount;
    item: ReconciliationItem;
    route: "check" | "adjust" | "record" | "product" | "import" | "newProduct";
  };
  const [editing, setEditing] = useState<EditingIntent & { requestId: number }>();
  const editingRequestId = useRef(0);
  const beginEditing = (intent: EditingIntent) => setEditing({ ...intent, requestId: ++editingRequestId.current });
  const endEditing = () => { editingRequestId.current += 1; setEditing(undefined); };
  const [closing, setClosing] = useState(false);
  const [closingNote, setClosingNote] = useState("");
  const [partial, setPartial] = useState(false);
  const trigger = useRef<HTMLElement | null>(null);
  const previewSequence = useRef(0);
  const invalidatePreview = () => {
    previewSequence.current += 1;
    setBusy(false);
  };

  const appliedNavigationRequest = useRef(navigationRequestId);
  useEffect(() => {
    // Only explicit entries reset the workspace. Visibility, tab changes and
    // Back retain the mounted session, filters and any editable draft.
    if (appliedNavigationRequest.current === navigationRequestId) return;
    appliedNavigationRequest.current = navigationRequestId;
    previewSequence.current += 1;
    editingRequestId.current += 1;
    setBusy(false);
    setActive(sessionId ?? "");
    setSelected(accountId ? [accountId] : null);
    setFilter("");
    setPage(0);
    setEditing(undefined);
    setReview(undefined);
    setClosing(false);
    setPartial(false);
    setClosingNote("");
    setError(undefined);
    setErrorField(undefined);
  }, [navigationRequestId, accountId, sessionId]);

  const refresh = () => {
    void client.invalidateQueries({ queryKey: ["reconciliation"] });
  };
  const preview = async (action: ReconciliationAction) => {
    const sequence = ++previewSequence.current;
    setBusy(true);
    clearError();
    try {
      const result = await callService(() =>
        ReconciliationService.Preview(action),
      );
      if (sequence !== previewSequence.current) return;
      setReview({ ...result, action: action.action });
    } catch (cause) {
      if (sequence !== previewSequence.current) return;
      const wire = (cause as { wireError?: WireError })?.wireError;
      setErrorField(wire?.field);
      const key =
        wire?.code === "validation" &&
        wire.field &&
        validationFields.has(wire.field)
          ? wire.field
          : undefined;
      if (knownReconciliationConflict(wire)) refresh();
      setError(
        completedReconciliation(wire)
          ? t("reconciliation.readOnly")
          : knownReconciliationConflict(wire)
            ? t("reconciliation.changedReview")
            : key
              ? t(`reconciliation.validation.${key}`)
              : displayError(cause, t("reconciliation.error")),
      );
    } finally {
      if (sequence === previewSequence.current) setBusy(false);
    }
  };
  const activeAccounts = (accounts.data ?? []).filter(
    (a) => !a.account.archivedAt,
  );
  const activeAccountIds = new Set(activeAccounts.map((a) => a.account.id));
  const chosen = selected ? selected.filter((id) => activeAccountIds.has(id)) : [...activeAccountIds];
  const sessionRows = sessions.data ?? [];
  const rows = sessionRows.filter((s) =>
    filter ? s.status === filter : s.status === "completed",
  );
  const hasUnfinished = sessionRows.some(
    (s) => s.status === "in_progress" || s.status === "paused",
  );
  const current = detail.data;
  const select = (id: string) => {
    if (hasUnresolvedSheetConfirmation()) return;
    invalidatePreview();
    setActive(id);
    endEditing();
    setReview(undefined);
    setClosing(false);
    setPartial(false);
    setClosingNote("");
    clearError();
  };
  return (
    <div className="flex min-w-0 flex-col gap-6">
      <p className="max-w-3xl text-sm leading-6 text-muted-foreground">
        {t("reconciliation.localOnly")}
      </p>
      {error && !editing && !closing && (
        <Notice tone="danger" role="alert">
          {error}
        </Notice>
      )}
      {!active ? (
        <>
          {accounts.isLoading || sessions.isLoading ? (
            <LoadingState label={t("reconciliation.loading")} variant="list" />
          ) : accounts.isError || sessions.isError ? (
            <ErrorState
              description={t("ui.state.errorDescription")}
              title={t("reconciliation.error")}
              retryLabel={t("reconciliation.retry")}
              onRetry={() => {
                void accounts.refetch();
                void sessions.refetch();
              }}
            />
          ) : (
            <>
              {(sessions.data ?? [])
                .filter((s) => s.status !== "completed")
                .map((s) => (
                  <Card
                    key={s.id}
                    className="flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between"
                  >
                    <div className="flex min-w-0 flex-1 flex-col gap-3">
                      <p className="break-words font-semibold">
                        {s.accountNames?.join(" · ")} ·{" "}
                        {t(`reconciliation.${s.status}`)}
                      </p>
                      <Counts counts={s.counts} />
                    </div>
                    <Button className="shrink-0" onClick={() => select(s.id)}>
                      {t("reconciliation.continue")}
                    </Button>
                  </Card>
                ))}
              {!activeAccounts.length && (
                <EmptyState title={t("reconciliation.noAccounts")} />
              )}
              <Card className="p-5">
                <fieldset className="flex min-w-0 flex-col gap-4">
                  <legend className="mb-4 p-0 text-base font-semibold">
                    {t("reconciliation.selectAccounts")}
                  </legend>
                  <div className="grid gap-2 sm:grid-cols-2">
                    {activeAccounts.map((a) => (
                      <label
                        key={a.account.id}
                        className={cn(
                          "flex min-w-0 cursor-pointer items-center gap-3 rounded-xl border px-3 py-2.5 text-sm transition-colors",
                          "focus-within:ring-2 focus-within:ring-primary/50",
                          chosen.includes(a.account.id)
                            ? "border-primary/40 bg-primary/5"
                            : "border-border bg-card hover:border-primary/30 hover:bg-muted/50",
                        )}
                      >
                        <input
                          type="checkbox"
                          className="size-4 shrink-0 accent-primary"
                          checked={chosen.includes(a.account.id)}
                          onChange={(e) =>
                            setSelected(
                              e.target.checked
                                ? [...chosen, a.account.id]
                                : chosen.filter((id) => id !== a.account.id),
                            )
                          }
                        />
                        <span className="min-w-0 break-words font-medium">
                          {a.account.name} · {a.account.defaultCurrency}
                        </span>
                      </label>
                    ))}
                  </div>
                  {(sessions.data ?? []).some(
                    (s) =>
                      s.status !== "completed" &&
                      s.accountIds?.some((id) => chosen.includes(id)),
                  ) && (
                    <Notice tone="warning" role="status">
                      {t("reconciliation.overlap")}
                    </Notice>
                  )}
                  <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border pt-4">
                    <Button
                      variant="ghost"
                      onClick={() =>
                        setSelected(activeAccounts.map((a) => a.account.id))
                      }
                    >
                      {t("reconciliation.selectAll")}
                    </Button>
                    <Button
                      disabled={busy || !chosen.length}
                      onClick={() =>
                        void preview({
                          ...reconciliationAction("start"),
                          accountIds: chosen,
                        })
                      }
                    >
                      {t("reconciliation.start")}
                    </Button>
                  </div>
                </fieldset>
              </Card>
              <section className="flex min-w-0 flex-col gap-3">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <Label
                    htmlFor={filterId}
                    className="text-base font-semibold"
                  >
                    {t("reconciliation.history")}
                  </Label>
                  <NativeSelect
                    id={filterId}
                    className="w-full sm:w-52"
                    value={filter}
                    onChange={(e) => {
                      setFilter(e.target.value);
                      setPage(0);
                    }}
                  >
                    <option value="">{t("reconciliation.all")}</option>
                    {["in_progress", "paused", "completed"].map((s) => (
                      <option key={s} value={s}>
                        {t(`reconciliation.${s}`)}
                      </option>
                    ))}
                  </NativeSelect>
                </div>
                {!rows.length && (Boolean(filter) || !hasUnfinished) && (
                  <EmptyState
                    title={t(
                      filter
                        ? "reconciliation.noMatches"
                        : "reconciliation.empty",
                    )}
                  />
                )}
                {rows.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE).map((s) => (
                  <Button
                    key={s.id}
                    variant="outline"
                    className="h-auto w-full justify-between gap-3 whitespace-normal rounded-xl px-4 py-3 text-left font-normal"
                    onClick={() => select(s.id)}
                  >
                    <span className="flex min-w-0 flex-1 flex-col gap-1.5">
                      <span className="break-words font-semibold">
                        {s.accountNames?.join(" · ")}
                      </span>
                      <span className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                        <span className="num">{formatTimestamp(s.startedAt)}</span>
                        <StatusBadge variant={sessionStatusVariant(s.status)}>
                          {t(`reconciliation.${s.status}`)}
                        </StatusBadge>
                        {s.partial && (
                          <StatusBadge variant="warning">
                            {t("reconciliation.partial")}
                          </StatusBadge>
                        )}
                      </span>
                    </span>
                    <ChevronRight
                      className="size-4 shrink-0 text-muted-foreground"
                      aria-hidden="true"
                    />
                  </Button>
                ))}
                <Pager page={page} total={rows.length} onPage={setPage} />
              </section>
            </>
          )}
        </>
      ) : (
        <>
          <Button
            variant="ghost"
            size="sm"
            className="-ml-2 self-start"
            onClick={() => select("")}
          >
            <ArrowLeft aria-hidden="true" />
            {t("reconciliation.back")}
          </Button>
          {detail.isLoading && !current ? (
            <LoadingState label={t("reconciliation.loading")} variant="table" />
          ) : detail.isError && !current ? (
            <ErrorState
              description={t("ui.state.errorDescription")}
              title={t("reconciliation.error")}
              retryLabel={t("reconciliation.retry")}
              onRetry={() => void detail.refetch()}
            />
          ) : (
            current && (
              <>
                {detail.isError && (
                  <Notice tone="warning" role="alert">
                    {t("reconciliation.stale")}
                  </Notice>
                )}
                <Card className="flex flex-col gap-4 p-5">
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <h2 className="min-w-0 break-words text-lg font-semibold tracking-tight">
                      {formatTimestamp(current.startedAt)} ·{" "}
                      {t(`reconciliation.${current.status}`)}
                    </h2>
                    {current.status !== "completed" && (
                      <div className="flex flex-wrap gap-2">
                        <Button
                          disabled={busy}
                          variant="outline"
                          onClick={() =>
                            void preview(
                              reconciliationAction(
                                current.status === "paused"
                                  ? "resume"
                                  : "pause",
                                current,
                              ),
                            )
                          }
                        >
                          {t(
                            `reconciliation.${current.status === "paused" ? "resume" : "pause"}`,
                          )}
                        </Button>
                        <Button
                          disabled={busy}
                          onClick={(e) => {
                            trigger.current = e.currentTarget;
                            setClosing(true);
                          }}
                        >
                          {t("reconciliation.finish")}
                        </Button>
                      </div>
                    )}
                  </div>
                  <Counts counts={current.counts} />
                  <div className="flex flex-col gap-1.5 text-sm empty:hidden">
                    {current.note && (
                      <p className="break-words">
                        {t("reconciliation.openingNote")}: {current.note}
                      </p>
                    )}
                    {current.closingNote && !current.partial && (
                      <p className="break-words">
                        {t("reconciliation.closingNote")}: {current.closingNote}
                      </p>
                    )}
                    {current.completedAt && (
                      <p className="num text-muted-foreground">
                        {t("reconciliation.completedAt")}:{" "}
                        {formatTimestamp(current.completedAt)}
                      </p>
                    )}
                  </div>
                  {current.status === "completed" && (
                    <Notice>{t("reconciliation.readOnly")}</Notice>
                  )}
                  {current.partial && (
                    <Notice tone="warning">
                      {t("reconciliation.partial")}: {current.closingNote}
                    </Notice>
                  )}
                </Card>
                {(current.accounts ?? []).map((a) => {
                  const edit =
                    (item: ReconciliationItem, route: EditingIntent["route"]) =>
                    (e: ReactMouseEvent<HTMLButtonElement>) => {
                      trigger.current = e.currentTarget;
                      beginEditing({ account: a, item, route });
                    };
                  return (
                    <section
                      key={a.accountId}
                      className="min-w-0 overflow-hidden rounded-2xl border border-border bg-card shadow-sm"
                    >
                      <header className="flex min-w-0 flex-col gap-3 border-b border-border bg-muted/30 px-5 py-4">
                        <h3 className="break-words text-base font-semibold">
                          {a.name} · {a.currency}
                        </h3>
                        <Counts counts={a.counts} />
                        <p className="num text-xs text-muted-foreground">
                          {t("reconciliation.lastSuccess")}:{" "}
                          {a.lastSuccessfulAt
                            ? formatTimestamp(a.lastSuccessfulAt)
                            : t("reconciliation.never")}
                        </p>
                        <ReconciliationCoverageNotice
                          account={a}
                          completed={current.status === "completed"}
                        />
                        {a.recheckRequired && (
                          <Notice tone="warning" role="status">
                            {t(
                              `reconciliation.${current.status === "completed" ? "recheck" : a.financialRevisionChanged ? "workingBookkeepingChanged" : "pendingReview"}`,
                            )}
                          </Notice>
                        )}
                      </header>
                      <div className="divide-y divide-border">
                        {[...(a.items ?? [])]
                          .sort(
                            (left, right) =>
                              Number(right.kind === "scope") -
                              Number(left.kind === "scope"),
                          )
                          .map((item) => {
                            const sourceDiffers =
                              item.outcome === "confirmed" &&
                              item.referenceComparable &&
                              /^-?\d+(\.\d+)?$/.test(item.referenceValue ?? "") &&
                              /^-?\d+(\.\d+)?$/.test(item.value ?? "") &&
                              compareCanonical(
                                item.referenceValue!,
                                item.value!,
                              ) !== 0;
                            return (
                              <article
                                key={item.id}
                                className={cn(
                                  "flex min-w-0 flex-col gap-3 px-5 py-4",
                                  item.outcome === "unresolved" &&
                                    "bg-warning/5",
                                  sourceDiffers && "bg-warning/5",
                                )}
                              >
                                <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
                                  <div className="flex min-w-0 flex-1 basis-56 flex-col gap-1">
                                    <h4 className="break-words font-semibold">
                                      {item.kind === "scope"
                                        ? t("reconciliation.scope")
                                        : item.label}{" "}
                                      · {t(`reconciliation.kind.${item.kind}`)}
                                    </h4>
                                    {item.kind === "holding" && (
                                      <HoldingIdentity detail={item.detail} />
                                    )}
                                    {item.kind === "scope" && (
                                      <p className="text-sm leading-6 text-muted-foreground">
                                        {t("reconciliation.scopeHelp")}
                                      </p>
                                    )}
                                  </div>
                                  <div className="flex min-w-0 shrink-0 flex-col items-end gap-1.5 text-right">
                                    {item.kind !== "scope" && (
                                      <p
                                        className={cn(
                                          "num break-words text-base font-semibold",
                                          !item.value &&
                                            "font-normal text-muted-foreground",
                                        )}
                                      >
                                        {checkpointValue(t, item)}
                                      </p>
                                    )}
                                    {item.cost && (
                                      <p className="num text-xs text-muted-foreground">
                                        {t("reconciliation.unitCost")}:{" "}
                                        {formatQuotePrice(
                                          item.cost,
                                          item.currency,
                                        )}
                                      </p>
                                    )}
                                    <StatusBadge
                                      variant={outcomeVariant(item.outcome)}
                                    >
                                      {t(`reconciliation.${item.outcome}`)}
                                    </StatusBadge>
                                  </div>
                                </div>
                                {item.archivedNonzero && (
                                  <Notice tone="warning" role="status">
                                    {t("reconciliation.archivedNonzero")}
                                  </Notice>
                                )}
                                {archivedTarget(a, item) &&
                                  current.status !== "completed" && (
                                    <Notice role="status">
                                      {t(
                                        "reconciliation.restoreBeforeCorrection",
                                      )}
                                    </Notice>
                                  )}
                                {item.kind === "product" && (
                                  <ProductEvidence detail={item.detail} />
                                )}
                                <div className="flex flex-col gap-1.5 rounded-xl bg-muted/50 px-3 py-2.5 text-sm empty:hidden">
                                  {item.checkedAt && (
                                    <p className="num break-words text-xs text-muted-foreground">
                                      {[
                                        sourceText(item.source),
                                        `${t("reconciliation.checkedAt")}: ${formatTimestamp(item.checkedAt)}`,
                                      ]
                                        .filter(Boolean)
                                        .join(" · ")}
                                    </p>
                                  )}
                                  {item.referenceValue && (
                                    <p
                                      className={cn(
                                        "num break-words",
                                        sourceDiffers &&
                                          "font-semibold text-warning-foreground",
                                      )}
                                    >
                                      {t("reconciliation.referenceValue")}:{" "}
                                      {checkpointReference(t, item)}
                                    </p>
                                  )}
                                  {sourceDiffers && (
                                    <Notice tone="warning" role="status">
                                      {t("reconciliation.mismatch")}
                                    </Notice>
                                  )}
                                  {item.note && (
                                    <p className="whitespace-pre-wrap break-words">
                                      {item.note}
                                    </p>
                                  )}
                                </div>
                                {current.status === "in_progress" && (
                                  <div className="flex flex-wrap items-center gap-2">
                                    <Button
                                      variant="outline"
                                      size="sm"
                                      onClick={edit(item, "check")}
                                    >
                                      {t("reconciliation.review")}
                                    </Button>
                                    {["cash", "balance", "holding"].includes(
                                      item.kind,
                                    ) &&
                                      !archivedTarget(a, item) && (
                                        <Button
                                          variant="ghost"
                                          size="sm"
                                          className={secondaryAction}
                                          onClick={edit(item, "adjust")}
                                        >
                                          {t("reconciliation.adjust")}
                                        </Button>
                                      )}
                                    {item.kind === "product" && (
                                      <Button
                                        variant="ghost"
                                        size="sm"
                                        className={secondaryAction}
                                        onClick={edit(item, "product")}
                                      >
                                        {t("reconciliation.productRoute")}
                                      </Button>
                                    )}
                                    {item.kind === "scope" &&
                                      a.mode === "holdings" && (
                                        <Button
                                          variant="ghost"
                                          size="sm"
                                          className={secondaryAction}
                                          onClick={edit(item, "newProduct")}
                                        >
                                          {t("reconciliation.addProduct")}
                                        </Button>
                                      )}
                                    {item.kind === "scope" &&
                                      a.mode === "holdings" && (
                                        <Button
                                          variant="ghost"
                                          size="sm"
                                          className={secondaryAction}
                                          onClick={edit(item, "import")}
                                        >
                                          {t("reconciliation.importPosition")}
                                        </Button>
                                      )}
                                    {item.kind !== "product" &&
                                      item.kind !== "removed" &&
                                      !archivedTarget(a, item) && (
                                        <Button
                                          variant="ghost"
                                          size="sm"
                                          className={secondaryAction}
                                          onClick={edit(item, "record")}
                                        >
                                          {t("reconciliation.recordMissing")}
                                        </Button>
                                      )}
                                  </div>
                                )}
                                {!!item.attempts?.length && (
                                  <details className={detailsClass}>
                                    <summary className={summaryClass}>
                                      {t("reconciliation.attempts", {
                                        count: item.attempts.length,
                                      })}
                                    </summary>
                                    <div className="flex flex-col gap-1.5 border-t border-border px-3 py-2.5">
                                      {item.attempts.map((attempt, i) => (
                                        <p
                                          key={i}
                                          className="num break-words text-sm"
                                        >
                                          {[
                                            formatTimestamp(attempt.checkedAt),
                                            t(`reconciliation.${attempt.outcome}`),
                                            checkpointValue(t, attempt, true),
                                            sourceText(attempt.source),
                                            attempt.note,
                                          ]
                                            .filter(Boolean)
                                            .join(" · ")}
                                        </p>
                                      ))}
                                    </div>
                                  </details>
                                )}
                              </article>
                            );
                          })}
                      </div>
                    </section>
                  );
                })}
                <RecentChanges sessionId={active} session={current} />
                <LinkedChanges sessionId={active} />
              </>
            )
          )}
        </>
      )}
      {closing && current && (
        <Sheet
          open
          onOpenChange={(open) => {
            if (!open) {
              invalidatePreview();
              setClosing(false);
              clearError();
            }
          }}
        >
          <SheetContent finalFocus={() => trigger.current}>
            <SheetHeader>
              <SheetTitle>{t("reconciliation.finish")}</SheetTitle>
            </SheetHeader>
            <div className="flex flex-col gap-4">
              <Notice>{t("reconciliation.finishNotice")}</Notice>
              {error && errorField !== "allowPartial" && (
                <FieldError>{error}</FieldError>
              )}
              <label className="flex cursor-pointer items-start gap-3 rounded-xl border border-border px-3 py-2.5 text-sm">
                <input
                  type="checkbox"
                  className="mt-0.5 size-4 shrink-0 accent-primary"
                  checked={partial}
                  aria-invalid={errorField === "allowPartial" || undefined}
                  aria-describedby={
                    errorField === "allowPartial" ? "finish-error" : undefined
                  }
                  onChange={(e) => {
                    setPartial(e.target.checked);
                    if (e.target.checked && closingNote.trim()) clearError();
                  }}
                />
                {t("reconciliation.allowPartial")}
              </label>
              <FieldError id="finish-error">
                {errorField === "allowPartial" ? error : undefined}
              </FieldError>
              <FormField>
                <Label htmlFor="closing-note">
                  {t("reconciliation.closingNote")}
                </Label>
                <Input
                  id="closing-note"
                  value={closingNote}
                  maxBytes={2000}
                  aria-invalid={errorField === "allowPartial" || undefined}
                  aria-describedby={
                    errorField === "allowPartial" ? "finish-error" : undefined
                  }
                  onChange={(e) => {
                    setClosingNote(e.target.value);
                    if (
                      errorField !== "allowPartial" ||
                      (partial && e.target.value.trim())
                    )
                      clearError();
                  }}
                />
              </FormField>
              <Button
                className="self-end"
                disabled={busy || utf8ByteLength(closingNote) > 2000}
                onClick={() =>
                  void preview({
                    ...reconciliationAction("finish", current),
                    allowPartial: partial,
                    note: closingNote,
                  })
                }
              >
                {t("reconciliation.preview")}
              </Button>
            </div>
          </SheetContent>
        </Sheet>
      )}
      {editing && editing.route !== "newProduct" && current && (
        <Sheet
          open
          onOpenChange={(open) => {
            if (!open) {
              invalidatePreview();
              endEditing();
              clearError();
            }
          }}
        >
          <SheetContent size="lg" finalFocus={() => trigger.current}>
            <SheetHeader>
              <SheetTitle>
                {editing.item.kind === "scope"
                  ? t("reconciliation.kind.scope")
                  : editing.item.label}{" "}
                ·{" "}
                {t(
                  `reconciliation.${editing.route === "check" ? "review" : editing.route === "adjust" ? "adjust" : editing.route === "product" ? "productRoute" : editing.route === "import" ? "importPosition" : "recordMissing"}`,
                )}
              </SheetTitle>
            </SheetHeader>
            {error &&
              (editing.route !== "check" ||
                ![
                  "source",
                  "asOfDate",
                  "asOfTime",
                  "note",
                  "outcome",
                  "referenceValue",
                  "referenceComparable",
                ].includes(errorField ?? "")) && (
                <FieldError>{error}</FieldError>
              )}
            {editing.route === "check" ? (
              <CheckForm
                key={editing.item.id}
                item={editing.item}
                error={error}
                errorField={errorField}
                onEdit={clearError}
                disabled={busy}
                onPreview={(fields) =>
                  preview({
                    ...reconciliationAction("check", current),
                    accountId: editing.account.accountId,
                    itemId: editing.item.id,
                    financialRevision: editing.account.financialRevision,
                    ...fields,
                  })
                }
              />
            ) : editing.route === "record" || editing.route === "import" ? (
              <RecordChangeForm
                lock={
                  editing.route === "import"
                    ? {
                        kind: ChangeCommandKind.ChangePositionImport,
                        hideKind: true,
                        accountId: editing.account.accountId,
                      }
                    : undefined
                }
                initial={{
                  ...emptyChangeRequest(
                    editing.route === "import"
                      ? ChangeCommandKind.ChangePositionImport
                      : editing.item.kind === "holding"
                        ? ChangeCommandKind.ChangeTrade
                        : ChangeCommandKind.ChangeMoneyAdded,
                    editing.item.currency || editing.account.currency,
                  ),
                  accountId: editing.account.accountId,
                  holdingId: editing.item.holdingId,
                  instrumentId: editing.item.instrumentId,
                  side: editing.item.kind === "holding" ? "buy" : undefined,
                  settlementAccountId: editing.account.accountId,
                  reconciliationLink: {
                    sessionId: current.id,
                    accountId: editing.account.accountId,
                    itemId: editing.item.id,
                    expectedRevision: current.revision,
                    financialRevision: editing.account.financialRevision,
                  },
                }}
                onRecorded={() => {
                  endEditing();
                  refresh();
                }}
              />
            ) : editing.route === "product" ? (
              <ProductCorrection
                productId={editing.item.productId!}
                link={{
                  sessionId: current.id,
                  accountId: editing.account.accountId,
                  itemId: editing.item.id,
                  expectedRevision: current.revision,
                  financialRevision: editing.account.financialRevision,
                }}
                onPreview={(p) => { if (editing.requestId === editingRequestId.current) setReview({ planId: p.planId, product: p }); }}
              />
            ) : (
              <AdjustmentForm
                key={`${current.id}:${editing.account.accountId}:${editing.item.id}`}
                session={current}
                account={editing.account}
                item={editing.item}
                onPreview={(p) => { if (editing.requestId === editingRequestId.current) setReview({ planId: p.planId, adjustment: p }); }}
              />
            )}
          </SheetContent>
        </Sheet>
      )}
      {editing?.route === "newProduct" && current && (
        <ProductFormSheet
          open
          accountId={editing.account.accountId}
          accountName={editing.account.name}
          currency={editing.account.currency}
          reconciliationLink={{
            sessionId: current.id,
            accountId: editing.account.accountId,
            itemId: editing.item.id,
            expectedRevision: current.revision,
            financialRevision: editing.account.financialRevision,
          }}
          onOpenChange={(open) => {
            if (!open) {
              endEditing();
              clearError();
            }
          }}
          onReconciliationPreview={(p) => { if (editing.requestId === editingRequestId.current) setReview({ planId: p.planId, product: p }); }}
        />
      )}
      {review && (
        <Sheet open onOpenChange={(open) => !open && setReview(undefined)}>
          <SheetContent size="lg">
            <SheetHeader>
              <SheetTitle>{t("reconciliation.preview")}</SheetTitle>
            </SheetHeader>
            <CommitReview
              review={review}
              onDone={(result) => {
                if (review.adjustment || review.product)
                  invalidateActivityChange(client);
                setReview(undefined);
                setClosing(false);
                setPartial(false);
                setClosingNote("");
                endEditing();
                if (result) setActive(result.id);
                refresh();
              }}
            />
          </SheetContent>
        </Sheet>
      )}
    </div>
  );
}
function ProductEvidence({ detail }: { detail: unknown }) {
  const { t } = useTranslation();
  const v = detail as {
    startOn?: string;
    maturityOn?: string;
    state?: string;
    interestMode?: string;
    annualRate?: string;
    maturityInterest?: string;
    interestPaidThroughOn?: string;
    note?: string;
    currency?: string;
  };
  const modeKey = (
    {
      none: "interestNone",
      manual_maturity_amount: "interestManual",
      simple_act_365: "interestSimple365",
      simple_act_360: "interestSimple360",
    } as Record<string, string>
  )[v?.interestMode ?? ""];
  return (
    <div className="num flex flex-col gap-1 rounded-xl bg-muted/50 px-3 py-2.5 text-sm">
      <p>
        {t("availableFunds.startOn")}: {formatRecordedDate(v?.startOn)} ·{" "}
        {t("availableFunds.maturityOn")}:{" "}
        {formatRecordedDate(v?.maturityOn) || t("reconciliation.unavailable")} ·{" "}
        {t(`reconciliation.productState.${v?.state}`, {
          defaultValue: t("reconciliation.unavailable"),
        })}
      </p>
      <p>
        {t("availableFunds.interestMode")}:{" "}
        {modeKey
          ? t(`availableFunds.${modeKey}`)
          : t("reconciliation.unavailable")}
      </p>
      {v?.annualRate && (
        <p>
          {t("availableFunds.annualRatePercent")}:{" "}
          {formatRecordedRate(v.annualRate)}
        </p>
      )}
      {v?.maturityInterest && (
        <p>
          {t("availableFunds.maturityInterest")}:{" "}
          {formatRecordedAmount(v.maturityInterest, v.currency)}
        </p>
      )}
      {v?.interestPaidThroughOn && (
        <p>
          {t("availableFunds.interestPaidThrough")}: {formatRecordedDate(v.interestPaidThroughOn)}
        </p>
      )}
      {v?.note && <p className="whitespace-pre-wrap break-words">{v.note}</p>}
    </div>
  );
}

function CheckForm({
  item,
  onPreview,
  disabled,
  error,
  errorField,
  onEdit,
}: {
  error?: string;
  errorField?: string;
  onEdit: () => void;
  item: ReconciliationItem;
  onPreview: (fields: Partial<ReconciliationAction>) => Promise<void>;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const history = useHistoryOrigin();
  const today = localDateTimeInTimeZone(history.data?.timezone ?? "UTC")?.date;
  const [outcome, setOutcome] = useState("confirmed");
  const [reference, setReference] = useState(item.source.reference);
  const [date, setDate] = useState(item.source.asOfDate);
  const [time, setTime] = useState(item.source.asOfTime ?? "");
  const [note, setNote] = useState(item.note);
  const [value, setValue] = useState(item.referenceValue ?? "");
  const [comparable, setComparable] = useState(
    item.referenceComparable ?? false,
  );
  const numeric = /^-?\d+(\.\d+)?$/;
  const mismatch =
    comparable &&
    date === today &&
    numeric.test(value.trim()) &&
    numeric.test(item.value ?? "") &&
    compareCanonical(value.trim(), item.value!) !== 0;
  const invalid = (field: string) => errorField === field;
  const aria = (field: string) => ({
    "aria-invalid": invalid(field) || undefined,
    "aria-describedby": invalid(field) ? `check-${field}-error` : undefined,
  });
  const fieldError = (field: string) => (
    <FieldError id={`check-${field}-error`}>
      {invalid(field) ? error : undefined}
    </FieldError>
  );
  return (
    <div className="flex flex-col gap-4">
      <FormField>
        <Label htmlFor="check-outcome">{t("reconciliation.outcome")}</Label>
        <NativeSelect
          id="check-outcome"
          value={outcome}
          {...aria("outcome")}
          onChange={(e) => {
            setOutcome(e.target.value);
            onEdit();
          }}
        >
          {["confirmed", "skipped", "unresolved", "pending"].map((s) => (
            <option key={s} value={s}>
              {t(`reconciliation.${s}`)}
            </option>
          ))}
        </NativeSelect>
        {fieldError("outcome")}
      </FormField>
      <FormField>
        <Label htmlFor="check-source">{t("reconciliation.source")}</Label>
        <Input
          id="check-source"
          maxBytes={512}
          value={reference}
          {...aria("source")}
          onChange={(e) => {
            setReference(e.target.value);
            onEdit();
          }}
        />
        {fieldError("source")}
      </FormField>
      <div className="flex flex-col gap-4">
        <FormField>
          <Label htmlFor="check-date">{t("reconciliation.sourceDate")}</Label>
          <DatePicker
            id="check-date"
            value={date}
            max={today}
            onChange={(value) => {
              setDate(value);
              onEdit();
            }}
            {...aria("asOfDate")}
            clearable
          />
          {fieldError("asOfDate")}
        </FormField>
        <FormField>
          <Label htmlFor="check-time">{t("reconciliation.sourceTime")}</Label>
          <TimePicker
            id="check-time"
            value={time}
            onChange={(value) => {
              setTime(value);
              onEdit();
            }}
            {...aria("asOfTime")}
            clearable
          />
          {fieldError("asOfTime")}
        </FormField>
      </div>
      <FormField>
        <Label htmlFor="check-value">{t("reconciliation.referenceValue")}</Label>
        <Input
          id="check-value"
          className="num"
          {...aria("referenceValue")}
          value={value}
          maxBytes={256}
          onChange={(e) => {
            setValue(e.target.value);
            onEdit();
          }}
        />
        {fieldError("referenceValue")}
      </FormField>
      {["cash", "balance", "holding"].includes(item.kind) && (
        <div className="flex flex-col gap-2 rounded-xl border border-border bg-muted/30 p-3">
          <label className="flex cursor-pointer items-start gap-3 text-sm font-medium">
            <input
              type="checkbox"
              className="mt-0.5 size-4 shrink-0 accent-primary"
              checked={comparable}
              {...aria("referenceComparable")}
              onChange={(e) => {
                setComparable(e.target.checked);
                onEdit();
              }}
            />
            {t("reconciliation.comparable")}
          </label>
          {fieldError("referenceComparable")}
          <p className="text-xs leading-5 text-muted-foreground">
            {t("reconciliation.comparisonHelp")}
          </p>
          {mismatch && (
            <Notice tone="warning" role="status">
              {t("reconciliation.mismatch")}
            </Notice>
          )}
        </div>
      )}
      <FormField>
        <Label htmlFor="check-note">{t("reconciliation.note")}</Label>
        <Input
          id="check-note"
          value={note}
          maxBytes={2000}
          {...aria("note")}
          onChange={(e) => {
            setNote(e.target.value);
            onEdit();
          }}
        />
        {fieldError("note")}
      </FormField>
      <Button
        className="self-end"
        disabled={
          disabled ||
          utf8ByteLength(reference) > 512 ||
          utf8ByteLength(value) > 256 ||
          utf8ByteLength(note) > 2000
        }
        onClick={() =>
          void onPreview({
            outcome,
            source: { reference, asOfDate: date, asOfTime: time },
            referenceValue: value,
            referenceComparable: comparable,
            note,
          })
        }
      >
        {t("reconciliation.preview")}
      </Button>
    </div>
  );
}
function AdjustmentForm({
  session,
  account,
  item,
  onPreview,
}: {
  session: ReconciliationSession;
  account: ReconciliationAccount;
  item: ReconciliationItem;
  onPreview: (p: AdjustmentPreview) => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [value, setValue] = useState(
    item.kind === "scope" ? "" : (item.value ?? ""),
  );
  const [currency, setCurrency] = useState(item.currency ?? account.currency);
  const [unit, setUnit] = useState("");
  const [total, setTotal] = useState("");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const previewSequence = useRef(0);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => { alive.current = false; previewSequence.current += 1; };
  }, []);
  const [errorField, setErrorField] = useState("");
  const [costChoiceError, setCostChoiceError] = useState(false);
  const clearCostChoiceError = (nextUnit: string, nextTotal: string) => {
    const validDecimal = /^(0|[1-9][0-9]*)(\.[0-9]+)?$/;
    const hasOne = Boolean(nextUnit) !== Boolean(nextTotal);
    if (costChoiceError && hasOne && validDecimal.test(nextUnit || nextTotal)) {
      setError("");
      setErrorField("");
      setCostChoiceError(false);
    }
  };
  const invalid = (field: string) => errorField === field;
  const aria = (field: string) => ({
    "aria-invalid": invalid(field) || undefined,
    "aria-describedby": invalid(field) ? `adjust-${field}-error` : undefined,
  });
  const fieldError = (field: string) => (
    <FieldError id={`adjust-${field}-error`}>
      {invalid(field) ? error : undefined}
    </FieldError>
  );
  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm leading-6 text-muted-foreground">
        {t("reconciliation.correctionHelp")}
      </p>
      <p className="num break-words rounded-xl bg-muted/50 px-3 py-2.5 text-sm font-medium">
        {t("reconciliation.current")}: {checkpointValue(t, item, true)} →{" "}
        {value || t("reconciliation.unavailable")}
      </p>
      {item.kind === "scope" && (
        <FormField>
          <Label htmlFor="adjust-currency">{t("history.currency")}</Label>
          <Input
            id="adjust-currency"
            maxLength={3}
            value={currency}
            onChange={(e) => setCurrency(e.target.value.toUpperCase())}
          />
        </FormField>
      )}
      <FormField>
        <Label htmlFor="adjust-value">{t("reconciliation.target")}</Label>
        <Input
          id="adjust-value"
          className="num"
          inputMode="decimal"
          value={value}
          onChange={(e) => setValue(e.target.value)}
        />
      </FormField>
      {item.kind === "holding" && (
        <>
          <FormField>
            <Label htmlFor="adjust-unit">
              {t("reconciliation.addedUnitCost")}
            </Label>
            <Input
              id="adjust-unit"
              className="num"
              {...aria("unitCost")}
              inputMode="decimal"
              value={unit}
              onChange={(e) => {
                setUnit(e.target.value);
                clearCostChoiceError(e.target.value, total);
              }}
            />
            {fieldError("unitCost")}
          </FormField>
          <FormField>
            <Label htmlFor="adjust-total">
              {t("reconciliation.totalCost")}
            </Label>
            <Input
              id="adjust-total"
              className="num"
              {...aria("totalCost")}
              inputMode="decimal"
              value={total}
              onChange={(e) => {
                setTotal(e.target.value);
                clearCostChoiceError(unit, e.target.value);
              }}
            />
            {fieldError("totalCost")}
          </FormField>
        </>
      )}
      <FormField>
        <Label htmlFor="adjust-note">{t("reconciliation.note")}</Label>
        <Input
          id="adjust-note"
          value={note}
          maxBytes={2000}
          onChange={(e) => setNote(e.target.value)}
        />
      </FormField>
      {error && !["unitCost", "totalCost"].includes(errorField) && (
        <Notice tone="danger" role="alert">
          {error}
        </Notice>
      )}
      <Button
        className="self-end"
        disabled={busy || utf8ByteLength(note) > 2000}
        onClick={() => {
          const sequence = ++previewSequence.current;
          setBusy(true);
          void callService(() =>
            ReconciliationService.PreviewAdjustment({
              link: {
                sessionId: session.id,
                accountId: account.accountId,
                itemId: item.id,
                expectedRevision: session.revision,
                financialRevision: account.financialRevision,
              },
              accountId: item.kind === "holding" ? "" : account.accountId,
              holdingId: item.holdingId ?? "",
              balance: item.kind === "holding" ? "" : value,
              currency,
              quantity: item.kind === "holding" ? value : "",
              unitCost: unit,
              totalCost: total,
              note,
            }),
          )
            .then((result) => {
              if (alive.current && sequence === previewSequence.current) onPreview(result);
            })
            .catch((e) => {
              if (!alive.current || sequence !== previewSequence.current) return;
              const wire = (e as { wireError?: WireError })?.wireError;
              const field = (wire?.field ?? "").split(".").slice(-1)[0] ?? "";
              const errorKey = adjustmentErrorKey(wire);
              setCostChoiceError(errorKey === "reconciliation.costChoice");
              setErrorField(field);
              if (knownReconciliationConflict(wire))
                void client.invalidateQueries({ queryKey: ["reconciliation"] });
              setError(
                knownReconciliationConflict(wire)
                  ? t("reconciliation.changedReview")
                  : errorKey
                    ? t(errorKey)
                    : displayError(e, t("reconciliation.error")),
              );
            })
            .finally(() => { if (alive.current && sequence === previewSequence.current) setBusy(false); });
        }}
      >
        {t("reconciliation.preview")}
      </Button>
    </div>
  );
}
function CommitReview({
  review,
  onDone,
}: {
  review: Review;
  onDone: (result?: ReconciliationSession) => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const confirmation = useSheetConfirmation();
  const [busy, setBusy] = useState(false);
  const [unknown, setUnknown] = useState(false);
  const [refusedReview, setRefusedReview] = useState(false);
  const dispatching = useRef(false);
  const [error, setError] = useState("");
  const commit = async () => {
    if (dispatching.current || refusedReview) return;
    if (!confirmation?.begin(review.planId, review) && !unknown) return;
    dispatching.current = true;
    setBusy(true);
    setError("");
    try {
      const result = review.product
        ? await callService(() =>
            ReconciliationService.CommitProduct(review.planId),
          )
        : review.adjustment
          ? await callService(() =>
              ReconciliationService.CommitAdjustment(review.planId),
            )
          : await callService(() =>
              ReconciliationService.Commit(review.planId),
            );
      confirmation?.resolve(review.planId);
      onDone(!result || Array.isArray(result) ? undefined : result);
    } catch (cause) {
      const wire = (cause as { wireError?: WireError })?.wireError;
      const refused =
        (wire?.code === "conflict" &&
          [
            "reconciliationLink",
            "financialRevision",
            "expectedRevision",
            "preview",
            "revision",
          ].includes(wire.field ?? "")) ||
        (wire?.code === "stale_preview" &&
          (["previewToken", "expiresAt"].includes(wire.field ?? "") ||
            (Boolean(review.product) && wire.field === "reviewedStateHash"))) ||
        (wire?.code === "validation" && Boolean(review.result)) ||
        [
          "invalid_change",
          "no_change",
          "insufficient_balance",
          "insufficient_quantity",
          "managed_position_conflict",
        ].includes(wire?.code ?? "");
      if (refused) {
        confirmation?.resolve(review.planId);
        setUnknown(false);
        setRefusedReview(true);
        void client.invalidateQueries({ queryKey: ["reconciliation"] });
      } else {
        setUnknown(true);
      }
      setError(
        completedReconciliation(wire)
          ? t("reconciliation.readOnly")
          : knownReconciliationConflict(wire)
            ? t("reconciliation.changedReview")
            : displayError(cause, t("reconciliation.error")),
      );
    } finally {
      dispatching.current = false;
      setBusy(false);
    }
  };
  return (
    <div className="flex flex-col gap-4">
      {review.result && (
        <>
          <p className="text-base font-semibold">
            {t(`reconciliation.${review.result.status}`)}
          </p>
          <Counts counts={review.result.counts} />
          <p className="break-words text-sm">{review.result.note}</p>
          <p className="break-words text-sm">{review.result.closingNote}</p>
          {review.result.accounts?.map((a) => (
            <p key={a.accountId} className="text-sm font-medium">
              {a.name} · {t(`reconciliation.${a.outcome}`)}
            </p>
          ))}
          {review.result.accounts?.flatMap((a) =>
            (a.items ?? [])
              .filter((item) => item.checkedAt)
              .map((item) => (
                <div
                  key={`${a.accountId}:${item.id}`}
                  className="flex flex-col gap-1 rounded-xl border border-border p-3 text-sm"
                >
                  <p className="font-semibold">
                    {a.name} ·{" "}
                    {item.kind === "scope"
                      ? t("reconciliation.kind.scope")
                      : item.label}{" "}
                    · {t(`reconciliation.${item.outcome}`)}
                  </p>
                  <p className="num break-words">
                    {[checkpointValue(t, item, true), checkpointReference(t, item, true)]
                      .filter(Boolean)
                      .join(" · ")}
                  </p>
                  <p className="break-words text-xs text-muted-foreground">
                    {sourceText(item.source)}
                  </p>
                  <p className="break-words">{item.note}</p>
                </div>
              )),
          )}
        </>
      )}
      {review.adjustment?.changes?.map((change, i) => (
        <div
          key={i}
          className="num flex flex-col gap-1 rounded-xl border border-border p-3 text-sm"
        >
          {change.activity?.note && <p>{change.activity.note}</p>}
          {change.effects?.map((effect, j) => (
            <p key={`effect-${j}`}>
              {t(
                effect.money
                  ? effect.direction === "added"
                    ? "history.detailAmountAdded"
                    : "history.detailAmountRemoved"
                  : effect.direction === "added"
                    ? "history.detailQuantityAdded"
                    : "history.detailQuantityRemoved",
              )}
              : {formatRecordedAmount(effect.money?.amount ?? effect.quantity, effect.money?.currency)}{" "}
              {effect.costUnitPrice &&
                `${t("reconciliation.unitCost")}: ${formatRecordedAmount(effect.costUnitPrice, effect.money?.currency)}`}
            </p>
          ))}
          {change.resulting?.map((v, j) => (
            <p key={j}>
              {v.name} · {formatRecordedAmount(v.amount || v.quantity, v.amount ? v.currency : "")}
            </p>
          ))}
        </div>
      ))}
      {review.product && (
        <>
          <p className="text-base font-semibold">
            {review.product.after?.product.name ??
              review.product.before?.product.name}
          </p>
          {review.product.valuation && (
            <p className="num text-sm">
              {t("reconciliation.target")}:{" "}
              {formatRecordedAmount(review.product.valuation.currentValueAfter?.amount, review.product.valuation.currentValueAfter?.currency)}
            </p>
          )}
          {review.product.operation && (
            <ProductOperationReview preview={review.product.operation} />
          )}
          {review.product.before && (
            <details className={detailsClass}>
              <summary className={summaryClass}>
                {t("reconciliation.reviewedBefore")}
              </summary>
              <fieldset
                disabled
                className="flex flex-col gap-3 border-t border-border p-3"
              >
                <ProductTermsFields
                  value={termsFromProduct(review.product.before.product)}
                  onChange={() => {}}
                  editing
                />
                <ProductPolicyFields
                  value={policyFromDTO(review.product.before.product.policy)}
                  onChange={() => {}}
                />
              </fieldset>
            </details>
          )}
          {review.product.after && (
            <fieldset disabled className="flex flex-col gap-3">
              <ProductTermsFields
                value={termsFromProduct(review.product.after.product)}
                onChange={() => {}}
                editing
              />
              <ProductPolicyFields
                value={policyFromDTO(review.product.after.product.policy)}
                onChange={() => {}}
              />
            </fieldset>
          )}
        </>
      )}
      <Notice>{t("reconciliation.explicitReview")}</Notice>
      {review.action === "finish" && (
        <Notice>{t("reconciliation.finishNotice")}</Notice>
      )}
      {error && (
        <Notice tone="danger" role="alert">
          {error}
        </Notice>
      )}
      <Button
        className="self-end"
        disabled={busy}
        aria-disabled={refusedReview || undefined}
        onClick={() => void commit()}
      >
        {t(
          unknown
            ? "reconciliation.retrySame"
            : review.action === "pause"
              ? "reconciliation.pause"
              : review.action === "resume"
                ? "reconciliation.resume"
                : review.action === "finish"
                  ? "reconciliation.finish"
                  : review.action === "start"
                    ? "reconciliation.start"
                    : "reconciliation.commit",
        )}
      </Button>
    </div>
  );
}
type LinkedAudit = {
  reviewedBefore?: {
    accountName: string;
    label: string;
    kind: string;
    value?: string;
    cost?: string;
    currency?: string;
    detail?: unknown;
    source?: { reference: string; asOfDate: string; asOfTime?: string };
    note?: string;
  };
  committed?: {
    kind?: string;
    effectiveAt?: string;
    recordedAt?: string;
    note?: string;
    resulting?: {
      Name: string;
      Amount?: string;
      Quantity?: string;
      Currency?: string;
    }[];
    effects?: {
      direction: string;
      amount?: string;
      currency?: string;
      quantity?: string;
      unitCost?: string;
    }[];
    contract?: {
      name: string;
      principal: string;
      currency: string;
      maturityOn?: string;
      annualRate?: string;
    };
    contracts?: {
      name: string;
      principal: string;
      currency: string;
      maturityOn?: string;
      annualRate?: string;
    }[];
  };
};
function LinkedChanges({ sessionId }: { sessionId: string }) {
  const { t } = useTranslation();
  const formatTimestamp = useReconciliationTimestamp();
  const links = useReconciliationLinks(sessionId);
  const [page, setPage] = useState(0);
  return (
    <details className={cn(detailsClass, "shadow-sm")}>
      <summary className={cn(summaryClass, "px-5 py-3.5 text-sm")}>
        {t("reconciliation.linkedChanges")}
      </summary>
      <div className="flex flex-col border-t border-border">
      {links.isError ? (
        <div role="alert" className="flex flex-wrap items-center gap-3 px-5 py-4">
          <p className="text-sm text-destructive">{t("reconciliation.error")}</p>
          <Button variant="outline" size="sm" onClick={() => void links.refetch()}>
            {t("reconciliation.retry")}
          </Button>
        </div>
      ) : (
        links.data?.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE).map((link, i) => {
          const evidence = link.evidence as unknown as LinkedAudit;
          const before = evidence.reviewedBefore;
          const after = evidence.committed;
          return (
            <article
              key={i}
              className="num flex flex-col gap-1.5 border-t border-border px-5 py-4 text-sm first:border-t-0"
            >
              <h4 className="font-semibold">
                {t("reconciliation.linkedChange", { count: page * 25 + i + 1 })}{" "}
                · {before?.accountName} ·{" "}
                {before?.kind === "scope"
                  ? t("reconciliation.kind.scope")
                  : before?.label}
              </h4>
              <p>
                {t("reconciliation.reviewedBefore")}:{" "}
                {before && checkpointValue(t, before, true)}{" "}
                {before?.cost &&
                  `${t("reconciliation.unitCost")}: ${formatRecordedAmount(before.cost, before.currency)}`}
              </p>
              {sourceText(before?.source) && (
                <p>{sourceText(before?.source)}</p>
              )}
              {after?.recordedAt && (
                <p>
                  {t("reconciliation.recordedAt")}:{" "}
                  {formatTimestamp(after.recordedAt)}
                </p>
              )}
              {after?.effectiveAt && (
                <p>
                  {t("reconciliation.effectiveAt")}:{" "}
                  {formatTimestamp(after.effectiveAt)}
                </p>
              )}
              {after?.resulting?.map((value, j) => (
                <p key={j}>
                  {value.Name}: {formatRecordedAmount(value.Amount || value.Quantity, value.Amount ? value.Currency : "")}
                </p>
              ))}
              {after?.effects?.map((effect, j) => (
                <p key={j}>
                  {t(
                    effect.amount != null
                      ? effect.direction === "added"
                        ? "history.detailAmountAdded"
                        : "history.detailAmountRemoved"
                      : effect.direction === "added"
                        ? "history.detailQuantityAdded"
                        : "history.detailQuantityRemoved",
                  )}
                  : {formatRecordedAmount(effect.amount ?? effect.quantity, effect.amount ? effect.currency : "")}{" "}
                  {effect.unitCost &&
                    `${t("reconciliation.unitCost")}: ${formatRecordedAmount(effect.unitCost, effect.currency)}`}
                </p>
              ))}
              {[
                ...(after?.contracts ?? []),
                ...(after?.contract ? [after.contract] : []),
              ].map((contract, j) => (
                <p key={j}>
                  {contract.name}: {formatRecordedAmount(contract.principal, contract.currency)} ·{" "}
                  {formatRecordedDate(contract.maturityOn)} · {formatRecordedRate(contract.annualRate)}
                </p>
              ))}
              {after?.note && <p>{after.note}</p>}
            </article>
          );
        })
      )}
      {!links.isLoading && !links.isError && !links.data?.length && (
        <p className="px-5 py-4 text-sm text-muted-foreground">
          {t("reconciliation.noLinkedChanges")}
        </p>
      )}
      <Pager
        page={page}
        total={links.data?.length ?? 0}
        onPage={setPage}
      />
      </div>
    </details>
  );
}

function RecentChanges({
  sessionId,
  session,
}: {
  sessionId: string;
  session: ReconciliationSession;
}) {
  const { t } = useTranslation();
  const formatTimestamp = useReconciliationTimestamp();
  const changes = useReconciliationChanges(sessionId);
  const [page, setPage] = useState(0);
  return (
    <details className={cn(detailsClass, "shadow-sm")}>
      <summary className={cn(summaryClass, "px-5 py-3.5 text-sm")}>
        {t("reconciliation.recentChanges")}
      </summary>
      <div className="flex flex-col border-t border-border">
      {session.accounts
        ?.filter((a) => !a.priorSuccessfulAt)
        .map((a) => (
          <p key={a.accountId} className="px-5 py-3 text-sm text-muted-foreground">
            {a.name}: {t("reconciliation.firstBaseline")}
          </p>
        ))}
      {changes.isError ? (
        <div role="alert" className="flex flex-wrap items-center gap-3 px-5 py-4">
          <p className="text-sm text-destructive">{t("reconciliation.error")}</p>
          <Button variant="outline" size="sm" onClick={() => void changes.refetch()}>
            {t("reconciliation.retry")}
          </Button>
        </div>
      ) : (
        changes.data?.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE).map((change, i) => {
          const evidence = change.evidence as unknown as {
            accountName?: string;
            productName?: string;
            kind?: string;
            amount?: string;
            currency?: string;
            quantity?: string;
          };
          return (
            <p key={i} className="num break-words border-t border-border px-5 py-3 text-sm first:border-t-0">
              {[
                change.afterCompletion
                  ? t("reconciliation.afterCompletion")
                  : t("reconciliation.originalInterval"),
                evidence.accountName,
                evidence.productName,
                formatRecordedAmount(evidence.amount ?? evidence.quantity, evidence.amount ? evidence.currency : ""),
                `${t("reconciliation.recordedAt")}: ${formatTimestamp(change.recordedAt)}`,
                `${t("reconciliation.effectiveAt")}: ${formatTimestamp(change.effectiveAt)}`,
              ]
                .filter(Boolean)
                .join(" · ")}
            </p>
          );
        })
      )}
      {!changes.isLoading && !changes.isError && !changes.data?.length && (
        <p className="px-5 py-4 text-sm text-muted-foreground">
          {t("reconciliation.noChanges")}
        </p>
      )}
      <Pager
        page={page}
        total={changes.data?.length ?? 0}
        onPage={setPage}
      />
      </div>
    </details>
  );
}

function HoldingIdentity({ detail }: { detail: unknown }) {
  const value = detail as {
    name?: string;
    symbol?: string;
    isin?: string;
    marketCode?: string;
    quantityUnit?: string;
  };
  return (
    <p className="break-words text-xs text-muted-foreground">
      {[
        value?.name,
        value?.symbol,
        value?.isin,
        value?.marketCode,
        value?.quantityUnit,
      ]
        .filter(Boolean)
        .join(" · ")}
    </p>
  );
}
