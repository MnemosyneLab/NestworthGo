import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import i18n from "@/i18n";
import { MobileApp } from "./MobileApp";
import { setMobileSession, type Job } from "./api";
import { backendBase, saveBackend } from "./connection";

const permissions = {
  read: true,
  record: true,
  market_refresh: false,
  manual_quote: true,
};
const session = {
  device: { id: "synthetic-phone", name: "Synthetic phone", permissions },
  csrfToken: "synthetic-csrf",
  instance: "synthetic-instance",
};
const activity = {
  id: "synthetic-receipt",
  kind: "cash_out",
  reason: "expense",
  effectiveAt: "2026-10-30T12:00:00Z",
  effectiveLocalDate: "2026-10-30",
  resulting: [{ name: "Synthetic bank", amount: "98.7655", currency: "CNY" }],
  effects: [],
};
const envelope = <T,>(data: T) => ({
  data,
  ledgerTimezone: "UTC",
  asOf: "2026-10-30T12:00:00Z",
  dataVersion: "synthetic-version",
});
const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
let confirms: { body: string; headers: Record<string, string> }[];
let mode: "success" | "lost" | "revoked" | "stale";
let previews: Record<string, string>[];
let marketJob: Job | null;
let marketLost: boolean;
let marketConfirms: string[];
let healthy = false;
let resolveFirst: (() => void) | undefined;

