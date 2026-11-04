import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import i18n from "@/i18n";
import { MobileApp } from "./MobileApp";
import { setMobileSession, type Job } from "./api";

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
let pinnedMarketJob: Job | null;
let marketLost: boolean;
let marketReadError: boolean;
let marketNext: string;
let marketCancelRequests: number;
let marketConfirms: string[];
let healthy = false;
let resolveFirst: (() => void) | undefined;

beforeEach(async () => {
  await i18n.changeLanguage("en");
  sessionStorage.clear();
  localStorage.clear();
  setMobileSession(null);
  confirms = [];
  previews = [];
  marketJob = null;
  pinnedMarketJob = null;
  marketLost = false;
  marketReadError = false;
  marketNext = "";
  marketCancelRequests = 0;
  marketConfirms = [];
  healthy = false;
  mode = "success";
  resolveFirst = undefined;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init: RequestInit) => {
      const route = url.replace("/mobile-api/v1", "");
      if (route === "/assistant/status") return json(envelope({ version: 1, surface: "mobile", pageGuide: true,
        provider: "unavailable", credentials: "unavailable", conversations: "memory", modelRuns: false, financialWrites: false, reason: "activation_incomplete" }));
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
            currency: "CNY",
            trend: [
              {
                date: "2026-10-29",
                valuation: { total: { amount: "90", currency: "CNY" } },
              },
              {
                date: "2026-10-30",
                valuation: { total: { amount: "100", currency: "CNY" } },
              },
            ],
            recent: [],
            distribution: [
              { key: "cash", label: "cash", amount: "100", shareBps: 10000 },
            ],
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
      if (route.startsWith("/market-jobs/") && route.endsWith("/cancel")) {
        marketCancelRequests++;
        return json({ requested: true });
      }
      if (route.startsWith("/market-jobs/"))
        return marketReadError
          ? json({ code: "unavailable" }, 503)
          : json({
              job: route.startsWith("/market-jobs/current?")
                ? marketJob
                : (pinnedMarketJob ?? marketJob),
              next: route.includes("cursor=page2") ? "" : marketNext,
            });
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
  it("opens the local assistant and separate closed review without dispatching a financial change", async () => {
    const user = userEvent.setup(); mount();
    await screen.findByRole("button", { name: "Quick record" });
    const trigger = screen.getByRole("button", { name: "Open AI assistant" });
    await user.click(trigger);
    await screen.findByText("Local page guide ready");
    await user.selectOptions(screen.getByLabelText("Visible metric"), screen.getByRole("option", { name: "Net worth" }));
    await user.click(screen.getByRole("button", { name: "Explain selected metric" }));
    await screen.findByText("Local page guide · no model");
    await user.click(screen.getByRole("button", { name: "Financial review" }));
    expect(screen.getByRole("button", { name: "AI confirmation unavailable" })).toBeDisabled();
    expect(previews).toHaveLength(0); expect(confirms).toHaveLength(0);
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.getByRole("button", { name: "Financial review" })).toHaveFocus());
    await user.keyboard("{Escape}");
    await waitFor(() => expect(trigger).toHaveFocus());
    expect(screen.getByRole("button", { name: "Quick record" })).toBeVisible();
  });
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

  it("shows server shares and keeps exact trend values accessible behind the sparkline", async () => {
    const user = userEvent.setup();
    mount();
    expect(await screen.findByText("100.0%")).toBeInTheDocument();
    await user.click(screen.getByText("Show values"));
    expect(screen.getByText("CN¥90.00")).toBeVisible();
    const range = screen.getByRole("group", { name: "Net worth history" });
    await user.click(within(range).getByRole("button", { name: "One year" }));
    expect(
      within(range).getByRole("button", { name: "One year" }),
    ).toHaveAttribute("aria-pressed", "true");
    await waitFor(() =>
      expect(
        vi.mocked(fetch).mock.calls.some(([url]) =>
          String(url).includes("/overview?range=1y"),
        ),
      ).toBe(true),
    );
  });

  it("keeps preferences and connection controls in the settings panel and marks the current tab", async () => {
    const user = userEvent.setup();
    mount();
    await screen.findByText("SYNTHETIC MOBILE TEST");
    expect(
      screen.queryByRole("button", { name: "Sign out" }),
    ).not.toBeInTheDocument();
    const settings = screen.getByRole("button", { name: "Settings" });
    expect(settings).toHaveAttribute("aria-expanded", "false");
    await user.click(settings);
    expect(settings).toHaveAttribute("aria-expanded", "true");
    const panel = screen.getByRole("region", { name: "Settings" });
    expect(
      within(panel).getByRole("combobox", { name: "Language" }),
    ).toBeInTheDocument();
    expect(
      within(panel).getByRole("combobox", { name: "Theme" }),
    ).toBeInTheDocument();
    expect(
      within(panel).getByRole("button", { name: "Sign out" }),
    ).toBeEnabled();
    const nav = screen.getByRole("navigation", { name: "Main navigation" });
    expect(
      within(nav).getByRole("button", { name: "Overview" }),
    ).toHaveAttribute("aria-current", "page");
    await user.click(within(nav).getByRole("button", { name: "Accounts" }));
    expect(
      within(nav).getByRole("button", { name: "Accounts" }),
    ).toHaveAttribute("aria-current", "page");
    expect(
      within(nav).getByRole("button", { name: "Overview" }),
    ).not.toHaveAttribute("aria-current");
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
  await user.click(screen.getByRole("button", { name: "Settings" }));
  expect(
    screen.queryByRole("button", { name: "Connection settings" }),
  ).not.toBeInTheDocument();
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
  await screen.findByRole("region", { name: i18n.t("mobile.marketJob") });
  expect(screen.queryByText("Synthetic BBB")).not.toBeInTheDocument();
  expect(
    screen.getByText(i18n.t("errorCode:provider_rate_limit")),
  ).toBeInTheDocument();
  await user.click(
    screen.getByRole("button", { name: i18n.t("mobile.viewJob") }),
  );
  expect(screen.getByText("Synthetic BBB")).not.toBeVisible();
  const disclosure = screen.getByRole("button", {
    name: i18n.t("mobile.showJobDetails"),
  });
  disclosure.focus();
  await user.keyboard("{Enter}");
  expect(screen.getByText("Synthetic BBB")).toBeVisible();
  expect(disclosure).toHaveAttribute("aria-expanded", "true");
  expect(screen.getByText(i18n.t("mobile.sharedJob"))).toBeVisible();
  await user.keyboard(" ");
  expect(disclosure).toHaveAttribute("aria-expanded", "false");
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

async function openSettings(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByText("SYNTHETIC MOBILE TEST");
  await user.click(screen.getByRole("button", { name: "Settings" }));
}

it.each(["record", "quote", "market"])(
  "prevents signing out while a %s draft is active",
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
    await openSettings(user);
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
    await user.click(screen.getByRole("button", { name: "Sign out" }));
    expect(screen.getByRole("button", { name: "Sign out" })).toBeDisabled();
    expect(
      screen.queryByLabelText("Backend address or LAN IP:port"),
    ).not.toBeInTheDocument();
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
  "preserves the original receipt and plan when signing out is attempted during %s confirmation",
  async (stage) => {
    mode = "lost";
    const user = userEvent.setup();
    mount();
    await openSettings(user);
    await fillRecord(user);
    await user.click(screen.getByRole("button", { name: "Confirm" }));
    const reference = sessionStorage.getItem("nestworth.mobile.pending.v1");
    try {
      if (stage === "unknown") {
        await act(async () => resolveFirst?.());
        await screen.findByRole("button", { name: "Retry same submission" });
      }
      await user.click(screen.getByRole("button", { name: "Sign out" }));
      expect(sessionStorage.getItem("nestworth.mobile.pending.v1")).toBe(
        reference,
      );
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
          .mock.calls.every(([url]) =>
            String(url).startsWith("/mobile-api/v1/"),
          ),
      ).toBe(true);
    } finally {
      await act(async () => resolveFirst?.());
    }
  },
);

it("preserves unknown market acceptance when signing out is attempted", async () => {
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
  await openSettings(user);
  await user.click(
    screen.getByRole("button", { name: i18n.t("mobile.updateMarket") }),
  );
  await user.click(await screen.findByRole("button", { name: "Confirm" }));
  await screen.findByText(i18n.t("mobile.jobAcceptanceUnknown"));
  await user.click(screen.getByRole("button", { name: "Sign out" }));
  await user.click(
    screen.getByRole("button", { name: "Retry same submission" }),
  );
  await screen.findByRole("button", { name: "Quick record" });
  expect(marketConfirms).toHaveLength(2);
  expect(marketConfirms[0]).toBe(marketConfirms[1]);
});

it("recovers an original receipt after signing in again with the shared token", async () => {
  mode = "revoked";
  const user = userEvent.setup();
  mount();
  await openSettings(user);
  await fillRecord(user);
  await user.click(screen.getByRole("button", { name: "Confirm" }));
  await screen.findByRole("button", { name: "Sign in" });
  expect(
    screen.queryByLabelText("Backend address or LAN IP:port"),
  ).not.toBeInTheDocument();
  const reference = sessionStorage.getItem("nestworth.mobile.pending.v1");
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

function syntheticJob(outcome: string): Job {
  return {
    jobId: "synthetic-shared-job",
    outcome,
    phase: outcome === "running" ? "queued" : "complete",
    completed: outcome === "running" ? 0 : 40,
    total: 40,
    request: { mode: "latest", scope: "stale" },
    source: "shared",
    retryable: false,
    cancellable: false,
    result: { Items: [] },
    items: Array.from({ length: 40 }, (_, index) => ({
      label: `Synthetic target ${index}`,
      status: "fetched",
    })),
  };
}
it.each(["succeeded", "running", "partial", "failed", "cancelled"])(
  "keeps %s task details collapsed and off account/ledger tabs",
  async (outcome) => {
    marketJob = syntheticJob(outcome);
    const user = userEvent.setup();
    mount();
    await screen.findByRole("button", { name: "Update market data" });
    if (outcome === "succeeded")
      expect(
        screen.queryByRole("region", { name: "Market data task" }),
      ).not.toBeInTheDocument();
    else await screen.findByRole("region", { name: "Market data task" });
    for (const tab of ["Accounts", "Ledger"]) {
      await user.click(screen.getByRole("button", { name: tab }));
      expect(
        screen.queryByRole("region", { name: "Market data task" }),
      ).not.toBeInTheDocument();
      if (outcome !== "succeeded")
        expect(
          screen.getByRole("button", { name: /Data health/ }),
        ).toHaveTextContent(
          outcome === "running" ? "Running" : "Task needs attention",
        );
    }
    await user.click(screen.getByRole("button", { name: /Data health/ }));
    const card = await screen.findByRole("region", {
      name: "Market data task",
    });
    const disclosure = within(card).getByRole("button", {
      name: "Show task details",
    });
    expect(disclosure).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByText("Synthetic target 39")).not.toBeVisible();
    if (["partial", "failed", "cancelled"].includes(outcome))
      expect(
        within(card).getByText(i18n.t("mobile.desktopRequired")),
      ).toBeVisible();
    await user.click(disclosure);
    expect(screen.getByText("Synthetic target 39")).toBeVisible();
  },
);
it("observes shared task completion on another tab and refreshes authoritative views", async () => {
  marketJob = syntheticJob("running");
  const user = userEvent.setup();
  const client = mount();
  await screen.findByRole("progressbar", { name: "Market data progress" });
  await user.click(screen.getByRole("button", { name: "Accounts" }));
  const invalidate = vi.spyOn(client, "invalidateQueries");
  marketJob = {
    ...syntheticJob("failed"),
    errorCode: "provider_authentication",
    nextEligibilityAt: "2026-11-05T12:00:00Z",
  };
  await act(async () => {
    await client.invalidateQueries({ queryKey: ["mobile", "job"] });
  });
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: /Data health/ }),
    ).toHaveTextContent("Task needs attention"),
  );
  expect(
    screen.queryByRole("region", { name: "Market data task" }),
  ).not.toBeInTheDocument();
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["mobile", "overview"] });
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["mobile", "/health"] });
  await user.click(screen.getByRole("button", { name: /Data health/ }));
  const card = await screen.findByRole("region", { name: "Market data task" });
  expect(
    within(card).getByText(i18n.t("errorCode:provider_authentication")),
  ).toBeVisible();
  expect(within(card).getByText(/Next eligible attempt/)).toBeVisible();
  expect(
    within(card).getByRole("button", { name: "Show task details" }),
  ).toHaveAttribute("aria-expanded", "false");
});

