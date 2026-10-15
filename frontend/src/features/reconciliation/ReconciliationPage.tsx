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
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
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
import { callService } from "@/lib/wails";
import { displayError } from "@/lib/display";
import { formatAmount, formatQuotePrice, multiplyCanonical } from "@/lib/money";
import { formatTimestamp as timestamp } from "@/lib/time";
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

const formatTimestamp = (value: string) => timestamp(value, undefined);
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
  planId: string;
  result?: ReconciliationSession;
  adjustment?: AdjustmentPreview;
  product?: ProductPreview;
};
export function ReconciliationPage({
  accountId,
  sessionId,
}: {
  accountId?: string;
  sessionId?: string;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
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
  const [busy, setBusy] = useState(false);
  const [editing, setEditing] = useState<{
    account: ReconciliationAccount;
    item: ReconciliationItem;
    route: "check" | "adjust" | "record" | "product" | "import" | "newProduct";
  }>();
  const [closing, setClosing] = useState(false);
  const [closingNote, setClosingNote] = useState("");
  const [partial, setPartial] = useState(false);
  const trigger = useRef<HTMLElement | null>(null);
  const refresh = () => {
    void client.invalidateQueries({ queryKey: ["reconciliation"] });
  };
  const preview = async (action: ReconciliationAction) => {
    setBusy(true);
    setError(undefined);
    try {
      const result = await callService(() =>
        ReconciliationService.Preview(action),
      );
      setReview(result);
    } catch (cause) {
      setError(displayError(cause, t("reconciliation.error")));
    } finally {
      setBusy(false);
    }
  };
  const activeAccounts = (accounts.data ?? []).filter(
    (a) => !a.account.archivedAt,
  );
  const chosen = selected ?? activeAccounts.map((a) => a.account.id);
  const rows = (sessions.data ?? []).filter(
    (s) => !filter || s.status === filter,
  );
  const current = detail.data;
  const select = (id: string) => {
    if (hasUnresolvedSheetConfirmation()) return;
    setActive(id);
    setEditing(undefined);
    setClosing(false);
    setError(undefined);
  };
  return (
    <div className="flex min-w-0 flex-col gap-5">
      <PageChrome pageId="reconciliation" title={t("reconciliation.title")} />
      <p className="text-sm text-muted-foreground">
        {t("reconciliation.localOnly")}
      </p>
      {error && <p role="alert">{error}</p>}
      {!active ? (
        <>
          {accounts.isLoading || sessions.isLoading ? (
            <LoadingState label={t("reconciliation.loading")} />
          ) : accounts.isError || sessions.isError ? (
            <ErrorState
              description={t("ui.state.errorDescription")}
              title={t("reconciliation.error")}
              onRetry={() => {
                void accounts.refetch();
                void sessions.refetch();
              }}
            />
          ) : (
            <>
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
              {!rows.length && <EmptyState title={t("reconciliation.empty")} />}
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
                {current.partial && (
                  <p>
                    {t("reconciliation.partial")}: {current.note}
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
                    <Button disabled={busy} onClick={() => setClosing(true)}>
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
                    {a.recheckRequired && (
                      <p role="status">{t("reconciliation.recheck")}</p>
                    )}
                    {(a.items ?? []).map((item) => (
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
                          <span>{t(`reconciliation.${item.outcome}`)}</span>
                        </div>
                        {item.kind !== "scope" && (
                          <p className="break-words font-mono">
                            {item.value
                              ? formatAmount(
                                  item.value,
                                  item.kind === "holding"
                                    ? undefined
                                    : item.currency,
                                )
                              : t("reconciliation.unavailable")}{" "}
                            {item.currency}
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
                            {item.source.reference} · {item.source.asOfDate}{" "}
                            {item.source.asOfTime} {item.source.timezone} ·{" "}
                            {t("reconciliation.checkedAt")}:{" "}
                            {formatTimestamp(item.checkedAt)}
                          </p>
                        )}
                        {item.referenceValue && (
                          <p>
                            {t("reconciliation.referenceValue")}:{" "}
                            {item.referenceValue}
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
                                setEditing({
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
                            ) && (
                              <Button
                                variant="ghost"
                                onClick={(e) => {
                                  trigger.current = e.currentTarget;
                                  setEditing({
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
                                  setEditing({
                                    account: a,
                                    item,
                                    route: "product",
                                  });
                                }}
                              >
                                {t("reconciliation.productRoute")}
                              </Button>
                            )}
                            {item.kind === "scope" && a.mode === "holdings" && (
                              <Button
                                variant="ghost"
                                onClick={(e) => {
                                  trigger.current = e.currentTarget;
                                  setEditing({
                                    account: a,
                                    item,
                                    route: "newProduct",
                                  });
                                }}
                              >
                                {t("reconciliation.addProduct")}
                              </Button>
                            )}
                            {item.kind === "scope" && a.mode === "holdings" && (
                              <Button
                                variant="ghost"
                                onClick={(e) => {
                                  trigger.current = e.currentTarget;
                                  setEditing({
                                    account: a,
                                    item,
                                    route: "import",
                                  });
                                }}
                              >
                                {t("reconciliation.importPosition")}
                              </Button>
                            )}
                            {item.kind !== "product" && (
                              <Button
                                variant="ghost"
                                onClick={(e) => {
                                  trigger.current = e.currentTarget;
                                  setEditing({
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
                                {formatTimestamp(attempt.checkedAt)} ·{" "}
                                {t(`reconciliation.${attempt.outcome}`)} ·{" "}
                                {attempt.value} · {attempt.source.reference} ·{" "}
                                {attempt.source.asOfDate}{" "}
                                {attempt.source.asOfTime} · {attempt.note}
                              </p>
                            ))}
                          </details>
                        )}
                      </article>
                    ))}
                  </section>
                ))}
                <RecentChanges sessionId={active} />
                <LinkedChanges sessionId={active} />
              </>
            )
          )}
        </>
      )}
      {closing && current && (
        <Sheet open onOpenChange={(open) => !open && setClosing(false)}>
          <SheetContent>
            <SheetHeader>
              <SheetTitle>{t("reconciliation.finish")}</SheetTitle>
            </SheetHeader>
            <div className="flex flex-col gap-4">
              {error && <p role="alert">{error}</p>}
              <label className="flex gap-2">
                <input
                  type="checkbox"
                  checked={partial}
                  onChange={(e) => setPartial(e.target.checked)}
                />
                {t("reconciliation.allowPartial")}
              </label>
              <Label htmlFor="closing-note">
                {t("reconciliation.closingNote")}
              </Label>
              <Input
                id="closing-note"
                value={closingNote}
                maxLength={2000}
                onChange={(e) => setClosingNote(e.target.value)}
              />
              <Button
                disabled={busy}
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
        <Sheet open onOpenChange={(open) => !open && setEditing(undefined)}>
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
            {error && <p role="alert">{error}</p>}
            {editing.route === "check" ? (
              <CheckForm
                item={editing.item}
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
                      : ChangeCommandKind.ChangeMoneyAdded,
                    editing.item.currency || editing.account.currency,
                  ),
                  accountId: editing.account.accountId,
                  holdingId: editing.item.holdingId,
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
                  setEditing(undefined);
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
                onPreview={(p) => setReview({ planId: p.planId, product: p })}
              />
            ) : (
              <AdjustmentForm
                session={current}
                account={editing.account}
                item={editing.item}
                onPreview={(p) =>
                  setReview({ planId: p.planId, adjustment: p })
                }
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
          onOpenChange={(open) => !open && setEditing(undefined)}
          onReconciliationPreview={(p) =>
            setReview({ planId: p.planId, product: p })
          }
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
                setEditing(undefined);
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
}: {
  item: ReconciliationItem;
  onPreview: (fields: Partial<ReconciliationAction>) => Promise<void>;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const [outcome, setOutcome] = useState("confirmed");
  const [reference, setReference] = useState(item.source.reference);
  const [date, setDate] = useState(item.source.asOfDate);
  const [time, setTime] = useState(item.source.asOfTime ?? "");
  const [note, setNote] = useState(item.note);
  const [value, setValue] = useState(item.referenceValue ?? "");
  return (
    <div className="flex flex-col gap-3">
      <Label htmlFor="check-outcome">{t("reconciliation.outcome")}</Label>
      <NativeSelect
        id="check-outcome"
        value={outcome}
        onChange={(e) => setOutcome(e.target.value)}
      >
        {["confirmed", "skipped", "unresolved", "pending"].map((s) => (
          <option key={s} value={s}>
            {t(`reconciliation.${s}`)}
          </option>
        ))}
      </NativeSelect>
      <Label htmlFor="check-source">{t("reconciliation.source")}</Label>
      <Input
        id="check-source"
        maxLength={512}
        value={reference}
        onChange={(e) => setReference(e.target.value)}
      />
      <Label htmlFor="check-date">{t("reconciliation.sourceDate")}</Label>
      <DatePicker id="check-date" value={date} onChange={setDate} clearable />
      <Label htmlFor="check-time">{t("reconciliation.sourceTime")}</Label>
      <TimePicker id="check-time" value={time} onChange={setTime} clearable />
      <Label htmlFor="check-value">{t("reconciliation.referenceValue")}</Label>
      <Input
        id="check-value"
        value={value}
        maxLength={256}
        onChange={(e) => setValue(e.target.value)}
      />
      <Label htmlFor="check-note">{t("reconciliation.note")}</Label>
      <Input
        id="check-note"
        value={note}
        maxLength={2000}
        onChange={(e) => setNote(e.target.value)}
      />
      <Button
        disabled={disabled}
        onClick={() =>
          void onPreview({
            outcome,
            source: { reference, asOfDate: date, asOfTime: time },
            referenceValue: value,
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
  const [value, setValue] = useState(
    item.kind === "scope" ? "" : (item.value ?? ""),
  );
  const [currency, setCurrency] = useState(item.currency ?? account.currency);
  const [unit, setUnit] = useState("");
  const [total, setTotal] = useState("");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  return (
    <div className="flex flex-col gap-3">
      <p>{t("reconciliation.correctionHelp")}</p>
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
            inputMode="decimal"
            value={unit}
            onChange={(e) => setUnit(e.target.value)}
          />
          <Label htmlFor="adjust-total">{t("reconciliation.totalCost")}</Label>
          <Input
            id="adjust-total"
            inputMode="decimal"
            value={total}
            onChange={(e) => setTotal(e.target.value)}
          />
        </>
      )}
      <Label htmlFor="adjust-note">{t("reconciliation.note")}</Label>
      <Input
        id="adjust-note"
        value={note}
        maxLength={2000}
        onChange={(e) => setNote(e.target.value)}
      />
      {error && <p role="alert">{error}</p>}
      <Button
        disabled={busy}
        onClick={() => {
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
            .then(onPreview)
            .catch((e) => setError(displayError(e, t("reconciliation.error"))))
            .finally(() => setBusy(false));
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
  const confirmation = useSheetConfirmation();
  const [busy, setBusy] = useState(false);
  const [unknown, setUnknown] = useState(false);
  const dispatching = useRef(false);
  const [error, setError] = useState("");
  const commit = async () => {
    if (dispatching.current) return;
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
      const wire = (cause as { wireError?: { code?: string; field?: string } })
        ?.wireError;
      const refused =
        (wire?.code === "conflict" && wire.field !== "operationId") ||
        (wire?.code === "stale_preview" && wire.field !== "planId") ||
        wire?.code === "validation";
      if (refused) {
        confirmation?.resolve(review.planId);
        setUnknown(false);
      } else {
        setUnknown(true);
      }
      setError(displayError(cause, t("reconciliation.error")));
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
                    {item.value} {item.currency} · {item.referenceValue}
                  </p>
                  <p>
                    {item.source.reference} · {item.source.asOfDate}{" "}
                    {item.source.asOfTime} {item.source.timezone}
                  </p>
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
                effect.direction === "added"
                  ? "history.detailAmountAdded"
                  : "history.detailAmountRemoved",
              )}
              : {effect.money?.amount ?? effect.quantity}{" "}
              {effect.money?.currency}{" "}
              {effect.costUnitPrice &&
                `${t("reconciliation.unitCost")}: ${effect.costUnitPrice}`}
            </p>
          ))}
          {change.resulting?.map((v, j) => (
            <p key={j}>
              {v.name} · {v.amount || v.quantity} {v.currency}
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
      {error && <p role="alert">{error}</p>}
      <Button disabled={busy} onClick={() => void commit()}>
        {t(unknown ? "reconciliation.retrySame" : "reconciliation.commit")}
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
  const links = useReconciliationLinks(sessionId);
  const [page, setPage] = useState(0);
  return (
    <details>
      <summary>{t("reconciliation.linkedChanges")}</summary>
      {links.isError ? (
        <p role="alert">{t("reconciliation.error")}</p>
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
                {t("reconciliation.reviewedBefore")}: {before?.value}{" "}
                {before?.currency}{" "}
                {before?.cost &&
                  `${t("reconciliation.unitCost")}: ${before.cost}`}
              </p>
              {before?.source?.reference && (
                <p>
                  {before.source.reference} · {before.source.asOfDate}{" "}
                  {before.source.asOfTime}
                </p>
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
                  {value.Currency}
                </p>
              ))}
              {after?.effects?.map((effect, j) => (
                <p key={j}>
                  {t(
                    effect.direction === "added"
                      ? "history.detailAmountAdded"
                      : "history.detailAmountRemoved",
                  )}
                  : {effect.amount ?? effect.quantity} {effect.currency}{" "}
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

function RecentChanges({ sessionId }: { sessionId: string }) {
  const { t } = useTranslation();
  const changes = useReconciliationChanges(sessionId);
  const [page, setPage] = useState(0);
  return (
    <details>
      <summary>{t("reconciliation.recentChanges")}</summary>
      {changes.isError ? (
        <p role="alert">{t("reconciliation.error")}</p>
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
              {evidence.accountName} · {evidence.productName} ·{" "}
              {evidence.amount ?? evidence.quantity} {evidence.currency} ·{" "}
              {t("reconciliation.recordedAt")}:{" "}
              {formatTimestamp(change.recordedAt)} ·{" "}
              {t("reconciliation.effectiveAt")}:{" "}
              {formatTimestamp(change.effectiveAt)}
            </p>
          );
        })
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