beforeEach(async () => {
  await i18n.changeLanguage("en");
  sessionStorage.clear();
  localStorage.clear();
  saveBackend("");
  setMobileSession(null);
  confirms = [];
  previews = [];
  marketJob = null;
  marketLost = false;
  marketConfirms = [];
  healthy = false;
  mode = "success";
  resolveFirst = undefined;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init: RequestInit) => {
      const route = url.replace("/mobile-api/v1", "");
      if (route === "/session")
        return json({
          ...session,
          device: {
            ...session.device,
            permissions: { ...permissions, market_refresh: !!marketJob },
          },
        });
      if (route === "/bootstrap")
        return json(
          envelope({
            householdId: "synthetic-household",
            name: "SYNTHETIC MOBILE TEST",
            currency: "CNY",
            historyStarted: true,
            today: "2026-10-30",
            time: "12:00:00",
            capabilities: { ...permissions, market_refresh: !!marketJob },
          }),
        );
      if (route.startsWith("/overview"))
        return json(
          envelope({
            valuation: {
              netWorth: { total: { amount: "100", currency: "CNY" } },
              assets: { total: { amount: "100", currency: "CNY" } },
              liabilities: { total: { amount: "0", currency: "CNY" } },
            },
            complete: true,
            trend: [],
            recent: [],
            distribution: [],
          }),
        );
      if (route.startsWith("/accounts?"))
        return json(
          envelope({
            items: [
              {
                id: "synthetic-bank",
                name: "Synthetic bank",
                currency: "CNY",
                mode: "balance",
                type: "bank_account",
                role: "asset",
                value: { amount: "100", currency: "CNY" },
                complete: true,
              },
            ],
            next: "",
          }),
        );
      if (route === "/accounts/synthetic-bank")
        return json(
          envelope({ account: { name: "Synthetic bank" }, components: [] }),
        );
      if (route.startsWith("/holdings?"))
        return json(
          envelope({
            items: [
              {
                id: "synthetic-holding",
                name: "Synthetic fund",
                quantity: "2",
                cost: { amount: "37.25", currency: "CNY" },
                value: null,
                gain: null,
              },
            ],
            next: "",
          }),
        );
      if (route === "/holdings/synthetic-holding")
        return json(
          envelope({
            holding: {
              InstrumentName: "Synthetic fund",
              InstrumentID: "synthetic-instrument",
              Quantity: "2",
              TotalCost: { amount: "37.25", currency: "CNY" },
              UnrealizedGain: null,
              RealizedGain: null,
            },
            valuation: { components: [] },
            quote: { unitPrice: "1.25", currency: "CNY" },
          }),
        );
      if (route === "/record-previews") {
        previews.push(JSON.parse(init.body as string));
        return json({
          planId: "immutable-plan",
          operationId: JSON.parse(init.body as string).operationId,
          kind: "expense",
          timezone: "UTC",
          preview: { activity, resulting: activity.resulting, effects: [] },
        });
      }
      if (route === "/record-confirmations") {
        confirms.push({
          body: init.body as string,
          headers: init.headers as Record<string, string>,
        });
        if (mode === "lost" && confirms.length === 1) {
          await new Promise<void>((resolve) => {
            resolveFirst = resolve;
          });
          throw new TypeError("synthetic lost response");
        }
        if (mode === "stale")
          return json(
            {
              error: {
                code: "stale_preview",
                field: "previewToken",
                operationState: "rejected",
              },
            },
            409,
          );
        if (mode === "revoked")
          return json(
            { error: { code: "session_expired", operationState: "unknown" } },
            401,
          );
        return json({ operationState: "committed", receipt: { activity } });
      }
      if (route === "/market-previews")
        return json({
          planId: "market-immutable-plan",
          request: JSON.parse(init.body as string),
        });
      if (route === "/market-jobs" && init.method === "POST") {
        marketConfirms.push(init.body as string);
        if (marketLost && marketConfirms.length === 1)
          throw new TypeError("synthetic lost acceptance");
        return json({
          job: marketJob,
          conflict: false,
          attached: marketConfirms.length > 1,
        });
      }
      if (route.startsWith("/market-jobs/"))
        return json({ job: marketJob, next: "" });
      if (route.startsWith("/operations/"))
        return json({ operationState: "committed", receipt: { activity } });
      if (route.startsWith("/health") && healthy)
        return json(envelope({ healthy: true, items: null, next: "" }));
      if (route.startsWith("/health"))
        return json(
          envelope({
            healthy: false,
            items: [
              {
                ID: "synthetic-issue",
                Label: "Synthetic fund",
                Kind: "missing_manual_price",
                Action: "manual_entry",
                Reason: "missing_quote",
                InstrumentID: "synthetic-fund",
                RangeEnd: "2026-10-29",
                Collapsed: false,
              },
            ],
            next: "",
          }),
        );
      throw new Error(`unexpected test route ${route}`);
    }),
  );
});
afterEach(() => vi.unstubAllGlobals());
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MobileApp />
    </QueryClientProvider>,
  );
  return client;
}
async function fillRecord(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByText("SYNTHETIC MOBILE TEST");
  await user.click(screen.getByRole("button", { name: "Quick record" }));
  await user.selectOptions(
    await screen.findByRole("combobox", { name: "Account" }),
    "synthetic-bank",
  );
  await user.type(screen.getByRole("textbox", { name: "Amount" }), "1.2345");
  await user.click(screen.getByRole("button", { name: "Preview" }));
  await screen.findByText("Review account effects before confirming.");
}