it("keeps cancellation and status read errors visible without opening details", async () => {
  marketJob = {
    ...syntheticJob("running"),
    cancellable: true,
    source: "mobile",
  };
  const user = userEvent.setup();
  const client = mount();
  await user.click(await screen.findByRole("button", { name: "Cancel task" }));
  expect(marketCancelRequests).toBe(1);
  await screen.findByText(i18n.t("mobile.cancelRequested"));
  expect(screen.getByRole("button", { name: "Cancel task" })).toBeDisabled();
  marketReadError = true;
  await act(async () => {
    await client.invalidateQueries({ queryKey: ["mobile", "job"] });
  });
  await screen.findByRole("button", { name: "Reload task status" });
  expect(screen.getByRole("button", { name: /Data health/ })).toHaveTextContent(
    "Task status unavailable",
  );
  marketReadError = false;
  marketJob = syntheticJob("cancelled");
  await user.click(screen.getByRole("button", { name: "Reload task status" }));
  await screen.findByText(i18n.t("mobile.desktopRequired"));
  expect(marketCancelRequests).toBe(1);
});
it("pins detail pagination and resets disclosure for a new current task", async () => {
  marketJob = syntheticJob("succeeded");
  marketNext = "page2";
  const user = userEvent.setup();
  mount();
  await screen.findByRole("button", { name: "Update market data" });
  await user.click(screen.getByRole("button", { name: /Data health/ }));
  await user.click(
    await screen.findByRole("button", { name: "Show task details" }),
  );
  const card = screen.getByRole("region", { name: "Market data task" });
  await user.click(within(card).getByRole("button", { name: "Next page" }));
  await waitFor(() =>
    expect(
      vi
        .mocked(fetch)
        .mock.calls.some(([url]) =>
          String(url).includes(
            "/market-jobs/synthetic-shared-job?cursor=page2",
          ),
        ),
    ).toBe(true),
  );
  expect(
    await screen.findByRole("button", { name: "Hide task details" }),
  ).toHaveAttribute("aria-expanded", "true");
  marketJob = { ...syntheticJob("running"), jobId: "synthetic-next-job" };
  await user.click(
    within(screen.getByRole("region", { name: "Market data task" })).getByRole(
      "button",
      { name: "First page" },
    ),
  );
  expect(
    await screen.findByRole("button", { name: "Show task details" }),
  ).toHaveAttribute("aria-expanded", "false");
  expect(screen.getByText("Synthetic target 39")).not.toBeVisible();
});

