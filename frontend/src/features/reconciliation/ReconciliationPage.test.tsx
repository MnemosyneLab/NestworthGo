import { beforeEach, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import i18n from "@/i18n";
import { AccountReconciliation } from "./AccountReconciliation";
import { formatTimestamp } from "@/lib/time";
import { ReconciliationPage } from "./ReconciliationPage";
import { hasUnresolvedSheetConfirmation } from "@/components/ui/sheet-confirmation";
import type {
  ReconciliationSession,
  ReconciliationItem,
} from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
const q = vi.hoisted(() => ({
  session: undefined as ReconciliationSession | undefined,
  error: false,
  timezone: "UTC",
  preview: vi.fn(),
  commit: vi.fn(),
  adjust: vi.fn(),
  commitAdjustment: vi.fn(),
  refetch: vi.fn(),
  productPreview: vi.fn(),
  commitProduct: vi.fn(),
}));
vi.mock("@/queries/reconciliation", () => ({
  useReconciliationLastSuccess: () => ({
    data: q.session, isError: false, isLoading: false,
  }),
  useReconciliationSessions: () => ({
    data: [],
    isLoading: false,
    isError: false,
    refetch: q.refetch,
  }),
  useReconciliationSession: (id: string) => ({
    data: id ? q.session : undefined,
    isLoading: false,
    isError: q.error,
    refetch: q.refetch,
  }),
  useReconciliationLinks: () => ({ data: [], isError: false }),
  useReconciliationChanges: () => ({ data: [], isError: false }),
  ReconciliationService: {
    Preview: q.preview,
    Commit: q.commit,
    PreviewAdjustment: q.adjust,
    CommitAdjustment: q.commitAdjustment,
    CommitProduct: q.commitProduct,
  },
}));
vi.mock("@/queries/accounts", () => ({
  useAccounts: () => ({
    data: [
      {
        account: { id: "one", name: "Synthetic bank", defaultCurrency: "USD" },
      },
      { account: { id: "two", name: "Other bank", defaultCurrency: "EUR" } },
    ],
    isLoading: false,
    isError: false,
    refetch: q.refetch,
  }),
}));
vi.mock("@/queries/settings", () => ({
  useSettings: () => ({
    data: { weekStart: "monday", dateFormat: "iso", timezone: q.timezone },
  }),
}));
vi.mock("@/features/history/RecordChangeForm", () => ({
  RecordChangeForm: ({ initial }: { initial: unknown }) => (
    <div data-testid="record-form">{JSON.stringify(initial)}</div>
  ),
}));
vi.mock("./ProductCorrection", () => ({
  ProductCorrection: ({
    onPreview,
  }: {
    onPreview: (preview: unknown) => void;
  }) => (
    <button onClick={() => onPreview(q.productPreview())}>
      Synthetic product preview
    </button>
  ),
}));
vi.mock("@/features/liquidity/ProductPreview", () => ({
  ProductPreview: () => null,
}));
vi.mock("@/features/liquidity/ProductFields", () => ({
  ProductTermsFields: () => null,
  ProductPolicyFields: () => null,
}));
vi.mock("@/lib/wails", () => ({ callService: (f: () => unknown) => f() }));
function session(): ReconciliationSession {
  return {
    id: "session-one",
    householdId: "household",
    status: "in_progress",
    revision: 1,
    startedAt: "2026-09-01T12:00:00Z",
    note: "",
    partial: false,
    counts: { pending: 2, confirmed: 0, skipped: 0, unresolved: 0 },
    accounts: [
      {
        accountId: "one",
        name: "Synthetic bank",
        currency: "USD",
        mode: "balance",
        financialRevision: 12,
        priorRevision: 0,
        outcome: "partial",
        recheckRequired: false,
        counts: { pending: 2, confirmed: 0, skipped: 0, unresolved: 0 },
        items: [
          {
            id: "balance",
            accountId: "one",
            accountName: "Synthetic bank",
            kind: "balance",
            label: "Synthetic bank",
            currency: "USD",
            value: "100.001",
            financialRevision: 12,
            recheckRequired: false,
            outcome: "pending",
            note: "",
            source: { reference: "", asOfDate: "2026-09-01" },
          },
          {
            id: "scope",
            accountId: "one",
            accountName: "Synthetic bank",
            kind: "scope",
            label: "Scope",
            financialRevision: 12,
            recheckRequired: false,
            outcome: "pending",
            note: "",
            source: { reference: "", asOfDate: "" },
          },
        ],
      },
    ],
  };
}
function mount(initial?: string) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <ReconciliationPage sessionId={initial} />
    </QueryClientProvider>,
  );
}
beforeEach(async () => {
  await i18n.changeLanguage("en");
  vi.clearAllMocks();
  q.session = session();
  q.error = false;
  q.timezone = "UTC";
  q.preview.mockResolvedValue({ planId: "review-plan", result: q.session });
  q.commit.mockResolvedValue(q.session);
  q.adjust.mockResolvedValue({ planId: "adjust-plan", changes: [] });
  q.commitAdjustment.mockResolvedValue([]);
  q.commitProduct.mockResolvedValue(undefined);
});
it("previews a selected subset and commits before entering the pending session", async () => {
  mount();
  const boxes = screen.getAllByRole("checkbox");
  expect(boxes).toHaveLength(2);
  await userEvent.click(boxes[1]);
  await userEvent.click(
    screen.getByRole("button", { name: "Start reconciliation" }),
  );
  expect(q.preview).toHaveBeenCalledWith(
    expect.objectContaining({ action: "start", accountIds: ["one"] }),
  );
  expect(q.commit).not.toHaveBeenCalled();
  await userEvent.click(
    await screen.findByRole("button", { name: "Commit reviewed result" }),
  );
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "Pause" })).toBeInTheDocument(),
  );
  expect(q.commit).toHaveBeenCalledWith("review-plan");
  expect(screen.getAllByText(/0 confirmed · 2 pending/).length).toBeGreaterThan(
    0,
  );
});
it("uses the actual date/time controls and passes explicit source evidence with current revisions", async () => {
  mount("session-one");
  const trigger = screen.getAllByRole("button", { name: "Review check" })[0];
  await userEvent.click(trigger);
  await userEvent.type(
    screen.getByLabelText("Statement or source reference"),
    "Synthetic September statement",
  );
  await userEvent.click(screen.getByLabelText("Source as-of date"));
  const grid = await screen.findByRole("grid");
  await userEvent.click(
    grid.querySelector('[data-day="2026-09-02"] button') as HTMLElement,
  );
  await userEvent.click(
    screen.getByLabelText("Source time (optional, ledger timezone)"),
  );
  const lists = await screen.findAllByRole("list", { name: "Local time" });
  await userEvent.click(within(lists[0]).getByRole("button", { name: "23" }));
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  expect(q.preview).toHaveBeenCalledWith(
    expect.objectContaining({
      action: "check",
      expectedRevision: 1,
      financialRevision: 12,
      accountId: "one",
      itemId: "balance",
      outcome: "confirmed",
      source: {
        reference: "Synthetic September statement",
        asOfDate: "2026-09-02",
        asOfTime: "23:00",
      },
    }),
  );
  expect(q.commit).not.toHaveBeenCalled();
});
it("links the existing Record form and direct adjustment separately without confirming either", async () => {
  const view = mount("session-one");
  await userEvent.click(
    screen.getAllByRole("button", {
      name: "Record missing income, expense, buy or sell",
    })[0],
  );
  expect(screen.getByTestId("record-form")).toHaveTextContent(
    '"reconciliationLink":{"sessionId":"session-one","accountId":"one","itemId":"balance","expectedRevision":1,"financialRevision":12}',
  );
  await userEvent.keyboard("{Escape}");
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Correct current balance or holding" }),
  );
  await userEvent.clear(screen.getByLabelText("Target balance or quantity"));
  await userEvent.type(
    screen.getByLabelText("Target balance or quantity"),
    "101.12345678",
  );
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  expect(q.adjust).toHaveBeenCalledWith(
    expect.objectContaining({
      balance: "101.12345678",
      link: {
        sessionId: "session-one",
        accountId: "one",
        itemId: "balance",
        expectedRevision: 1,
        financialRevision: 12,
      },
    }),
  );
  expect(q.preview).not.toHaveBeenCalled();
  view.unmount();
});
it("retains an unknown commit through failed reads, guards Escape and retries the identical plan", async () => {
  let refuse: (e: unknown) => void = () => {};
  q.commit.mockImplementationOnce(
    () =>
      new Promise((_resolve, reject) => {
        refuse = reject;
      }),
  );
  const view = mount();
  await userEvent.click(
    screen.getByRole("button", { name: "Start reconciliation" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Commit reviewed result" }),
  );
  expect(hasUnresolvedSheetConfirmation()).toBe(true);
  await userEvent.keyboard("{Escape}");
  expect(screen.getByRole("dialog")).toBeInTheDocument();
  await act(async () => {
    refuse(new Error("synthetic lost response"));
  });
  const retry = await screen.findByRole("button", {
    name: "Retry same submission",
  });
  q.error = true;
  view.rerender(
    <QueryClientProvider client={createTestQueryClient()}>
      <ReconciliationPage />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("dialog")).toBeInTheDocument();
  await userEvent.click(retry);
  await waitFor(() => expect(q.commit).toHaveBeenCalledTimes(2));
  expect(q.commit.mock.calls).toEqual([["review-plan"], ["review-plan"]]);
  await waitFor(() => expect(hasUnresolvedSheetConfirmation()).toBe(false));
});
it("releases a receipt-checked expired preview for dismissal and fresh review", async () => {
  q.commit.mockRejectedValueOnce({
    wireError: { code: "stale_preview", field: "expiresAt" },
  });
  mount();
  await userEvent.click(
    screen.getByRole("button", { name: "Start reconciliation" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Commit reviewed result" }),
  );
  await screen.findByRole("alert");
  expect(hasUnresolvedSheetConfirmation()).toBe(false);
  expect(
    screen.queryByRole("button", { name: "Retry same submission" }),
  ).not.toBeInTheDocument();
  await userEvent.keyboard("{Escape}");
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  q.preview.mockResolvedValueOnce({
    planId: "fresh-review-plan",
    result: q.session,
  });
  await userEvent.click(
    screen.getByRole("button", { name: "Start reconciliation" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Commit reviewed result" }),
  );
  await waitFor(() =>
    expect(q.commit.mock.calls).toEqual([
      ["review-plan"],
      ["fresh-review-plan"],
    ]),
  );
  await waitFor(() => expect(hasUnresolvedSheetConfirmation()).toBe(false));
});
it.each(["planId", "reviewedStateHash", undefined])(
  "keeps an unavailable preview guarded when its outcome is unknown (%s)",
  async (field) => {
    q.commit.mockRejectedValueOnce({
      wireError: { code: "stale_preview", field },
    });
    mount();
    await userEvent.click(
      screen.getByRole("button", { name: "Start reconciliation" }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Commit reviewed result" }),
    );
    await screen.findByRole("button", { name: "Retry same submission" });
    expect(hasUnresolvedSheetConfirmation()).toBe(true);
    await userEvent.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  },
);
it("requires explicit partial intent and preserves the supplied closing explanation in preview", async () => {
  mount("session-one");
  await userEvent.click(screen.getByRole("button", { name: "Finish session" }));
  await userEvent.click(
    screen.getByRole("checkbox", {
      name: "Finish partially with outstanding checks",
    }),
  );
  await userEvent.type(
    screen.getByLabelText("Closing explanation"),
    "Awaiting broker statement",
  );
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  expect(q.preview).toHaveBeenCalledWith(
    expect.objectContaining({
      action: "finish",
      expectedRevision: 1,
      allowPartial: true,
      note: "Awaiting broker statement",
    }),
  );
});
it.each(["en", "zh-CN", "zh-TW"])(
  "renders business labels and completed evidence in %s",
  async (language) => {
    await i18n.changeLanguage(language);
    q.session = {
      ...session(),
      status: "completed",
      completedAt: "2026-09-01T13:00:00Z",
      partial: true,
      note: "Synthetic outstanding checks",
    };
    q.session.accounts![0].items!.push({
      id: "product:frozen",
      accountId: "one",
      accountName: "Synthetic bank",
      kind: "product",
      label: "Frozen synthetic product",
      currency: "EUR",
      value: "1000",
      financialRevision: 12,
      outcome: "pending",
      note: "",
      source: { reference: "", asOfDate: "" },
      recheckRequired: false,
      detail: {
        startOn: "2026-08-01",
        maturityOn: "2026-09-01",
        state: "settled",
        interestMode: "simple_act_365",
        annualRate: "0.025",
        interestPaidThroughOn: "2026-08-20",
        note: "Frozen product terms",
      } as unknown as ReconciliationItem["detail"],
    });
    mount("session-one");
    expect(screen.getByText("Frozen product terms")).toBeInTheDocument();
    expect(screen.getByText(/2.5%/)).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: i18n.t("reconciliation.review") }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText(/Synthetic outstanding checks/),
    ).toBeInTheDocument();
    expect(screen.queryByText(/session-one|household/)).not.toBeInTheDocument();
    expect(screen.queryByText(/reconciliation\./)).not.toBeInTheDocument();
  },
);

it.each(["mutationId", "operationId", ""])(
  "keeps an ambiguous receipt conflict guarded (%s)",
  async (field) => {
    q.commit.mockRejectedValueOnce({
      wireError: {
        code: "conflict",
        field,
        message: "receipt identity conflict",
      },
    });
    mount();
    await userEvent.click(
      screen.getByRole("button", { name: "Start reconciliation" }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Commit reviewed result" }),
    );
    await screen.findByRole("button", { name: "Retry same submission" });
    expect(hasUnresolvedSheetConfirmation()).toBe(true);
    await userEvent.keyboard("{Escape}");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole("button", { name: "Retry same submission" }),
    );
    await waitFor(() => expect(hasUnresolvedSheetConfirmation()).toBe(false));
    expect(q.commit.mock.calls).toEqual([["review-plan"], ["review-plan"]]);
  },
);

function mountProductReview(kind: string) {
  q.session!.accounts![0].items![0] = {
    ...q.session!.accounts![0].items![0],
    kind: "product",
    productId: "synthetic-product",
  };
  // The child form supplies the API preview; exercise the real review and Sheet.
  q.productPreview.mockReturnValue({ planId: `${kind}-plan` });
  return mount("session-one");
}
async function previewProduct() {
  await userEvent.click(
    screen.getByRole("button", { name: i18n.t("reconciliation.productRoute") }),
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Synthetic product preview" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Commit reviewed result" }),
  );
}
it.each(
  ["terms", "valuation", "lifecycle"].flatMap((kind) =>
    ["previewToken", "reviewedStateHash"].map((field) => [kind, field]),
  ),
)(
  "recovers a refused %s correction (%s) with dismissal and fresh review",
  async (kind, field) => {
    q.commitProduct.mockRejectedValueOnce({
      wireError: { code: "stale_preview", field },
    });
    mountProductReview(kind);
    await previewProduct();
    await screen.findByRole("alert");
    expect(hasUnresolvedSheetConfirmation()).toBe(false);
    expect(
      screen.queryByRole("button", { name: "Retry same submission" }),
    ).not.toBeInTheDocument();
    await userEvent.keyboard("{Escape}");
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Commit reviewed result" }),
      ).not.toBeInTheDocument(),
    );
    expect(
      screen.getByRole("button", { name: "Synthetic product preview" }),
    ).toBeInTheDocument();
    // Dismissing the review returns to its still-editable correction form.
    await userEvent.keyboard("{Escape}");
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    q.productPreview.mockReturnValueOnce({ planId: `${kind}-fresh-plan` });
    await previewProduct();
    await waitFor(() =>
      expect(q.commitProduct.mock.calls).toEqual([
        [`${kind}-plan`],
        [`${kind}-fresh-plan`],
      ]),
    );
    await waitFor(() => expect(hasUnresolvedSheetConfirmation()).toBe(false));
  },
);
it.each([
  ["stale_preview", "planId"],
  ["stale_preview", undefined],
  ["conflict", "mutationId"],
  ["conflict", "operationId"],
  ["conflict", undefined],
  ["internal", undefined],
])("preserves unknown product recovery for %s/%s", async (code, field) => {
  q.commitProduct.mockRejectedValueOnce({ wireError: { code, field } });
  mountProductReview("terms");
  await previewProduct();
  const retry = await screen.findByRole("button", {
    name: "Retry same submission",
  });
  expect(hasUnresolvedSheetConfirmation()).toBe(true);
  await userEvent.keyboard("{Escape}");
  expect(screen.getByRole("dialog")).toBeInTheDocument();
  await userEvent.click(retry);
  await waitFor(() => expect(hasUnresolvedSheetConfirmation()).toBe(false));
  expect(q.commitProduct.mock.calls).toEqual([["terms-plan"], ["terms-plan"]]);
});

it.each(["a", "中", "😀"])("blocks over-budget %s fields before preview and allows valid byte bounds", async (text) => {
  mount("session-one");
  await userEvent.click(screen.getAllByRole("button", { name: "Review check" })[0]);
  const preview = screen.getByRole("button", { name: "Preview" });
  for (const [label, limit] of [["Statement or source reference", 512], ["Value shown by your source (optional)", 256], ["Explanation or note", 2000]] as const) {
    const input = screen.getByLabelText(label);
    const value = text.repeat(Math.floor(limit / new TextEncoder().encode(text).length));
    fireEvent.change(input, { target: { value: value + text } });
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(preview).toBeDisabled();
    await userEvent.click(preview);
    expect(q.preview).not.toHaveBeenCalled();
    fireEvent.change(input, { target: { value } });
    expect(input).not.toHaveAttribute("aria-invalid", "true");
    expect(preview).toBeEnabled();
  }
  await userEvent.click(preview);
  expect(q.preview).toHaveBeenCalledTimes(1);
});
it.each(["Asia/Tokyo", "America/Los_Angeles"])("uses configured %s for session and account completion timestamps", async (timezone) => {
  q.timezone = timezone;
  const hour = timezone === "Asia/Tokyo" ? "23" : "00";
  q.session!.startedAt = `2026-09-01T${hour}:30:00Z`;
  q.session!.completedAt = `2026-09-01T${hour}:40:00Z`;
  q.session!.status = "completed";
  const view = mount("session-one");
  expect(formatTimestamp(q.session!.startedAt, q.timezone, "en")).not.toEqual(formatTimestamp(q.session!.startedAt, "UTC", "en"));
  expect(screen.getByText(new RegExp(formatTimestamp(q.session!.startedAt, q.timezone, "en")))).toBeInTheDocument();
  expect(screen.getByText(new RegExp(formatTimestamp(q.session!.completedAt, q.timezone, "en")))).toBeInTheDocument();
  view.unmount();
  render(<QueryClientProvider client={createTestQueryClient()}><AccountReconciliation accountId="one" /></QueryClientProvider>);
  expect(screen.getByText(new RegExp(formatTimestamp(q.session!.completedAt, q.timezone, "en")))).toBeInTheDocument();
});

it.each(["en", "zh-CN", "zh-TW"])("identifies missing source evidence inline and clears corrected errors in %s", async (language) => {
  await i18n.changeLanguage(language);
  q.preview.mockRejectedValueOnce(Object.assign(new Error("Source: Check this field and try again."), { wireError: { code: "validation", field: "source", message: "confirmation requires a user-provided source and as-of date" } }));
  mount("session-one");
  await userEvent.click(screen.getAllByRole("button", { name: i18n.t("reconciliation.review") })[0]);
  await userEvent.click(screen.getByRole("button", { name: i18n.t("reconciliation.preview") }));
  expect(await screen.findByRole("alert")).toHaveTextContent(i18n.t("reconciliation.validation.source"));
  const reference = screen.getByLabelText(i18n.t("reconciliation.source"));
  expect(reference).toHaveAttribute("aria-invalid", "true");
  expect(reference).toHaveAccessibleDescription(/source|来源|來源/);
  await userEvent.type(reference, "Synthetic statement");
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(reference).not.toHaveAttribute("aria-invalid");
});

it("identifies partial finish intent and closing explanation and clears stale errors", async () => {
  q.preview.mockRejectedValueOnce(Object.assign(new Error("Check this field and try again."), { wireError: { code: "validation", field: "allowPartial", message: "partial finish requires explicit intent and a closing explanation" } }));
  mount("session-one");
  await userEvent.click(screen.getByRole("button", { name: "Finish session" }));
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(i18n.t("reconciliation.validation.allowPartial"));
  const partial = screen.getByRole("checkbox", { name: "Finish partially with outstanding checks" });
  expect(partial).toHaveAccessibleDescription(i18n.t("reconciliation.validation.allowPartial"));
  await userEvent.type(screen.getByLabelText("Closing explanation"), "Waiting for statement");
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});

it("shows holding quantities without quote currency in completed checkpoints, attempts and previews", async () => {
  const item = q.session!.accounts![0].items![0];
  Object.assign(item, { kind: "holding", label: "Synthetic shares", value: "100", cost: "10", detail: { quantityUnit: "" }, checkedAt: "2026-09-01T12:00:00Z", source: { reference: "", asOfDate: "" }, outcome: "skipped" });
  item.attempts = [{ ...item, value: "100", checkedAt: "2026-09-01T12:00:00Z", note: "Awaiting statement" }];
  const view = mount("session-one");
  const article = screen.getByText(/Synthetic shares/).closest("article")!;
  expect(within(article).getByText("100", { selector: "p" })).toBeInTheDocument();
  expect(article).not.toHaveTextContent(/100 USD|·\s*·|·\s*Checked at|·\s*$/);
  view.unmount();
  mount();
  await userEvent.click(screen.getByRole("button", { name: "Start reconciliation" }));
  const review = await screen.findByRole("dialog");
  expect(within(review).getByText("100", { selector: "p" })).toBeInTheDocument();
  expect(review).not.toHaveTextContent(/100 USD|·\s*·/);
});


it("preserves the canonical cash amount in the reviewed checkpoint", async () => {
  const item = q.session!.accounts![0].items![0];
  Object.assign(item, { checkedAt: "2026-09-01T12:00:00Z", source: { reference: "Synthetic statement", asOfDate: "2026-09-01" }, outcome: "confirmed" });
  mount("session-one");
  await userEvent.click(screen.getAllByRole("button", { name: "Review check" })[0]);
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  expect(await screen.findByText("100.001 USD", { selector: "p" })).toBeInTheDocument();
});