describe("mobile financial interruption flow", () => {
  it("shows a known holding cost from the tagged money DTO instead of a blank amount", async () => {
    const user = userEvent.setup();
    mount();
    await screen.findByText("SYNTHETIC MOBILE TEST");
    await user.click(screen.getByRole("button", { name: "Accounts" }));
    await user.click(
      await screen.findByRole("button", { name: /Synthetic bank/ }),
    );
    await user.click(
      await screen.findByRole("button", { name: /Synthetic fund/ }),
    );
    expect((await screen.findByText("Cost")).parentElement).toHaveTextContent(
      "CN¥37.25",
    );
  });

  it("renders a healthy empty report without losing the application", async () => {
    healthy = true;
    const user = userEvent.setup();
    mount();
    await screen.findByText("SYNTHETIC MOBILE TEST");
    await user.click(screen.getByRole("button", { name: "Data health" }));
    await screen.findByText("No outstanding local data issues.");
    expect(
      screen.getByRole("button", { name: "Quick record" }),
    ).toBeInTheDocument();
  });

  it("recovers a lost response through the receipt query without dispatching again", async () => {
    mode = "lost";
    const user = userEvent.setup();
    mount();
    await fillRecord(user);
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await act(async () => resolveFirst?.());
    await user.click(
      await screen.findByRole("button", { name: "Check original result" }),
    );
    await screen.findByText("Saved successfully");
    expect(confirms).toHaveLength(1);
  });

  it("keeps the reviewed payload and one operation ID through a lost response and repeated clicks", async () => {
    mode = "lost";
    const user = userEvent.setup();
    mount();
    await fillRecord(user);
    expect(screen.getByRole("textbox", { name: "Amount" })).toBeDisabled();
    await user.dblClick(screen.getByRole("button", { name: "Confirm" }));
    expect(confirms).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    const saved = JSON.parse(
      sessionStorage.getItem("nestworth.mobile.pending.v1")!,
    );
    expect(Object.keys(saved).sort()).toEqual(["householdId", "operationId"]);
    expect(JSON.stringify(saved)).not.toContain("1.2345");
    await act(async () => resolveFirst?.());
    await user.click(
      await screen.findByRole("button", { name: "Retry same submission" }),
    );
    await screen.findByText("Saved successfully");
    expect(confirms).toHaveLength(2);
    expect(confirms[0].body).toBe(confirms[1].body);
    expect(confirms[1].headers["X-CSRF-Token"]).toBe("synthetic-csrf");
    expect(sessionStorage.getItem("nestworth.mobile.pending.v1")).toBeNull();
  });

  it("clears financial query data after revocation and retains only the pending recovery identity", async () => {
    mode = "revoked";
    const user = userEvent.setup();
    const client = mount();
    await fillRecord(user);
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await screen.findByRole("button", { name: "Sign in" });
    await waitFor(() =>
      expect(client.getQueryData(["mobile", "bootstrap"])).toBeUndefined(),
    );
    expect(screen.queryByText("Synthetic bank")).not.toBeInTheDocument();
    expect(
      sessionStorage.getItem("nestworth.mobile.pending.v1"),
    ).not.toBeNull();
  });

  it.each(["zh-CN", "zh-TW"])(
    "uses real locale resources for manual health repair in %s",
    async (language) => {
      await i18n.changeLanguage(language);
      const user = userEvent.setup();
      mount();
      await screen.findByText("SYNTHETIC MOBILE TEST");
      await user.click(
        screen.getByRole("button", { name: i18n.t("mobile.health") }),
      );
      await screen.findByText("Synthetic fund");
      await user.click(
        screen.getByRole("button", { name: i18n.t("mobile.manualQuote") }),
      );
      expect(
        screen.getByRole("textbox", { name: i18n.t("mobile.quoteValue") }),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: i18n.t("mobile.effectiveDate") }),
      ).toBeInTheDocument();
      expect(screen.getByText(i18n.t("mobile.manualHelp"))).toBeInTheDocument();
    },
  );
});

it("continues with a fresh operation ID and cleared values while keeping the chosen account", async () => {
  const user = userEvent.setup();
  mount();
  await fillRecord(user);
  const firstID = previews[0].operationId;
  await user.click(
    screen.getByRole("button", { name: "Confirm and add another" }),
  );
  await screen.findByText("Saved successfully");
  const amount = screen.getByRole("textbox", { name: "Amount" });
  expect(amount).toHaveValue("");
  expect(amount).toHaveFocus();
  expect(screen.getByRole("combobox", { name: "Account" })).toHaveValue(
    "synthetic-bank",
  );
  expect(screen.getByRole("textbox", { name: "Note" })).toHaveValue("");
  expect(
    screen.queryByText("Review account effects before confirming."),
  ).not.toBeInTheDocument();
  await user.type(amount, "2.0009");
  await user.click(screen.getByRole("button", { name: "Preview" }));
  await screen.findByText("Review account effects before confirming.");
  expect(previews[1].operationId).not.toBe(firstID);
  expect(previews[1].amount).toBe("2.0009");
  expect(previews[1].effectiveLocalTime).toBeUndefined();
  await user.click(screen.getByRole("button", { name: "Confirm" }));
  await waitFor(() =>
    expect(
      screen.queryByRole("textbox", { name: "Amount" }),
    ).not.toBeInTheDocument(),
  );
  expect(confirms).toHaveLength(2);
});