it.each(["Accounts", "Ledger", "Overview", "collapse"])(
  "resumes current shared tasks after leaving pinned details via %s",
  async (destination) => {
    marketJob = syntheticJob("succeeded");
    pinnedMarketJob = marketJob;
    marketNext = "page2";
    const user = userEvent.setup();
    const client = mount();
    await screen.findByRole("button", { name: "Update market data" });
    await user.click(screen.getByRole("button", { name: /Data health/ }));
    await user.click(
      await screen.findByRole("button", { name: "Show task details" }),
    );
    await user.click(
      within(
        screen.getByRole("region", { name: "Market data task" }),
      ).getByRole("button", { name: "Next page" }),
    );
    await screen.findByRole("button", { name: "Hide task details" });
    await waitFor(() =>
      expect(
        vi
          .mocked(fetch)
          .mock.calls.some(([url]) =>
            String(url).includes(
              "/market-jobs/synthetic-shared-job?cursor=page2",
            ),
          ),
      ).toBe(true),
    );
    marketJob = {
      ...syntheticJob("running"),
      jobId: "synthetic-new-shared-job",
    };
    const readStart = vi.mocked(fetch).mock.calls.length;
    await user.click(
      screen.getByRole("button", {
        name: destination === "collapse" ? "Hide task details" : destination,
      }),
    );
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /Data health/ }),
      ).toHaveTextContent("Running"),
    );
    marketJob = {
      ...marketJob,
      outcome: "failed",
      phase: "complete",
      errorCode: "provider_authentication",
    };
    await act(async () => {
      await client.invalidateQueries({ queryKey: ["mobile", "job"] });
    });
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /Data health/ }),
      ).toHaveTextContent("Task needs attention"),
    );
    const subsequentReads = vi
      .mocked(fetch)
      .mock.calls.slice(readStart)
      .map(([url]) => String(url))
      .filter((url) => url.includes("/market-jobs/"));
    expect(subsequentReads.length).toBeGreaterThan(0);
    expect(
      subsequentReads.every((url) =>
        url.endsWith("/market-jobs/current?cursor="),
      ),
    ).toBe(true);
    if (["Accounts", "Ledger"].includes(destination))
      expect(
        screen.queryByRole("region", { name: "Market data task" }),
      ).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Data health/ }));
    const card = await screen.findByRole("region", {
      name: "Market data task",
    });
    expect(
      within(card).getByText(i18n.t("errorCode:provider_authentication")),
    ).toBeVisible();
    expect(
      within(card).getByRole("button", { name: "Show task details" }),
    ).toHaveAttribute("aria-expanded", "false");
    expect(pinnedMarketJob.outcome).toBe("succeeded");
    expect(marketConfirms).toHaveLength(0);
  },
);
