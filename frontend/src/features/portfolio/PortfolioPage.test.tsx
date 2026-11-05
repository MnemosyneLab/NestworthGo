import i18n from "@/i18n";
import { capturePageContent } from "@/features/assistant/pageContent";
import { verifyPageNavigation } from "@/test/assistantNavigation";
import { PageChromeProvider } from "@/components/layout/PageChrome";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { PortfolioPage } from "./PortfolioPage";

const aiAPI = vi.hoisted(() => ({Status: vi.fn(), PendingPageReads: vi.fn(), DeliverPageContent: vi.fn(), PublishPage: vi.fn(), GrantNavigation: vi.fn(), ConsumeNavigation: vi.fn(), List: vi.fn(), Inspect: vi.fn(), Messages: vi.fn(), Start: vi.fn(), Cancel: vi.fn(), Create: vi.fn(), Delete: vi.fn()}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service", () => aiAPI);

const portfolio = vi.fn();
const listAccounts = vi.fn();
const instruments = vi.fn();
const holdingGroups = vi.fn();

vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/portfolio", () => ({
  Service: {
    Portfolio: () => portfolio(),
    PortfolioTrend: () => Promise.resolve({ range: "30d", currency: "USD", points: [] }),
    Overview: vi.fn(),
    NetWorthTrend: vi.fn(),
  },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/catalog", async () => {
  const { TEST_CATALOG } = await import("@/test/catalog");
  return { Service: { Catalog: () => Promise.resolve(TEST_CATALOG) } };
});
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/account", () => ({
  Service: { ListAccounts: () => listAccounts() },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/liquidity", () => ({
  Service: { ListProducts: () => Promise.resolve([]), Overview: vi.fn() },
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument", () => ({Service:{ListInstruments:()=>instruments(),CreateInstrument:vi.fn(),ArchiveInstrument:vi.fn()}}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history", () => ({Service:{HistoryOrigin:()=>Promise.resolve(null)}}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics", () => ({
  Service: { InstrumentHoldings: () => Promise.resolve([]), InstrumentHoldingGroups: () => holdingGroups() },
}));

function renderPage(onOpenAccount?: (id: string) => void, view?: "holdings" | "instruments" | "allocation") {
  const queryClient = createTestQueryClient();
  return render(
    <QueryClientProvider client={queryClient}>
      <PortfolioPage onOpenAccount={onOpenAccount} view={view} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  portfolio.mockReset();
  listAccounts.mockReset();
  listAccounts.mockResolvedValue([]);
  instruments.mockResolvedValue([]); holdingGroups.mockResolvedValue([]);
});