it("releases a proven stale review without losing the draft and guards navigation and cancellation", async () => {
  mode = "stale";
  const user = userEvent.setup();
  mount();
  await fillRecord(user);
  expect(
    screen.getByRole("button", { name: "Connection settings" }),
  ).toBeDisabled();
  expect(screen.getByRole("button", { name: "Sign out" })).toBeDisabled();
  expect(
    screen.queryByRole("button", { name: "Accounts" }),
  ).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Confirm" }));
  await waitFor(() =>
    expect(screen.getByRole("textbox", { name: "Amount" })).toBeEnabled(),
  );
  expect(screen.getByRole("textbox", { name: "Amount" })).toHaveValue("1.2345");
  expect(sessionStorage.getItem("nestworth.mobile.pending.v1")).toBeNull();
  await user.click(screen.getByRole("button", { name: "Cancel" }));
  await screen.findByRole("alertdialog");
  await user.click(screen.getByRole("button", { name: "Discard" }));
  await screen.findByRole("button", { name: "Quick record" });
});

it("keeps the same market acceptance after a lost response and shows partial failure causes", async () => {
  marketLost = true;
  marketJob = {
    jobId: "shared-job",
    outcome: "partial",
    phase: "complete",
    completed: 2,
    total: 2,
    request: { mode: "latest", scope: "stale" },
    retryable: true,
    source: "shared",
    items: [
      { label: "Synthetic AAA", status: "fetched" },
      {
        label: "Synthetic BBB",
        status: "rate_limited",
        errorCode: "provider_rate_limit",
      },
    ],
    result: { Items: [] },
  };
  const user = userEvent.setup();
  mount();
  await screen.findByText("Synthetic BBB", { exact: false });
  expect(
    screen.getByText(i18n.t("errorCode:provider_rate_limit")),
  ).toBeInTheDocument();
  expect(screen.getByText(i18n.t("mobile.sharedJob"))).toBeInTheDocument();
  await user.click(
    screen.getByRole("button", { name: i18n.t("mobile.retryFailed") }),
  );
  await user.click(await screen.findByRole("button", { name: "Confirm" }));
  await screen.findByText(i18n.t("mobile.jobAcceptanceUnknown"));
  expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
  await user.click(
    screen.getByRole("button", { name: "Retry same submission" }),
  );
  await screen.findByRole("button", { name: "Quick record" });
  expect(marketConfirms).toHaveLength(2);
  expect(marketConfirms[0]).toBe(marketConfirms[1]);
});

it("reconnects with a mounted draft intact and never sends another write", async () => {
  const user = userEvent.setup();
  mount();
  await fillRecord(user);
  await user.click(screen.getByRole("button", { name: "Reconnect" }));
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "Reconnect" })).toBeEnabled(),
  );
  expect(screen.getByRole("textbox", { name: "Amount" })).toHaveValue("1.2345");
  expect(screen.getByRole("textbox", { name: "Amount" })).toBeDisabled();
  expect(
    screen.getByText("Review account effects before confirming."),
  ).toBeInTheDocument();
  expect(confirms).toHaveLength(0);
});

it("retains the continuous-entry choice through a lost confirmation and receipt recovery", async () => {
  mode = "lost";
  const user = userEvent.setup();
  mount();
  await fillRecord(user);
  await user.click(
    screen.getByRole("button", { name: "Confirm and add another" }),
  );
  expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
  await act(async () => resolveFirst?.());
  await user.click(
    await screen.findByRole("button", { name: "Check original result" }),
  );
  await screen.findByText("Saved successfully");
  expect(screen.getByRole("textbox", { name: "Amount" })).toHaveValue("");
  expect(screen.getByRole("textbox", { name: "Amount" })).toBeEnabled();
  expect(confirms).toHaveLength(1);
});

