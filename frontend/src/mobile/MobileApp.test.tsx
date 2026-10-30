import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import i18n from "@/i18n";
import { MobileApp } from "./MobileApp";
import { setMobileSession } from "./api";

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
let mode: "success" | "lost" | "revoked";
let healthy = false;
let resolveFirst: (() => void) | undefined;

beforeEach(async () => {
  await i18n.changeLanguage("en");
  sessionStorage.clear();
  setMobileSession(null);
  confirms = [];
  healthy = false;
  mode = "success";
  resolveFirst = undefined;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init: RequestInit) => {
      const route = url.replace("/mobile-api/v1", "");
      if (route === "/session") return json(session);
      if (route === "/bootstrap")
        return json(
          envelope({
            householdId: "synthetic-household",
            name: "SYNTHETIC MOBILE TEST",
            currency: "CNY",
            historyStarted: true,
            today: "2026-10-30",
            time: "12:00:00",
            capabilities: permissions,
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
      if (route === "/record-previews")
        return json({
          planId: "immutable-plan",
          operationId: JSON.parse(init.body as string).operationId,
          kind: "expense",
          timezone: "UTC",
          preview: { activity, resulting: activity.resulting, effects: [] },
        });
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
        if (mode === "revoked")
          return json(
            { error: { code: "session_expired", operationState: "unknown" } },
            401,
          );
        return json({ operationState: "committed", receipt: { activity } });
      }
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
