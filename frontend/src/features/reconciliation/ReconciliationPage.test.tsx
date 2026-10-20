import { beforeEach, expect, it, vi } from "vitest";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import i18n from "@/i18n";
import { AccountReconciliation } from "./AccountReconciliation";
import { localDateTimeInTimeZone } from "@/lib/time";
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
  sessions: [] as ReconciliationSession[],
  auditError: false,
  archivedAccountId: "",
  preview: vi.fn(),
  commit: vi.fn(),
  adjust: vi.fn(),
  commitAdjustment: vi.fn(),
  refetch: vi.fn(),
  productPreview: vi.fn(),
  newProductPreview: vi.fn(),
  commitProduct: vi.fn(),
}));
vi.mock("@/queries/reconciliation", () => ({
  useReconciliationLastSuccess: () => ({
    data: q.session,
    isError: false,
    isLoading: false,
  }),
  useReconciliationSessions: () => ({
    data: q.sessions,
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
  useReconciliationLinks: () => ({
    data: [],
    isError: q.auditError,
    refetch: q.refetch,
  }),
  useReconciliationChanges: () => ({
    data: [],
    isError: q.auditError,
    refetch: q.refetch,
  }),
  ReconciliationService: {
    Preview: q.preview,
    Commit: q.commit,
    PreviewAdjustment: q.adjust,
    CommitAdjustment: q.commitAdjustment,
    PreviewProductOperation: q.newProductPreview,
    CommitProduct: q.commitProduct,
  },
}));
vi.mock("@/queries/accounts", () => ({
  useAccounts: () => ({
    data: [
      {
        account: { id: "one", name: "Synthetic bank", defaultCurrency: "USD", archivedAt: q.archivedAccountId === "one" ? "2026-09-02T00:00:00Z" : undefined },
      },
      { account: { id: "two", name: "Other bank", defaultCurrency: "EUR" } },
    ],
    isLoading: false,
    isError: false,
    refetch: q.refetch,
  }),
  useAccountValuations: () => ({ data: [{ account: { id: "one", name: "Synthetic bank" }, components: [{ nativeCurrency: "USD", nativeAmount: "5000" }] }] }),
}));
vi.mock("@/queries/history", () => ({
  useHistoryOrigin: () => ({ data: { timezone: q.timezone } }),
}));
vi.mock("@/queries/settings", () => ({
  useSettings: () => ({
    data: { weekStart: "monday", dateFormat: "iso", timezone: q.timezone },
  }),
  useSupportedCurrencies: () => ({ data: ["USD", "EUR"] }),
}));
vi.mock("@/queries/liquidity", () => ({
  usePreviewProductOperation: () => ({ isPending: false, mutateAsync: vi.fn() }),
  useRecordProductOperation: () => ({ isPending: false, mutateAsync: vi.fn() }),
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
    <button onClick={() => {
      const result = q.productPreview();
      if (result instanceof Promise) void result.then(onPreview);
      else onPreview(result);
    }}>
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
    closingNote: "",
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
  q.sessions = [];
  q.auditError = false;
  q.archivedAccountId = "";
  q.preview.mockResolvedValue({ planId: "review-plan", result: q.session });
  q.commit.mockResolvedValue(q.session);
  q.adjust.mockResolvedValue({ planId: "adjust-plan", changes: [] });
  q.commitAdjustment.mockResolvedValue([]);
  q.commitProduct.mockResolvedValue(undefined);
  q.newProductPreview.mockResolvedValue({ planId: "new-product-plan" });
});
it("keeps archived reconciliation records readable without offering an invalid start", () => {
  q.archivedAccountId = "one";
  q.session!.status = "completed";
  q.session!.completedAt = "2026-09-01T13:00:00Z";
  render(<QueryClientProvider client={createTestQueryClient()}><AccountReconciliation accountId="one" archived /></QueryClientProvider>);
  expect(screen.queryByRole("button", { name: i18n.t("reconciliation.open") })).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: i18n.t("reconciliation.lastRecord") })).toBeInTheDocument();
  mountWithArchivedAccount();
  expect(screen.getByRole("button", { name: "Start reconciliation" })).toBeDisabled();
});