it("keeps recovery in memory when browser session storage is unavailable", async () => {
  mode = "lost";
  const original = Storage.prototype.setItem;
  const write = vi
    .spyOn(Storage.prototype, "setItem")
    .mockImplementation(function (this: Storage, key, value) {
      if (key.startsWith("nestworth.mobile.pending"))
        throw new DOMException("storage unavailable", "SecurityError");
      original.call(this, key, value);
    });
  try {
    const user = userEvent.setup();
    mount();
    await fillRecord(user);
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    await screen.findByText(i18n.t("mobile.recoveryMemoryOnly"));
    await act(async () => resolveFirst?.());
    await user.click(
      await screen.findByRole("button", { name: "Check original result" }),
    );
    await screen.findByText("Saved successfully");
    expect(confirms).toHaveLength(1);
  } finally {
    write.mockRestore();
  }
});

it("cancels pairing and ignores a delayed response without storing its token", async () => {
  let finish!: (value: Response) => void;
  let exchanges = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init: RequestInit) => {
      if (url.endsWith("/session") && init.method === "POST") {
        exchanges++;
        return new Promise<Response>((resolve) => {
          finish = resolve;
        });
      }
      return Promise.resolve(json({ error: { code: "session_expired" } }, 401));
    }),
  );
  const user = userEvent.setup();
  mount();
  await screen.findByRole("button", { name: "Sign in" });
  await user.type(screen.getByLabelText("Mobile token"), "synthetic-one-use");
  await user.dblClick(screen.getByRole("button", { name: "Sign in" }));
  expect(exchanges).toBe(1);
  await user.click(screen.getByRole("button", { name: "Cancel" }));
  await act(async () =>
    finish(json({ ...session, accessToken: "synthetic-session-token" })),
  );
  await screen.findByText(i18n.t("mobile.transport.cancelled"));
  expect(
    screen.queryByRole("button", { name: "Quick record" }),
  ).not.toBeInTheDocument();
  expect(JSON.stringify(localStorage)).not.toMatch(
    /synthetic-one-use|synthetic-session-token/,
  );
});

async function prepareServerSwitch(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByText("SYNTHETIC MOBILE TEST");
  await user.click(screen.getByRole("button", { name: "Connection settings" }));
  await user.type(
    screen.getByLabelText("Backend HTTPS address"),
    "https://other.synthetic.example",
  );
}

it.each(["record", "quote", "market"])(
  "locks an already-open connection panel while a %s draft is active",
  async (kind) => {
    if (kind === "market")
      marketJob = {
        jobId: "existing",
        request: { mode: "latest", scope: "stale" },
        outcome: "succeeded",
        phase: "complete",
        completed: 0,
        total: 0,
        result: { Items: [] },
      };
    const user = userEvent.setup();
    mount();
    await prepareServerSwitch(user);
    if (kind === "record") await fillRecord(user);
    else if (kind === "quote") {
      await user.click(screen.getByRole("button", { name: "Data health" }));
      await user.click(
        await screen.findByRole("button", {
          name: i18n.t("mobile.manualQuote"),
        }),
      );
    } else
      await user.click(
        screen.getByRole("button", { name: i18n.t("mobile.updateMarket") }),
      );
    await user.click(screen.getByRole("button", { name: "Use this server" }));
    expect(backendBase()).toBe("");
    expect(
      screen.getByRole("button", { name: "Use this server" }),
    ).toBeDisabled();
    expect(screen.getByLabelText("Backend HTTPS address")).toBeDisabled();
    if (kind === "record")
      expect(screen.getByRole("textbox", { name: "Amount" })).toHaveValue(
        "1.2345",
      );
    if (kind === "quote")
      expect(
        screen.getByRole("textbox", { name: i18n.t("mobile.quoteValue") }),
      ).toBeInTheDocument();
    if (kind === "market")
      await screen.findByRole("button", { name: "Confirm" });
    expect(confirms).toHaveLength(0);
    expect(marketConfirms).toHaveLength(0);
  },
);