describe("PortfolioPage", () => {
  it("shows only backend-included accounts and opens account detail", async () => {
    const onOpenAccount = vi.fn();
    portfolio.mockResolvedValue({
      currency: "USD",
      complete: true,
      valuedSubtotal: { amount: "320000", currency: "USD" },
      accounts: [
        {
          account: { id: "brk-small", name: "Small Brokerage", accountType: "brokerage", includeInPortfolio: true },
          complete: true,
          baseValue: { amount: "50000", currency: "USD" },
          institutionName: "Small",
          components: [],
          missingInputs: [],
        },
        {
          account: { id: "brk-1", name: "MooMoo SG Brokerage", accountType: "brokerage", includeInPortfolio: true },
          complete: true,
          baseValue: { amount: "180000", currency: "USD" },
          institutionName: "MooMoo SG",
          components: [],
          missingInputs: [],
        },
      ],
      missingInputs: [],
      byInstrumentType: [
        { key: "etf", label: "etf", amount: { amount: "50000", currency: "USD" }, shareBps: 1563 },
        { key: "stock", label: "stock", amount: { amount: "270000", currency: "USD" }, shareBps: 8437 },
      ],
      byCurrency: [],
      byCountry: [],
    });
    renderPage(onOpenAccount, "allocation");
    expect(await screen.findByTestId("portfolio-total")).toHaveTextContent("US$320,000.00");
    expect(screen.getByRole("tab", { name: "Allocation & trend" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getAllByText("Stock").length).toBeGreaterThan(0);
    const stockSlice = screen.getByTestId("composition-slice-stock");
    const etfSlice = screen.getByTestId("composition-slice-etf");
    expect(stockSlice.compareDocumentPosition(etfSlice) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    const largeAccount = screen.getByRole("button", { name: "MooMoo SG Brokerage" });
    const smallAccount = screen.getByRole("button", { name: "Small Brokerage" });
    expect(largeAccount.compareDocumentPosition(smallAccount) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    await userEvent.click(largeAccount);
    expect(onOpenAccount).toHaveBeenCalledWith("brk-1");
  });

  it("shows an empty state when no accounts are included", async () => {
    portfolio.mockResolvedValue({
      currency: "USD",
      complete: true,
      accounts: [],
      missingInputs: [],
      byInstrumentType: [],
      byCurrency: [],
      byCountry: [],
    });
    renderPage(undefined, "allocation");
    expect(await screen.findByText("No holdings yet")).toBeInTheDocument();
  });

  it("does not treat incomplete valuation as zero", async () => {
    portfolio.mockResolvedValue({
      currency: "USD",
      complete: false,
      valuedSubtotal: { amount: "1000", currency: "USD" },
      accounts: [
        {
          account: { id: "brk-1", name: "MooMoo", accountType: "brokerage" },
          complete: false,
          baseValue: { amount: "1000", currency: "USD" },
          components: [],
          missingInputs: [{ kind: "instrument_price", accountId: "brk-1" }],
        },
      ],
      missingInputs: [{ kind: "instrument_price", accountId: "brk-1" }],
      byInstrumentType: [],
      byCurrency: [],
      byCountry: [],
    });
    renderPage(undefined, "allocation");
    expect(await screen.findByText(/excludes values that still need a price/i)).toBeInTheDocument();
    expect(screen.getByText("Partial valuation")).toBeInTheDocument();
    expect(screen.getByTestId("portfolio-total")).toHaveTextContent("US$1,000.00");
  });
});


it("reads all real portfolio tabs, search properties, held values and inactive-tab exclusion", async()=>{
 portfolio.mockResolvedValue({currency:"USD",complete:true,valuedSubtotal:{amount:"123",currency:"USD"},accounts:[{account:{id:"synthetic-account",name:"Synthetic brokerage",accountType:"brokerage"},baseValue:{amount:"123",currency:"USD"},complete:true}],byInstrumentType:[],missingInputs:[]});
 listAccounts.mockResolvedValue([{account:{id:"synthetic-account",name:"Synthetic brokerage",trackingMode:"holdings",accountType:"brokerage"},ownership:[]}]);
 instruments.mockResolvedValue([{id:"synthetic-fund",name:"Synthetic fund",type:"stock",quoteCurrency:"USD"}]);
 const amounts={quantity:"2",totalCost:{amount:"100",currency:"USD"},currentValue:{amount:"123",currency:"USD"},unrealizedGain:{amount:"23",currency:"USD"}};
 holdingGroups.mockResolvedValue([{key:"stock",instruments:[{instrumentId:"synthetic-fund",name:"Synthetic fund",type:"stock",quoteCurrency:"USD",amounts,holdings:[{holdingId:"synthetic-holding",accountId:"synthetic-account",accountName:"Synthetic brokerage",amounts}]}],subtotals:[]}]);
 const target=document.createElement("div");document.body.append(target);
 const view=render(<QueryClientProvider client={createTestQueryClient()}><PageChromeProvider activePageId="portfolio" target={target}><PortfolioPage /></PageChromeProvider></QueryClientProvider>);
 try {
 await screen.findByText("Synthetic fund"); const held=capturePageContent("portfolio:");
 expect(held.nodes.some(n=>n.module==="holdings" && n.name.includes("Synthetic fund"))).toBe(true);
 expect(held.nodes.some(n=>n.module==="summary" && n.name.includes("123.00"))).toBe(true);
 expect(held.nodes.some(n=>n.module==="page" && n.role==="button" && n.name.includes("Add holding"))).toBe(true);
 await userEvent.click(screen.getByLabelText(i18n.t("holdingsSummary.showZero")));
 const zero=capturePageContent("portfolio:");
 expect(zero.nodes.find(n=>n.id==="holdings/holdings-show-zero")?.state).toContain("checked=true");
 expect(zero.id).not.toBe(held.id);
 await userEvent.click(screen.getByRole("button",{name:i18n.t("portfolio.currentValue")}));
 const sorted=capturePageContent("portfolio:");
 expect(sorted.nodes.find(n=>n.role==="columnheader" && n.name===i18n.t("portfolio.currentValue"))?.state).toContain("sort=ascending");
 expect(sorted.id).not.toBe(zero.id);
 await userEvent.click(screen.getByRole("tab",{name:"All instruments"}));
 const search=await screen.findByLabelText("Search instruments"); await userEvent.type(search,"Missing synthetic");
 const saved=capturePageContent("portfolio:");
 expect(saved.nodes.find(n=>n.id==="instruments/instrument-search")?.value).toBe("Missing synthetic");
 expect(saved.id).not.toBe(held.id); expect(saved.nodes.some(n=>n.module==="holdings")).toBe(false);
 await userEvent.click(screen.getByRole("tab",{name:"Allocation & trend"})); await screen.findByTestId("portfolio-page");
 const allocation=capturePageContent("portfolio:");
 expect(allocation.nodes.some(n=>n.module==="allocation-accounts" && n.name.includes("Synthetic brokerage"))).toBe(true);
 expect(allocation.nodes.some(n=>n.module==="allocation-trend")).toBe(true);
 expect(allocation.nodes.some(n=>n.module==="instruments")).toBe(false);
 } finally {view.unmount();target.remove();}
});

it.each(["before-consume","during-consume","unchanged"] as const)("fences real Portfolio search state: %s", async phase=>{
 portfolio.mockResolvedValue({currency:"USD",complete:true,accounts:[],byInstrumentType:[],missingInputs:[]});
 await verifyPageNavigation({api:aiAPI,page:<PortfolioPage view="instruments" />,viewKey:"portfolio:",phase,
  ready:async()=>{await screen.findByLabelText("Search instruments"); await waitFor(()=>expect(document.querySelector('[data-assistant-page="portfolio"]')).toHaveAttribute("aria-busy","false"));},
  change:async()=>{await userEvent.type(screen.getByLabelText("Search instruments"),"Synthetic search"); expect(screen.getByLabelText("Search instruments")).toHaveValue("Synthetic search");},
 });
});


it("reports real portfolio loading and unavailable allocation states", async()=>{
 portfolio.mockImplementation(()=>new Promise(()=>{})); const view=renderPage(undefined,"allocation");
 await screen.findByRole("status"); expect(capturePageContent("portfolio:").nodes.some(n=>n.state.includes("busy=true"))).toBe(true);
 view.unmount(); portfolio.mockRejectedValue(new Error("synthetic unavailable"));
 render(<QueryClientProvider client={createTestQueryClient({retry:false})}><PortfolioPage view="allocation" /></QueryClientProvider>);
 await screen.findByRole("alert"); const page=capturePageContent("portfolio:");
 expect(page.page).toBe("portfolio");expect(page.nodes.some(n=>n.module==="allocation" && n.role==="alert")).toBe(true);
 expect(page.nodes.some(n=>n.module==="allocation-accounts")).toBe(false);
});
