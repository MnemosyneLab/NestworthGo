import { ReconciliationCoverageNotice } from "./ReconciliationCoverageNotice";
import { useHistoryOrigin } from "@/queries/history";
import { localDateTimeInTimeZone } from "@/lib/time";
import {
  adjustmentErrorKey,
  knownReconciliationConflict,
  completedReconciliation,
} from "./reconciliationErrors";
import { Badge } from "@/components/ui/badge";
import { checkpointValue, sourceText } from "./reconciliationDisplay";
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
import { useEffect, useRef, useState } from "react";
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
import { PageChrome } from "@/components/layout/PageChrome";
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
  formatAmount,
  formatQuotePrice,
  multiplyCanonical,
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
}: {
  accountId?: string;
  sessionId?: string;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const formatTimestamp = useReconciliationTimestamp();
  const accounts = useAccounts();
  const sessions = useReconciliationSessions(accountId);
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
  const rows = (sessions.data ?? []).filter(
    (s) => !filter || s.status === filter,
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
    <div className="flex min-w-0 flex-col gap-5">
      <PageChrome pageId="reconciliation" title={t("reconciliation.title")} />
      <p className="text-sm text-muted-foreground">
        {t("reconciliation.localOnly")}
      </p>
      {error && !editing && !closing && <p role="alert">{error}</p>}
      {!active ? (
        <>
          {accounts.isLoading || sessions.isLoading ? (
            <LoadingState label={t("reconciliation.loading")} />
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
                  <div
                    key={s.id}
                    className="rounded-xl border border-border p-4"
                  >
                    <p>
                      {s.accountNames?.join(" · ")} ·{" "}
                      {t(`reconciliation.${s.status}`)}
                    </p>
                    <Counts counts={s.counts} />
                    <Button onClick={() => select(s.id)}>
                      {t("reconciliation.continue")}
                    </Button>
                  </div>
                ))}
              {!activeAccounts.length && (
                <EmptyState title={t("reconciliation.noAccounts")} />
              )}
              <fieldset className="rounded-xl border border-border p-4">
                <legend>{t("reconciliation.selectAccounts")}</legend>
                <Button
                  variant="ghost"
                  onClick={() =>
                    setSelected(activeAccounts.map((a) => a.account.id))
                  }
                >
                  {t("reconciliation.selectAll")}
                </Button>
                {activeAccounts.map((a) => (
                  <label
                    key={a.account.id}
                    className="flex items-center gap-2 py-2"
                  >
                    <input
                      type="checkbox"
                      checked={chosen.includes(a.account.id)}
                      onChange={(e) =>
                        setSelected(
                          e.target.checked
                            ? [...chosen, a.account.id]
                            : chosen.filter((id) => id !== a.account.id),
                        )
                      }
                    />
                    {a.account.name} · {a.account.defaultCurrency}
                  </label>
                ))}
                {(sessions.data ?? []).some(
                  (s) =>
                    s.status !== "completed" &&
                    s.accountIds?.some((id) => chosen.includes(id)),
                ) && <p role="status">{t("reconciliation.overlap")}</p>}
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
              </fieldset>
              <Label htmlFor="reconciliation-filter">
                {t("reconciliation.history")}
              </Label>
              <NativeSelect
                id="reconciliation-filter"
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
              {!rows.length && (
                <EmptyState
                  title={t(
                    filter
                      ? "reconciliation.noMatches"
                      : "reconciliation.empty",
                  )}
                />
              )}
              {rows.slice(page * 25, (page + 1) * 25).map((s) => (
                <Button
                  key={s.id}
                  variant="outline"
                  className="h-auto justify-start whitespace-normal py-3 text-left"
                  onClick={() => select(s.id)}
                >
                  {s.accountNames?.join(" · ")} · {formatTimestamp(s.startedAt)}{" "}
                  · {t(`reconciliation.${s.status}`)}
                  {s.partial ? ` · ${t("reconciliation.partial")}` : ""}
                </Button>
              ))}
              <div className="flex gap-2">
                <Button
                  variant="ghost"
                  disabled={page === 0}
                  onClick={() => setPage(page - 1)}
                >
                  {t("reconciliation.previous")}
                </Button>
                <Button
                  variant="ghost"
                  disabled={(page + 1) * 25 >= rows.length}
                  onClick={() => setPage(page + 1)}
                >
                  {t("reconciliation.next")}
                </Button>
              </div>
            </>
          )}
        </>
      ) : (
        <>
          <Button variant="ghost" onClick={() => select("")}>
            {t("reconciliation.back")}
          </Button>
          {detail.isLoading && !current ? (
            <LoadingState label={t("reconciliation.loading")} />
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
                  <p role="alert">{t("reconciliation.stale")}</p>
                )}
                <h2 className="text-lg font-semibold">
                  {formatTimestamp(current.startedAt)} ·{" "}
                  {t(`reconciliation.${current.status}`)}
                </h2>
                <Counts counts={current.counts} />
                {current.note && (
                  <p>
                    {t("reconciliation.openingNote")}: {current.note}
                  </p>
                )}
                {current.closingNote && !current.partial && (
                  <p>
                    {t("reconciliation.closingNote")}: {current.closingNote}
                  </p>
                )}
                {current.status === "completed" && (
                  <p>{t("reconciliation.readOnly")}</p>
                )}
                {current.partial && (
                  <p>
                    {t("reconciliation.partial")}: {current.closingNote}
                  </p>
                )}
                {current.completedAt && (
                  <p>
                    {t("reconciliation.completedAt")}:{" "}
                    {formatTimestamp(current.completedAt)}
                  </p>
                )}
                {current.status !== "completed" && (
                  <div className="flex flex-wrap gap-2">
                    <Button
                      disabled={busy}
                      variant="outline"
                      onClick={() =>
                        void preview(
                          reconciliationAction(
                            current.status === "paused" ? "resume" : "pause",
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
                {(current.accounts ?? []).map((a) => (
                  <section
                    key={a.accountId}
                    className="flex min-w-0 flex-col gap-3 rounded-xl border border-border p-4"
                  >
                    <h3 className="font-semibold">
                      {a.name} · {a.currency}
                    </h3>
                    <Counts counts={a.counts} />
                    <p className="text-sm text-muted-foreground">
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
                      <p role="status">
                        {t(
                          `reconciliation.${current.status === "completed" ? "recheck" : a.financialRevisionChanged ? "workingBookkeepingChanged" : "pendingReview"}`,
                        )}
                      </p>
                    )}
                    {[...(a.items ?? [])]
                      .sort(
                        (left, right) =>
                          Number(right.kind === "scope") -
                          Number(left.kind === "scope"),
                      )
                      .map((item) => (
                        <article
                          key={item.id}
                          className="flex min-w-0 flex-col gap-2 border-t border-border pt-3"
                        >
                          <div className="flex flex-wrap justify-between gap-2">
                            <h4>
                              {item.kind === "scope"
                                ? t("reconciliation.scope")
                                : item.label}{" "}
                              · {t(`reconciliation.kind.${item.kind}`)}
                            </h4>
                            <Badge
                              variant={
                                item.outcome === "confirmed"
                                  ? "success"
                                  : item.outcome === "unresolved"
                                    ? "warning"
                                    : "secondary"
                              }
                            >
                              {t(`reconciliation.${item.outcome}`)}
                            </Badge>
                          </div>
                          {item.kind === "scope" && (
                            <p className="text-sm text-muted-foreground">
                              {t("reconciliation.scopeHelp")}
                            </p>
                          )}
                          {item.archivedNonzero && (
                            <p role="status">
                              {t("reconciliation.archivedNonzero")}
                            </p>
                          )}
                          {archivedTarget(a, item) &&
                            current.status !== "completed" && (
                              <p role="status">
                                {t("reconciliation.restoreBeforeCorrection")}
                              </p>
                            )}
                          {item.kind !== "scope" && (
                            <p className="break-words font-mono">
                              {checkpointValue(t, item)}
                            </p>
                          )}
                          {item.cost && (
                            <p>
                              {t("reconciliation.unitCost")}:{" "}
                              {formatQuotePrice(item.cost, item.currency)}
                            </p>
                          )}
                          {item.kind === "holding" && (
                            <HoldingIdentity detail={item.detail} />
                          )}
                          {item.kind === "product" && (
                            <ProductEvidence detail={item.detail} />
                          )}
                          {item.checkedAt && (
                            <p className="text-sm">
                              {[
                                sourceText(item.source),
                                `${t("reconciliation.checkedAt")}: ${formatTimestamp(item.checkedAt)}`,
                              ]
                                .filter(Boolean)
                                .join(" · ")}
                            </p>
                          )}
                          {item.referenceValue && (
                            <p>
                              {t("reconciliation.referenceValue")}:{" "}
                              {item.referenceValue}
                            </p>
                          )}
                          {item.outcome === "confirmed" && item.referenceComparable &&
                            /^-?\d+(\.\d+)?$/.test(item.referenceValue ?? "") &&
                            /^-?\d+(\.\d+)?$/.test(item.value ?? "") &&
                            compareCanonical(
                              item.referenceValue!,
                              item.value!,
                            ) !== 0 && (
                              <p role="status">
                                {t("reconciliation.mismatch")}
                              </p>
                            )}
                          {item.note && (
                            <p className="whitespace-pre-wrap break-words">
                              {item.note}
                            </p>
                          )}
                          {current.status === "in_progress" && (
                            <div className="flex flex-wrap gap-2">
                              <Button
                                variant="outline"
                                onClick={(e) => {
                                  trigger.current = e.currentTarget;
                                  beginEditing({
                                    account: a,
                                    item,
                                    route: "check",
                                  });
                                }}
                              >
                                {t("reconciliation.review")}
                              </Button>
                              {["cash", "balance", "holding"].includes(
                                item.kind,
                              ) &&
                                !archivedTarget(a, item) && (
                                  <Button
                                    variant="ghost"
                                    onClick={(e) => {
                                      trigger.current = e.currentTarget;
                                      beginEditing({
                                        account: a,
                                        item,
                                        route: "adjust",
                                      });
                                    }}
                                  >
                                    {t("reconciliation.adjust")}
                                  </Button>
                                )}
                              {item.kind === "product" && (
                                <Button
                                  variant="ghost"
                                  onClick={(e) => {
                                    trigger.current = e.currentTarget;
                                    beginEditing({
                                      account: a,
                                      item,
                                      route: "product",
                                    });
                                  }}
                                >
                                  {t("reconciliation.productRoute")}
                                </Button>
                              )}
                              {item.kind === "scope" &&
                                a.mode === "holdings" && (
                                  <Button
                                    variant="ghost"
                                    onClick={(e) => {
                                      trigger.current = e.currentTarget;
                                      beginEditing({
                                        account: a,
                                        item,
                                        route: "newProduct",
                                      });
                                    }}
                                  >
                                    {t("reconciliation.addProduct")}
                                  </Button>
                                )}
                              {item.kind === "scope" &&
                                a.mode === "holdings" && (
                                  <Button
                                    variant="ghost"
                                    onClick={(e) => {
                                      trigger.current = e.currentTarget;
                                      beginEditing({
                                        account: a,
                                        item,
                                        route: "import",
                                      });
                                    }}
                                  >
                                    {t("reconciliation.importPosition")}
                                  </Button>
                                )}
                              {item.kind !== "product" &&
                                item.kind !== "removed" &&
                                !archivedTarget(a, item) && (
                                  <Button
                                    variant="ghost"
                                    onClick={(e) => {
                                      trigger.current = e.currentTarget;
                                      beginEditing({
                                        account: a,
                                        item,
                                        route: "record",
                                      });
                                    }}
                                  >
                                    {t("reconciliation.recordMissing")}
                                  </Button>
                                )}
                            </div>
                          )}
                          {!!item.attempts?.length && (
                            <details>
                              <summary>
                                {t("reconciliation.attempts", {
                                  count: item.attempts.length,
                                })}
                              </summary>
                              {item.attempts.map((attempt, i) => (
                                <p key={i} className="break-words text-sm">
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
                            </details>
                          )}
                        </article>
                      ))}
                  </section>
                ))}
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
              <p>{t("reconciliation.finishNotice")}</p>
              {error && errorField !== "allowPartial" && (
                <FieldError>{error}</FieldError>
              )}
              <label className="flex gap-2">
                <input
                  type="checkbox"
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
              <Button
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
function Counts({ counts }: { counts: ReconciliationSession["counts"] }) {
  const { t } = useTranslation();
  return (
    <p role="status" className="text-sm">
      {t("reconciliation.counts", counts as unknown as Record<string, number>)}
    </p>
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
    <div className="flex flex-col gap-1 text-sm">
      <p>
        {t("availableFunds.startOn")}: {v?.startOn} ·{" "}
        {t("availableFunds.maturityOn")}:{" "}
        {v?.maturityOn ?? t("reconciliation.unavailable")} ·{" "}
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
          {formatAmount(multiplyCanonical(v.annualRate, "100"))}%
        </p>
      )}
      {v?.maturityInterest && (
        <p>
          {t("availableFunds.maturityInterest")}:{" "}
          {formatAmount(v.maturityInterest, v.currency)} {v.currency}
        </p>
      )}
      {v?.interestPaidThroughOn && (
        <p>
          {t("availableFunds.interestPaidThrough")}: {v.interestPaidThroughOn}
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
    <div className="flex flex-col gap-3">
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
      <Label htmlFor="check-date">{t("reconciliation.sourceDate")}</Label>
      <DatePicker
        id="check-date"
        value={date}
        onChange={(value) => {
          setDate(value);
          onEdit();
        }}
        {...aria("asOfDate")}
        clearable
      />
      {fieldError("asOfDate")}
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
      <Label htmlFor="check-value">{t("reconciliation.referenceValue")}</Label>
      <Input
        id="check-value"
        {...aria("referenceValue")}
        value={value}
        maxBytes={256}
        onChange={(e) => {
          setValue(e.target.value);
          onEdit();
        }}
      />
      {fieldError("referenceValue")}
      {["cash", "balance", "holding"].includes(item.kind) && (
        <>
          <label className="flex gap-2">
            <input
              type="checkbox"
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
          <p className="text-sm text-muted-foreground">
            {t("reconciliation.comparisonHelp")}
          </p>
          {mismatch && <p role="status">{t("reconciliation.mismatch")}</p>}
        </>
      )}
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
      <Button
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
    <div className="flex flex-col gap-3">
      <p>{t("reconciliation.correctionHelp")}</p>
      <p>
        {t("reconciliation.current")}: {checkpointValue(t, item, true)} →{" "}
        {value || t("reconciliation.unavailable")}
      </p>
      {item.kind === "scope" && (
        <>
          <Label htmlFor="adjust-currency">{t("history.currency")}</Label>
          <Input
            id="adjust-currency"
            maxLength={3}
            value={currency}
            onChange={(e) => setCurrency(e.target.value.toUpperCase())}
          />
        </>
      )}
      <Label htmlFor="adjust-value">{t("reconciliation.target")}</Label>
      <Input
        id="adjust-value"
        inputMode="decimal"
        value={value}
        onChange={(e) => setValue(e.target.value)}
      />
      {item.kind === "holding" && (
        <>
          <Label htmlFor="adjust-unit">
            {t("reconciliation.addedUnitCost")}
          </Label>
          <Input
            id="adjust-unit"
            {...aria("unitCost")}
            inputMode="decimal"
            value={unit}
            onChange={(e) => {
              setUnit(e.target.value);
              clearCostChoiceError(e.target.value, total);
            }}
          />
          {fieldError("unitCost")}
          <Label htmlFor="adjust-total">{t("reconciliation.totalCost")}</Label>
          <Input
            id="adjust-total"
            {...aria("totalCost")}
            inputMode="decimal"
            value={total}
            onChange={(e) => {
              setTotal(e.target.value);
              clearCostChoiceError(unit, e.target.value);
            }}
          />
          {fieldError("totalCost")}
        </>
      )}
      <Label htmlFor="adjust-note">{t("reconciliation.note")}</Label>
      <Input
        id="adjust-note"
        value={note}
        maxBytes={2000}
        onChange={(e) => setNote(e.target.value)}
      />
      {error && !["unitCost", "totalCost"].includes(errorField) && (
        <p role="alert">{error}</p>
      )}
      <Button
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
          <p>{t(`reconciliation.${review.result.status}`)}</p>
          <Counts counts={review.result.counts} />
          <p>{review.result.note}</p>
          <p>{review.result.closingNote}</p>
          {review.result.accounts?.map((a) => (
            <p key={a.accountId}>
              {a.name} · {t(`reconciliation.${a.outcome}`)}
            </p>
          ))}
          {review.result.accounts?.flatMap((a) =>
            (a.items ?? [])
              .filter((item) => item.checkedAt)
              .map((item) => (
                <div
                  key={`${a.accountId}:${item.id}`}
                  className="rounded-lg border border-border p-3"
                >
                  <p>
                    {a.name} ·{" "}
                    {item.kind === "scope"
                      ? t("reconciliation.kind.scope")
                      : item.label}{" "}
                    · {t(`reconciliation.${item.outcome}`)}
                  </p>
                  <p>
                    {[checkpointValue(t, item, true), item.referenceValue]
                      .filter(Boolean)
                      .join(" · ")}
                  </p>
                  <p>{sourceText(item.source)}</p>
                  <p>{item.note}</p>
                </div>
              )),
          )}
        </>
      )}
      {review.adjustment?.changes?.map((change, i) => (
        <div key={i}>
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
              : {effect.money?.amount ?? effect.quantity}{" "}
              {effect.money?.currency}{" "}
              {effect.costUnitPrice &&
                `${t("reconciliation.unitCost")}: ${effect.costUnitPrice}`}
            </p>
          ))}
          {change.resulting?.map((v, j) => (
            <p key={j}>
              {v.name} · {v.amount || v.quantity} {v.amount ? v.currency : ""}
            </p>
          ))}
        </div>
      ))}
      {review.product && (
        <>
          <p>
            {review.product.after?.product.name ??
              review.product.before?.product.name}
          </p>
          {review.product.valuation && (
            <p>
              {t("reconciliation.target")}:{" "}
              {review.product.valuation.currentValueAfter?.amount}{" "}
              {review.product.valuation.currentValueAfter?.currency}
            </p>
          )}
          {review.product.operation && (
            <ProductOperationReview preview={review.product.operation} />
          )}
          {review.product.before && (
            <details>
              <summary>{t("reconciliation.reviewedBefore")}</summary>
              <fieldset disabled className="flex flex-col gap-3">
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
      <p>{t("reconciliation.explicitReview")}</p>
      {review.action === "finish" && <p>{t("reconciliation.finishNotice")}</p>}
      {error && <p role="alert">{error}</p>}
      <Button
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
    <details>
      <summary>{t("reconciliation.linkedChanges")}</summary>
      {links.isError ? (
        <div role="alert">
          <p>{t("reconciliation.error")}</p>
          <Button variant="outline" onClick={() => void links.refetch()}>
            {t("reconciliation.retry")}
          </Button>
        </div>
      ) : (
        links.data?.slice(page * 25, (page + 1) * 25).map((link, i) => {
          const evidence = link.evidence as unknown as LinkedAudit;
          const before = evidence.reviewedBefore;
          const after = evidence.committed;
          return (
            <article
              key={i}
              className="flex flex-col gap-2 border-t border-border py-3 text-sm"
            >
              <h4>
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
                  `${t("reconciliation.unitCost")}: ${before.cost}`}
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
                  {value.Name}: {value.Amount || value.Quantity}{" "}
                  {value.Amount ? value.Currency : ""}
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
                  : {effect.amount ?? effect.quantity}{" "}
                  {effect.amount ? effect.currency : ""}{" "}
                  {effect.unitCost &&
                    `${t("reconciliation.unitCost")}: ${effect.unitCost}`}
                </p>
              ))}
              {[
                ...(after?.contracts ?? []),
                ...(after?.contract ? [after.contract] : []),
              ].map((contract, j) => (
                <p key={j}>
                  {contract.name}: {contract.principal} {contract.currency} ·{" "}
                  {contract.maturityOn} · {contract.annualRate}
                </p>
              ))}
              {after?.note && <p>{after.note}</p>}
            </article>
          );
        })
      )}
      {!links.isLoading && !links.isError && !links.data?.length && (
        <p>{t("reconciliation.noLinkedChanges")}</p>
      )}
      <div className="flex gap-2">
        <Button
          variant="ghost"
          disabled={page === 0}
          onClick={() => setPage(page - 1)}
        >
          {t("reconciliation.previous")}
        </Button>
        <Button
          variant="ghost"
          disabled={(page + 1) * 25 >= (links.data?.length ?? 0)}
          onClick={() => setPage(page + 1)}
        >
          {t("reconciliation.next")}
        </Button>
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
    <details>
      <summary>{t("reconciliation.recentChanges")}</summary>
      {session.accounts
        ?.filter((a) => !a.priorSuccessfulAt)
        .map((a) => (
          <p key={a.accountId}>
            {a.name}: {t("reconciliation.firstBaseline")}
          </p>
        ))}
      {changes.isError ? (
        <div role="alert">
          <p>{t("reconciliation.error")}</p>
          <Button variant="outline" onClick={() => void changes.refetch()}>
            {t("reconciliation.retry")}
          </Button>
        </div>
      ) : (
        changes.data?.slice(page * 25, (page + 1) * 25).map((change, i) => {
          const evidence = change.evidence as unknown as {
            accountName?: string;
            productName?: string;
            kind?: string;
            amount?: string;
            currency?: string;
            quantity?: string;
          };
          return (
            <p key={i} className="break-words text-sm">
              {[
                change.afterCompletion
                  ? t("reconciliation.afterCompletion")
                  : t("reconciliation.originalInterval"),
                evidence.accountName,
                evidence.productName,
                [
                  evidence.amount ?? evidence.quantity,
                  evidence.amount ? evidence.currency : "",
                ]
                  .filter(Boolean)
                  .join(" "),
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
        <p>{t("reconciliation.noChanges")}</p>
      )}
      <div className="flex gap-2">
        <Button
          variant="ghost"
          disabled={page === 0}
          onClick={() => setPage(page - 1)}
        >
          {t("reconciliation.previous")}
        </Button>
        <Button
          variant="ghost"
          disabled={(page + 1) * 25 >= (changes.data?.length ?? 0)}
          onClick={() => setPage(page + 1)}
        >
          {t("reconciliation.next")}
        </Button>
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
    <p className="break-words text-sm text-muted-foreground">
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