it.each(["pending", "unknown"])(
  "preserves the original receipt and plan when switching is attempted during %s confirmation",
  async (stage) => {
    mode = "lost";
    const user = userEvent.setup();
    mount();
    await prepareServerSwitch(user);
    await fillRecord(user);
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    const reference = sessionStorage.getItem("nestworth.mobile.pending.v1");
    try {
      if (stage === "unknown") {
        await act(async () => resolveFirst?.());
        await screen.findByRole("button", { name: "Retry same submission" });
      }
      await user.click(screen.getByRole("button", { name: "Use this server" }));
      expect(backendBase()).toBe("");
      expect(sessionStorage.getItem("nestworth.mobile.pending.v1")).toBe(
        reference,
      );
      expect(
        sessionStorage.getItem(
          "nestworth.mobile.pending.v1:https://other.synthetic.example",
        ),
      ).toBeNull();
      if (stage === "pending") await act(async () => resolveFirst?.());
      await user.click(
        await screen.findByRole("button", { name: "Retry same submission" }),
      );
      await screen.findByText("Saved successfully");
      expect(confirms).toHaveLength(2);
      expect(confirms[0].body).toBe(confirms[1].body);
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some(([url]) =>
            String(url).startsWith("https://other.synthetic.example"),
          ),
      ).toBe(false);
    } finally {
      await act(async () => resolveFirst?.());
    }
  },
);

it("preserves unknown market acceptance when a previously opened connection panel is used", async () => {
  marketLost = true;
  marketJob = {
    jobId: "accepted",
    request: { mode: "latest", scope: "stale" },
    outcome: "succeeded",
    phase: "complete",
    completed: 0,
    total: 0,
    result: { Items: [] },
  };
  const user = userEvent.setup();
  mount();
  await prepareServerSwitch(user);
  await user.click(
    screen.getByRole("button", { name: i18n.t("mobile.updateMarket") }),
  );
  await user.click(await screen.findByRole("button", { name: "Confirm" }));
  await screen.findByText(i18n.t("mobile.jobAcceptanceUnknown"));
  await user.click(screen.getByRole("button", { name: "Use this server" }));
  expect(backendBase()).toBe("");
  await user.click(
    screen.getByRole("button", { name: "Retry same submission" }),
  );
  await screen.findByRole("button", { name: "Quick record" });
  expect(marketConfirms).toHaveLength(2);
  expect(marketConfirms[0]).toBe(marketConfirms[1]);
});

it("allows a server change when no draft or recovery reference is active", async () => {
  const user = userEvent.setup();
  mount();
  await prepareServerSwitch(user);
  await user.click(screen.getByRole("button", { name: "Use this server" }));
  await screen.findByRole("button", { name: "Sign in" });
  expect(backendBase()).toBe("https://other.synthetic.example");
  expect(confirms).toHaveLength(0);
});

it("keeps server switching locked during re-pairing while an original receipt remains unresolved", async () => {
  mode = "revoked";
  const user = userEvent.setup();
  mount();
  await prepareServerSwitch(user);
  await fillRecord(user);
  await user.click(screen.getByRole("button", { name: "Confirm" }));
  await screen.findByRole("button", { name: "Sign in" });
  expect(screen.getByLabelText("Backend HTTPS address")).toBeDisabled();
  expect(
    screen.getByRole("button", { name: "Use this server" }),
  ).toBeDisabled();
  const reference = sessionStorage.getItem("nestworth.mobile.pending.v1");
  await user.click(screen.getByRole("button", { name: "Use this server" }));
  expect(backendBase()).toBe("");
  expect(sessionStorage.getItem("nestworth.mobile.pending.v1")).toBe(reference);
  await user.type(screen.getByLabelText("Mobile token"), "synthetic-new-pair");
  await user.click(screen.getByRole("button", { name: "Sign in" }));
  await user.click(
    await screen.findByRole("button", { name: "Check original result" }),
  );
  await screen.findByText("Saved successfully");
  expect(confirms).toHaveLength(1);
  expect(sessionStorage.getItem("nestworth.mobile.pending.v1")).toBeNull();
});
