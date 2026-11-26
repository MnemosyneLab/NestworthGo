import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import i18n from "@/i18n";
import type { SyncItemDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/marketdata/models";
import { SyncWorkDetails } from "./SyncWorkDetails";

const items: SyncItemDTO[] = [
  {
    targetKey: "instrument:sol", label: "Solana", symbol: "SOL", kind: "instrument", status: "fetched",
    provider: "coingecko", startDate: "2026-11-25", endDate: "2026-11-25", detail: "2026-11-25..2026-11-25",
  },
  {
    targetKey: "instrument:btc", label: "Bitcoin", symbol: "BTC", kind: "instrument", status: "fetched",
    provider: "coingecko", startDate: "2026-11-24", endDate: "2026-11-25", detail: "2026-11-24..2026-11-25",
  },
  {
    targetKey: "fx:USD/CNY", label: "USD/CNY", kind: "fx", status: "fetched",
    provider: "frankfurter", startDate: "2026-11-25", endDate: "2026-11-25", detail: "2026-11-25..2026-11-25",
  },
  {
    targetKey: "instrument:eth", label: "Ether", symbol: "ETH", kind: "instrument", status: "failed",
    provider: "coingecko", detail: "provider_unavailable",
  },
  {
    targetKey: "instrument:blocked", label: "Blocked fund", kind: "history_instrument", status: "blocked",
    detail: "unsupported_price_basis", startDate: "2026-11-25", endDate: "2026-11-25",
  },
  {
    targetKey: "fx:EUR/USD", label: "EUR/USD", kind: "fx", status: "rate_limited", detail: "provider_rate_limit",
  },
  {
    targetKey: "instrument:planned", label: "Planned", kind: "history_instrument", status: "planned",
    detail: "missing_or_recheck", startDate: "2026-11-01", endDate: "2026-11-02",
  },
  {
    targetKey: "instrument:cached", label: "Cached", kind: "latest_instrument", status: "cached", detail: "recently_checked",
  },
  {
    targetKey: "instrument:skip", label: "Quiet skip", kind: "latest_instrument", status: "skipped", detail: "not_a_real_reason",
  },
  {
    targetKey: "instrument:limited", label: "Limited skip", kind: "latest_instrument", status: "skipped", detail: "rate_limited",
  },
];

function row(name: string): HTMLElement {
  const item = screen.getByText(name).closest("li");
  if (!item) throw new Error(`missing row ${name}`);
  return item;
}

describe("SyncWorkDetails", () => {
  it("shows successful history once and keeps real failures visible", () => {
    render(<SyncWorkDetails items={items} />);
    const unavailable = i18n.t("syncReasons.unavailable");

    const sol = row("Solana · SOL");
    expect(sol).toHaveTextContent(`${i18n.t("marketData.work.history_instrument")} · ${i18n.t("marketData.work.fetched")}`);
    expect(sol).toHaveTextContent("coingecko");
    expect(sol.textContent?.match(/2026-11-25/g)).toEqual(["2026-11-25"]);
    expect(sol).not.toHaveTextContent("2026-11-25..2026-11-25");
    expect(sol).not.toHaveTextContent(unavailable);

    const btc = row("Bitcoin · BTC");
    expect(btc).toHaveTextContent("2026-11-24 – 2026-11-25");
    expect(btc.textContent?.match(/2026-11-24/g)).toEqual(["2026-11-24"]);
    expect(btc.textContent?.match(/2026-11-25/g)).toEqual(["2026-11-25"]);
    expect(btc).not.toHaveTextContent(unavailable);

    const fx = row("USD/CNY");
    expect(fx).toHaveTextContent(`${i18n.t("marketData.work.history_fx")} · ${i18n.t("marketData.work.fetched")}`);
    expect(fx).toHaveTextContent("frankfurter");
    expect(fx.textContent?.match(/2026-11-25/g)).toEqual(["2026-11-25"]);
    expect(fx).not.toHaveTextContent(unavailable);

    const failed = row("Ether · ETH");
    expect(failed).toHaveTextContent(i18n.t("marketData.work.failed"));
    expect(failed).toHaveTextContent(i18n.t("provider_unavailable", { ns: "errorCode" }));

    const blocked = row("Blocked fund");
    expect(blocked).toHaveTextContent(i18n.t("marketData.work.blocked"));
    expect(blocked).toHaveTextContent(i18n.t("dataHealth.reason.unsupported_price_basis"));
    expect(blocked.textContent?.match(/2026-11-25/g)).toEqual(["2026-11-25"]);

    const limited = row("EUR/USD");
    expect(limited).toHaveTextContent(i18n.t("marketData.work.rate_limited"));
    expect(limited).toHaveTextContent(i18n.t("provider_rate_limit", { ns: "errorCode" }));

    expect(row("Planned")).toHaveTextContent(i18n.t("marketData.work.missing_or_recheck"));
    expect(row("Cached")).toHaveTextContent(i18n.t("marketData.work.recently_checked"));
    const quiet = row("Quiet skip");
    expect(quiet).toHaveTextContent(i18n.t("marketData.work.skipped"));
    expect(quiet).not.toHaveTextContent(unavailable);
    expect(quiet).not.toHaveTextContent("not_a_real_reason");
    expect(row("Limited skip")).toHaveTextContent(i18n.t("syncReasons.rate_limited"));
  });

  it.each(["en", "zh-CN", "zh-TW", "ja"] as const)("does not turn the reproduced success detail into a connection error in %s", async (language) => {
    await i18n.changeLanguage(language);
    render(<SyncWorkDetails items={[items[0], items[2]]} />);
    const details = screen.getByTestId("sync-work-details");
    expect(details).toHaveTextContent(i18n.t("marketData.work.fetched"));
    expect(details).not.toHaveTextContent(i18n.t("syncReasons.unavailable"));
    expect(details).not.toHaveTextContent("2026-11-25..2026-11-25");
  });
});