function mountWithArchivedAccount() {
  return render(<QueryClientProvider client={createTestQueryClient()}><ReconciliationPage accountId="one" /></QueryClientProvider>);
}
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
    await screen.findByRole("button", {
      name: /Start reconciliation|Save reviewed result|Pause|Resume|Finish session/,
    }),
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
  const trigger = within(
    screen.getByText(/Synthetic bank · Account balance/).closest("article")!,
  ).getByRole("button", { name: "Check item" });
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
    within(
      screen.getByText(/Synthetic bank · Account balance/).closest("article")!,
    ).getByRole("button", {
      name: "Record missing income, expense, buy or sell",
    }),
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
    await screen.findByRole("button", {
      name: /Start reconciliation|Save reviewed result|Pause|Resume|Finish session/,
    }),
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
    await screen.findByRole("button", {
      name: /Start reconciliation|Save reviewed result|Pause|Resume|Finish session/,
    }),
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
    await screen.findByRole("button", {
      name: /Start reconciliation|Save reviewed result|Pause|Resume|Finish session/,
    }),
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
      await screen.findByRole("button", {
        name: /Start reconciliation|Save reviewed result|Pause|Resume|Finish session/,
      }),
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
it("starts each session's finishing form without another session's draft", async () => {
  const second = { ...session(), id: "session-two", accountNames: ["Other bank"] };
  q.sessions = [second];
  mount("session-one");
  await userEvent.click(screen.getByRole("button", { name: "Finish session" }));
  await userEvent.click(screen.getByRole("checkbox", { name: "Finish partially with outstanding checks" }));
  await userEvent.type(screen.getByLabelText("Closing explanation"), "Waiting for first statement");
  await userEvent.keyboard("{Escape}");
  await userEvent.click(screen.getByRole("button", { name: i18n.t("reconciliation.back") }));
  q.session = second;
  await userEvent.click(screen.getByRole("button", { name: /Other bank.*In progress/i }));
  await userEvent.click(screen.getByRole("button", { name: "Finish session" }));
  expect(screen.getByRole("checkbox", { name: "Finish partially with outstanding checks" })).not.toBeChecked();
  expect(screen.getByLabelText("Closing explanation")).toHaveValue("");
});
it("does not reopen a closed sheet when its preview response arrives late", async () => {
  let resolvePreview!: (value: unknown) => void;
  q.preview.mockImplementationOnce(() => new Promise((resolve) => { resolvePreview = resolve; }));
  mount("session-one");
  await userEvent.click(screen.getByRole("button", { name: "Finish session" }));
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  expect(q.preview).toHaveBeenCalledOnce();
  await userEvent.keyboard("{Escape}");
  await act(async () => { resolvePreview({ planId: "late-plan", result: q.session }); });
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
it("discards a late adjustment preview after closing and switching sessions", async () => {
  let resolveAdjustment!: (value: unknown) => void;
  q.adjust.mockImplementationOnce(() => new Promise((resolve) => { resolveAdjustment = resolve; }));
  const second = { ...session(), id: "session-two", accountNames: ["Other bank"] };
  q.sessions = [second];
  mount("session-one");
  await userEvent.click(screen.getByRole("button", { name: "Correct current balance or holding" }));
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  expect(q.adjust).toHaveBeenCalledOnce();
  await userEvent.keyboard("{Escape}");
  await userEvent.click(screen.getByRole("button", { name: i18n.t("reconciliation.back") }));
  q.session = second;
  await userEvent.click(screen.getByRole("button", { name: /Other bank.*In progress/i }));
  await act(async () => { resolveAdjustment({ planId: "late-adjustment", changes: [] }); });
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
it("discards a late product correction preview after closing and switching sessions", async () => {
  mountProductReview("late");
  let resolveProduct!: (value: unknown) => void;
  q.productPreview.mockImplementationOnce(() => new Promise((resolve) => { resolveProduct = resolve; }));
  const second = { ...session(), id: "session-two", accountNames: ["Other bank"] };
  q.sessions = [second];
  await userEvent.click(screen.getByRole("button", { name: i18n.t("reconciliation.productRoute") }));
  await userEvent.click(screen.getByRole("button", { name: "Synthetic product preview" }));
  await userEvent.keyboard("{Escape}");
  await userEvent.click(screen.getByRole("button", { name: i18n.t("reconciliation.back") }));
  q.session = second;
  await userEvent.click(screen.getByRole("button", { name: /Other bank.*In progress/i }));
  await act(async () => { resolveProduct({ planId: "late-product" }); });
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
it("keeps a late new-product preview closed after Escape and a session switch", async () => {
  q.session!.accounts![0].mode = "holdings";
  const second = { ...session(), id: "session-two", accountNames: ["Other bank"] };
  q.sessions = [second];
  let resolveProduct!: (value: unknown) => void;
  q.newProductPreview.mockImplementationOnce(() => new Promise((resolve) => { resolveProduct = resolve; }));
  mount("session-one");
  await userEvent.click(screen.getByRole("button", { name: i18n.t("reconciliation.addProduct") }));
  fireEvent.change(screen.getByLabelText("Principal"), { target: { value: "1000" } });
  await userEvent.click(screen.getByLabelText("Start date"));
  await userEvent.click(within(screen.getByRole("grid")).getAllByRole("button").find((button) => !button.hasAttribute("disabled"))!);
  await userEvent.click(screen.getByLabelText("Maturity date"));
  await userEvent.click(within(screen.getByRole("grid")).getAllByRole("button").find((button) => !button.hasAttribute("disabled"))!);
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(q.newProductPreview).toHaveBeenCalledOnce();
  await userEvent.keyboard("{Escape}");
  await userEvent.click(screen.getByRole("button", { name: i18n.t("reconciliation.back") }));
  q.session = second;
  await userEvent.click(screen.getByRole("button", { name: /Other bank.*In progress/i }));
  await act(async () => { resolveProduct({ planId: "late-new-product" }); });
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
it("clears an open preview when selecting the session list", async () => {
  mountProductReview("selected-away");
  await userEvent.click(screen.getByRole("button", { name: i18n.t("reconciliation.productRoute") }));
  await userEvent.click(screen.getByRole("button", { name: "Synthetic product preview" }));
  expect(await screen.findByRole("dialog")).toBeInTheDocument();
  fireEvent.click(screen.getByText(i18n.t("reconciliation.back")));
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});
it.each(["en"])(
  "renders business labels and completed evidence in %s",
  async (language) => {
    await i18n.changeLanguage(language);
    q.session = {
      ...session(),
      status: "completed",
      completedAt: "2026-09-01T13:00:00Z",
      partial: true,
      closingNote: "Synthetic outstanding checks",
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
      await screen.findByRole("button", {
        name: /Start reconciliation|Save reviewed result|Pause|Resume|Finish session/,
      }),
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
    await screen.findByRole("button", {
      name: /Start reconciliation|Save reviewed result|Pause|Resume|Finish session/,
    }),
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
        screen.queryByRole("button", { name: "Save reviewed result" }),
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

it.each(["a", "中", "😀"])(
  "blocks over-budget %s fields before preview and allows valid byte bounds",
  async (text) => {
    mount("session-one");
    await userEvent.click(
      within(
        screen
          .getByText(/Synthetic bank · Account balance/)
          .closest("article")!,
      ).getByRole("button", { name: "Check item" }),
    );
    const preview = screen.getByRole("button", { name: "Preview" });
    for (const [label, limit] of [
      ["Statement or source reference", 512],
      ["Value shown by your source (optional)", 256],
      ["Explanation or note", 2000],
    ] as const) {
      const input = screen.getByLabelText(label);
      const value = text.repeat(
        Math.floor(limit / new TextEncoder().encode(text).length),
      );
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
  },
);
it.each(["Asia/Tokyo", "America/Los_Angeles"])(
  "uses configured %s for session and account completion timestamps",
  async (timezone) => {
    q.timezone = timezone;
    const hour = timezone === "Asia/Tokyo" ? "23" : "00";
    q.session!.startedAt = `2026-09-01T${hour}:30:00Z`;
    q.session!.completedAt = `2026-09-01T${hour}:40:00Z`;
    q.session!.status = "completed";
    const view = mount("session-one");
    expect(formatTimestamp(q.session!.startedAt, q.timezone, "en")).not.toEqual(
      formatTimestamp(q.session!.startedAt, "UTC", "en"),
    );
    expect(
      screen.getByText(
        new RegExp(formatTimestamp(q.session!.startedAt, q.timezone, "en")),
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        new RegExp(formatTimestamp(q.session!.completedAt, q.timezone, "en")),
      ),
    ).toBeInTheDocument();
    view.unmount();
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <AccountReconciliation accountId="one" />
      </QueryClientProvider>,
    );
    expect(
      screen.getByText(
        new RegExp(formatTimestamp(q.session!.completedAt, q.timezone, "en")),
      ),
    ).toBeInTheDocument();
  },
);

it.each(["en"])(
  "identifies missing source evidence inline and clears corrected errors in %s",
  async (language) => {
    await i18n.changeLanguage(language);
    q.preview.mockRejectedValueOnce(
      Object.assign(new Error("Source: Check this field and try again."), {
        wireError: {
          code: "validation",
          field: "source",
          message:
            "confirmation requires a user-provided source and as-of date",
        },
      }),
    );
    mount("session-one");
    await userEvent.click(
      screen.getAllByRole("button", {
        name: i18n.t("reconciliation.review"),
      })[0],
    );
    await userEvent.click(
      screen.getByRole("button", { name: i18n.t("reconciliation.preview") }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      i18n.t("reconciliation.validation.source"),
    );
    const reference = screen.getByLabelText(i18n.t("reconciliation.source"));
    expect(reference).toHaveAttribute("aria-invalid", "true");
    expect(reference).toHaveAccessibleDescription(/source|来源|來源/);
    await userEvent.type(reference, "Synthetic statement");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(reference).not.toHaveAttribute("aria-invalid");
  },
);

it("identifies partial finish intent and closing explanation and clears stale errors", async () => {
  q.preview.mockRejectedValueOnce(
    Object.assign(new Error("Check this field and try again."), {
      wireError: {
        code: "validation",
        field: "allowPartial",
        message:
          "partial finish requires explicit intent and a closing explanation",
      },
    }),
  );
  mount("session-one");
  await userEvent.click(screen.getByRole("button", { name: "Finish session" }));
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    i18n.t("reconciliation.validation.allowPartial"),
  );
  const partial = screen.getByRole("checkbox", {
    name: "Finish partially with outstanding checks",
  });
  expect(partial).toHaveAccessibleDescription(
    i18n.t("reconciliation.validation.allowPartial"),
  );
  await userEvent.type(
    screen.getByLabelText("Closing explanation"),
    "Waiting for statement",
  );
  expect(screen.getByRole("alert")).toBeInTheDocument();
  await userEvent.click(partial);
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});

it("shows holding quantities without quote currency in completed checkpoints, attempts and previews", async () => {
  const item = q.session!.accounts![0].items![0];
  Object.assign(item, {
    kind: "holding",
    label: "Synthetic shares",
    value: "100",
    cost: "10",
    detail: { quantityUnit: "" },
    checkedAt: "2026-09-01T12:00:00Z",
    source: { reference: "", asOfDate: "" },
    outcome: "skipped",
  });
  item.attempts = [
    {
      ...item,
      value: "100",
      checkedAt: "2026-09-01T12:00:00Z",
      note: "Awaiting statement",
    },
  ];
  const view = mount("session-one");
  const article = screen.getByText(/Synthetic shares/).closest("article")!;
  expect(
    within(article).getByText("100", { selector: "p" }),
  ).toBeInTheDocument();
  expect(article).not.toHaveTextContent(/100 USD|·\s*·|·\s*Checked at|·\s*$/);
  view.unmount();
  mount();
  await userEvent.click(
    screen.getByRole("button", { name: "Start reconciliation" }),
  );
  const review = await screen.findByRole("dialog");
  expect(
    within(review).getByText("100", { selector: "p" }),
  ).toBeInTheDocument();
  expect(review).not.toHaveTextContent(/100 USD|·\s*·/);
});

it("preserves the canonical cash amount in the reviewed checkpoint", async () => {
  const item = q.session!.accounts![0].items![0];
  Object.assign(item, {
    checkedAt: "2026-09-01T12:00:00Z",
    source: { reference: "Synthetic statement", asOfDate: "2026-09-01" },
    outcome: "confirmed",
  });
  mount("session-one");
  await userEvent.click(
    within(
      screen.getByText(/Synthetic bank · Account balance/).closest("article")!,
    ).getByRole("button", { name: "Check item" }),
  );
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  expect(
    await screen.findByText("100.001 USD", { selector: "p" }),
  ).toBeInTheDocument();
});

it("puts unfinished Continue and omission entry first without confirming anything", async () => {
  q.sessions = [
    { ...session(), accountIds: ["one"], accountNames: ["Synthetic bank"] },
  ];
  mount();
  expect(screen.getByText(/An unfinished session/)).toBeInTheDocument();
  await userEvent.click(
    screen.getByRole("button", { name: "Continue session" }),
  );
  expect(q.preview).not.toHaveBeenCalled();
  const articles = screen.getAllByRole("article");
  expect(articles[0]).toHaveTextContent("Anything missing?");
  await userEvent.click(
    within(articles[0]).getByRole("button", {
      name: "Record missing income, expense, buy or sell",
    }),
  );
  expect(screen.getByTestId("record-form")).toHaveTextContent(
    '"itemId":"scope"',
  );
  expect(q.preview).not.toHaveBeenCalled();
});

it("defaults a missing holding transaction to trade with the selected holding", async () => {
  const item = q.session!.accounts![0].items![0];
  Object.assign(item, {
    kind: "holding",
    holdingId: "held-one",
    label: "Synthetic shares",
  });
  mount("session-one");
  const article = screen.getByText(/Synthetic shares/).closest("article")!;
  await userEvent.click(
    within(article).getByRole("button", {
      name: "Record missing income, expense, buy or sell",
    }),
  );
  expect(screen.getByTestId("record-form")).toHaveTextContent('"kind":"trade"');
  expect(screen.getByTestId("record-form")).toHaveTextContent(
    '"holdingId":"held-one"',
  );
  expect(screen.getByTestId("record-form")).toHaveTextContent('"side":"buy"');
});

it("shows comparable numeric differences without changing the selected outcome", async () => {
  q.session!.accounts![0].items![0].source.asOfDate =
    localDateTimeInTimeZone("UTC")!.date;
  mount("session-one");
  await userEvent.click(
    within(
      screen.getByText(/Synthetic bank · Account balance/).closest("article")!,
    ).getByRole("button", { name: "Check item" }),
  );
  await userEvent.type(
    screen.getByLabelText("Value shown by your source (optional)"),
    "99",
  );
  expect(
    screen.queryByText(/Source and ledger values differ/),
  ).not.toBeInTheDocument();
  await userEvent.click(
    screen.getByRole("checkbox", {
      name: "Compare: same units, currency and current as-of basis",
    }),
  );
  expect(
    screen.getByText(/Source and ledger values differ/),
  ).toBeInTheDocument();
  expect(screen.getByLabelText("Check outcome")).toHaveValue("confirmed");
  expect(q.preview).not.toHaveBeenCalled();
});

it("refreshes proven revision conflicts and requires a fresh review", async () => {
  q.commit.mockRejectedValueOnce({
    wireError: {
      code: "conflict",
      field: "financialRevision",
      message: "evidence changed",
    },
  });
  const client = createTestQueryClient();
  const invalidate = vi.spyOn(client, "invalidateQueries");
  render(
    <QueryClientProvider client={client}>
      <ReconciliationPage />
    </QueryClientProvider>,
  );
  await userEvent.click(
    screen.getByRole("button", { name: "Start reconciliation" }),
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Start reconciliation" }),
  );
  expect(await screen.findByRole("alert")).toHaveTextContent(
    /Close this review/,
  );
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["reconciliation"] });
  expect(hasUnresolvedSheetConfirmation()).toBe(false);
  expect(
    screen.getByRole("button", { name: "Start reconciliation" }),
  ).toHaveAttribute("aria-disabled", "true");
  expect(q.commit).toHaveBeenCalledTimes(1);
  await userEvent.keyboard("{Escape}");
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
});

it("keeps removed evidence and hides correction actions, and names audit Retry buttons", async () => {
  const item = q.session!.accounts![0].items![0];
  item.kind = "removed";
  item.note = "Historical evidence retained";
  q.auditError = true;
  mount("session-one");
  const article = screen
    .getByText(/Synthetic bank · Removed from current scope/)
    .closest("article")!;
  expect(article).toHaveTextContent("Historical evidence retained");
  expect(
    within(article).queryByRole("button", {
      name: "Record missing income, expense, buy or sell",
    }),
  ).not.toBeInTheDocument();
  expect(
    within(article).getByRole("button", { name: "Check item" }),
  ).toBeInTheDocument();
  await userEvent.click(
    screen.getByText("Linked corrections", { selector: "summary" }),
  );
  await userEvent.click(
    screen.getByText("Changes since prior successful check", {
      selector: "summary",
    }),
  );
  for (const button of screen.getAllByRole("button", {
    name: i18n.t("reconciliation.retry"),
  })) {
    await userEvent.click(button);
  }
  expect(q.refetch).toHaveBeenCalledTimes(2);
});

it("shows historical last success with neutral later-bookkeeping text", () => {
  q.session!.status = "completed";
  q.session!.completedAt = "2026-09-01T13:00:00Z";
  q.session!.accounts![0].recheckRequired = true;
  render(
    <QueryClientProvider client={createTestQueryClient()}>
      <AccountReconciliation accountId="one" />
    </QueryClientProvider>,
  );
  expect(
    screen.getByText(/Bookkeeping has changed since this successful check/),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "View last successful record" }),
  ).toBeInTheDocument();
});

it("names added-unit and whole-holding costs precisely and retains an unfixed choice error", async () => {
  const item = q.session!.accounts![0].items![0];
  Object.assign(item, {
    kind: "holding",
    holdingId: "held-one",
    label: "Synthetic shares",
    value: "2",
    cost: "10",
  });
  q.adjust.mockRejectedValueOnce({
    wireError: {
      code: "cost_basis_required",
      field: "targets[0].unitCost",
      message: "an explicit per-unit cost is required for added quantity",
    },
  });
  mount("session-one");
  await userEvent.click(
    screen.getByRole("button", { name: "Correct current balance or holding" }),
  );
  const target = screen.getByLabelText("Target balance or quantity");
  fireEvent.change(target, { target: { value: "3" } });
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  const unit = screen.getByLabelText("Added units: cost per unit");
  expect(await screen.findByRole("alert")).toHaveTextContent(
    /enter exactly one/,
  );
  expect(unit).toHaveAttribute("aria-invalid", "true");
  fireEvent.change(unit, { target: { value: "" } });
  expect(screen.getByRole("alert")).toBeInTheDocument();
  fireEvent.change(
    screen.getByLabelText("Entire remaining holding: total cost"),
    { target: { value: "30" } },
  );
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  expect(unit).not.toHaveAttribute("aria-invalid");
});

for (const locale of ["en"]) {
  it.each([
    [
      "unitCost",
      "validation",
      "must be a canonical non-negative decimal with up to eight fractional digits",
      "costUnitInvalid",
      "3",
      "oops",
      "",
    ],
    [
      "totalCost",
      "validation",
      "must be a canonical non-negative decimal with up to four fractional digits",
      "costTotalInvalid",
      "3",
      "",
      "bad",
    ],
    [
      "unitCost",
      "validation",
      "unit cost cannot be changed by reducing quantity",
      "costUnitIncreaseOnly",
      "1",
      "10",
      "",
    ],
    [
      "unitCost",
      "invalid_change",
      "cost-only reconciliation requires totalCost",
      "costOnlyTotal",
      "2",
      "10",
      "",
    ],
    [
      "totalCost",
      "validation",
      "total cost currency must match the Holding's Instrument currency",
      "costCurrency",
      "3",
      "",
      "30",
    ],
    [
      "totalCost",
      "validation",
      "a zero-quantity Holding must have zero total cost",
      "costZeroQuantity",
      "0",
      "",
      "10",
    ],
    [
      "unitCost",
      "validation",
      "unitCost and totalCost cannot be supplied together",
      "costChoice",
      "3",
      "10",
      "30",
    ],
    [
      "unitCost",
      "cost_basis_required",
      "an explicit per-unit cost is required for added quantity",
      "costChoice",
      "3",
      "",
      "",
    ],
    [
      "totalCost",
      "validation",
      "total cost cannot be represented at the supported cost precision",
      "costPrecision",
      "3",
      "",
      "10",
    ],
  ])(
    `shows accurate ${locale} adjustment guidance for %s: %s / %s`,
    async (field, code, message, key, quantity, unitCost, totalCost) => {
      await i18n.changeLanguage(locale);
      Object.assign(q.session!.accounts![0].items![0], {
        kind: "holding",
        holdingId: "held-one",
        value: "2",
        cost: "10",
      });
      q.adjust.mockRejectedValueOnce({
        wireError: { code, field: `targets[0].${field}`, message },
      });
      mount("session-one");
      await userEvent.click(
        screen.getByRole("button", { name: i18n.t("reconciliation.adjust") }),
      );
      fireEvent.change(screen.getByLabelText(i18n.t("reconciliation.target")), {
        target: { value: quantity },
      });
      const unit = screen.getByLabelText(
        i18n.t("reconciliation.addedUnitCost"),
      );
      const total = screen.getByLabelText(i18n.t("reconciliation.totalCost"));
      fireEvent.change(unit, { target: { value: unitCost } });
      fireEvent.change(total, { target: { value: totalCost } });
      await userEvent.click(
        screen.getByRole("button", { name: i18n.t("reconciliation.preview") }),
      );
      expect(await screen.findByRole("alert")).toHaveTextContent(
        i18n.t(`reconciliation.${key}`),
      );
      expect(field === "unitCost" ? unit : total).toHaveAttribute(
        "aria-invalid",
        "true",
      );
      if (key !== "costChoice") {
        fireEvent.change(total, { target: { value: "11" } });
        expect(screen.getByRole("alert")).toHaveTextContent(
          i18n.t(`reconciliation.${key}`),
        );
      }
    },
  );
}

it("preserves an unrecognized localized cost error instead of inventing a choice error", async () => {
  Object.assign(q.session!.accounts![0].items![0], {
    kind: "holding",
    holdingId: "held-one",
    value: "2",
  });
  q.adjust.mockRejectedValueOnce(
    Object.assign(new Error("Specific localized validation guidance"), {
      wireError: {
        code: "validation",
        field: "targets[0].totalCost",
        message: "new backend reason",
      },
    }),
  );
  mount("session-one");
  await userEvent.click(
    screen.getByRole("button", { name: "Correct current balance or holding" }),
  );
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Specific localized validation guidance",
  );
});

for (const locale of ["en"]) {
  it(`shows independent current coverage on a frozen record and account (${locale})`, async () => {
    await i18n.changeLanguage(locale);
    q.session!.status = "completed";
    q.session!.completedAt = "2026-09-01T13:00:00Z";
    const account = q.session!.accounts![0];
    account.inventoryCoverageGap = true;
    account.financialRevisionChanged = false;
    account.uncoveredArchivedHoldings = [
      {
        holdingId: "legacy-held",
        instrumentId: "legacy-asset",
        label: "Synthetic omitted ETF",
        quantity: "2",
        currency: "USD",
      },
    ];
    const frozen = JSON.stringify(q.session);
    const page = mount("session-one");
    expect(
      screen.getByText(i18n.t("reconciliation.historicalCoverageGap")),
    ).toBeInTheDocument();
    expect(
      screen.getByText(i18n.t("reconciliation.coverageNewCheck")),
    ).toBeInTheDocument();
    expect(screen.getByText(/Synthetic omitted ETF · 2/)).toBeInTheDocument();
    expect(
      screen.queryByText(i18n.t("reconciliation.recheck")),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: i18n.t("reconciliation.adjust") }),
    ).not.toBeInTheDocument();
    expect(JSON.stringify(q.session)).toBe(frozen);
    page.unmount();
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <AccountReconciliation accountId="one" />
      </QueryClientProvider>,
    );
    expect(
      screen.getByText(i18n.t("reconciliation.historicalCoverageGap")),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: i18n.t("reconciliation.lastRecord") }),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(i18n.t("reconciliation.laterBookkeeping")),
    ).not.toBeInTheDocument();
    expect(JSON.stringify(q.session)).toBe(frozen);
  });

  it.each(["inventory", "financial", "pending"])(
    `distinguishes %s working checks from completed bookkeeping (${locale})`,
    async (reason) => {
      await i18n.changeLanguage(locale);
      const account = q.session!.accounts![0];
      account.recheckRequired = true;
      account.inventoryCoverageGap = reason === "inventory";
      account.financialRevisionChanged = reason === "financial";
      mount("session-one");
      expect(
        screen.getByText(
          i18n.t(
            `reconciliation.${reason === "financial" ? "workingBookkeepingChanged" : "pendingReview"}`,
          ),
        ),
      ).toBeInTheDocument();
      expect(
        Boolean(
          screen.queryByText(i18n.t("reconciliation.inventoryCoverageGap")),
        ),
      ).toBe(reason === "inventory");
      expect(
        screen.queryByText(i18n.t("reconciliation.recheck")),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByText(i18n.t("reconciliation.laterBookkeeping")),
      ).not.toBeInTheDocument();
      expect(
        screen.getAllByText(
          i18n.t("reconciliation.counts", {
            confirmed: 0,
            pending: 2,
            skipped: 0,
            unresolved: 0,
          }),
        ).length,
      ).toBeGreaterThan(0);
      expect(
        screen.getAllByRole("button", {
          name: i18n.t("reconciliation.review"),
        }),
      ).toHaveLength(2);
    },
  );

  it.each(["0", "2"])(
    `keeps archived evidence and partial finish while hiding unsupported corrections for quantity %s (${locale})`,
    async (quantity) => {
      await i18n.changeLanguage(locale);
      q.session!.accounts![0].archivedHoldingIds = ["held-one"];
      Object.assign(q.session!.accounts![0].items![0], {
        kind: "holding",
        holdingId: "held-one",
        instrumentId: "asset-one",
        archivedNonzero: quantity !== "0",
        label: "Synthetic archived ETF",
        value: quantity,
        note: "Retain existing explanation",
      });
      const activePage = mount("session-one");
      const article = screen
        .getByText(/Synthetic archived ETF ·/)
        .closest("article")!;
      expect(article).toHaveTextContent("Retain existing explanation");
      expect(article).toHaveTextContent(
        i18n.t("reconciliation.restoreBeforeCorrection"),
      );
      expect(
        within(article).queryByRole("button", {
          name: i18n.t("reconciliation.adjust"),
        }),
      ).not.toBeInTheDocument();
      expect(
        within(article).queryByRole("button", {
          name: i18n.t("reconciliation.recordMissing"),
        }),
      ).not.toBeInTheDocument();
      expect(
        within(article).getByRole("button", {
          name: i18n.t("reconciliation.review"),
        }),
      ).toBeInTheDocument();
      if (quantity === "0")
        expect(
          within(article).queryByText(i18n.t("reconciliation.archivedNonzero")),
        ).not.toBeInTheDocument();
      activePage.unmount();
      q.session!.status = "paused";
      mount("session-one");
      // Paused sessions retain direct finish without resuming.
      await userEvent.click(
        screen.getByRole("button", { name: i18n.t("reconciliation.finish") }),
      );
      await userEvent.click(
        screen.getByRole("checkbox", {
          name: i18n.t("reconciliation.allowPartial"),
        }),
      );
      fireEvent.change(
        screen.getByLabelText(i18n.t("reconciliation.closingNote")),
        { target: { value: "Restore position before correcting" } },
      );
      await userEvent.click(
        screen.getByRole("button", { name: i18n.t("reconciliation.preview") }),
      );
      expect(q.preview).toHaveBeenCalledWith(
        expect.objectContaining({
          action: "finish",
          allowPartial: true,
          note: "Restore position before correcting",
        }),
      );
    },
  );
}

it("allows an archived position's evidence disposition through the real check form", async () => {
  Object.assign(q.session!.accounts![0].items![0], {
    kind: "holding",
    holdingId: "held-one",
    archivedNonzero: true,
    label: "Synthetic archived ETF",
    value: "2",
  });
  mount("session-one");
  const article = screen
    .getByText(/Synthetic archived ETF ·/)
    .closest("article")!;
  await userEvent.click(
    within(article).getByRole("button", { name: "Check item" }),
  );
  await userEvent.selectOptions(
    screen.getByLabelText("Check outcome"),
    "unresolved",
  );
  await userEvent.type(
    screen.getByLabelText("Explanation or note"),
    "Restore this holding before correcting",
  );
  await userEvent.click(screen.getByRole("button", { name: "Preview" }));
  expect(q.preview).toHaveBeenCalledWith(
    expect.objectContaining({
      action: "check",
      itemId: "balance",
      outcome: "unresolved",
      note: "Restore this holding before correcting",
    }),
  );
});
