import { beforeEach, expect, it, vi } from "vitest";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import i18n from "@/i18n";
import { ReconciliationPage } from "./ReconciliationPage";
import { hasUnresolvedSheetConfirmation } from "@/components/ui/sheet-confirmation";
import type {
  ReconciliationSession,
  ReconciliationItem,
} from "../../../bindings/github.com/waltwang/nestworth-go/internal/domain/models";
const q = vi.hoisted(() => ({
  session: undefined as ReconciliationSession | undefined,
  error: false,
  preview: vi.fn(),
  commit: vi.fn(),
  adjust: vi.fn(),
  commitAdjustment: vi.fn(),
  refetch: vi.fn(),
}));
vi.mock("@/queries/reconciliation", () => ({
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
    data: { weekStart: "monday", dateFormat: "iso", timezone: "UTC" },
  }),
}));
vi.mock("@/features/history/RecordChangeForm", () => ({
  RecordChangeForm: ({ initial }: { initial: unknown }) => (
    <div data-testid="record-form">{JSON.stringify(initial)}</div>
  ),
}));
vi.mock("./ProductCorrection", () => ({ ProductCorrection: () => null }));
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
  q.preview.mockResolvedValue({ planId: "review-plan", result: q.session });
  q.commit.mockResolvedValue(q.session);
  q.adjust.mockResolvedValue({ planId: "adjust-plan", changes: [] });
  q.commitAdjustment.mockResolvedValue([]);
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
it.each(["planId", undefined])(
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
