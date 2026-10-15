import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { InstrumentHoldingsTable } from "./InstrumentHoldingsTable";
import type { InstrumentHoldingsDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics/models";
const api=vi.hoisted(()=>({history:vi.fn(),performance:vi.fn(),instruments:vi.fn()}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument",()=>({Service:{ListInstruments:api.instruments}}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote",()=>({Service:{InstrumentQuoteSeries:api.history,InstrumentQuotePerformance:api.performance}}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history",()=>({Service:{HistoryOrigin:()=>Promise.resolve(null)}}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings",()=>({Service:{Load:()=>Promise.resolve({timezone:"UTC"})}}));
const amounts={quantity:"2",totalCost:{amount:"100.00012",currency:"USD"},currentValue:{amount:"110",currency:"USD"},unrealizedGain:{amount:"10",currency:"USD"}};
const group={instrumentId:"i",name:"Synthetic fund",symbol:"SYN",quoteCurrency:"USD",quantityUnit:"",archived:false,amounts,holdings:[{holdingId:"h1",accountId:"a",accountName:"Account A",amounts},{holdingId:"h2",accountId:"b",accountName:"Account B",amounts:{...amounts,totalCost:{amount:"90",currency:"USD"}}}]} as InstrumentHoldingsDTO;
// Compile the real detail/chart module before timing UI interactions. Rendering
// still goes through InstrumentHoldingsTable -> React.lazy -> Suspense, and
// loading code alone must never initiate a service query.
beforeAll(async () => {
 await import("./HoldingDetailPanel");
 expect(api.instruments).not.toHaveBeenCalled();
 expect(api.history).not.toHaveBeenCalled();
 expect(api.performance).not.toHaveBeenCalled();
});
const clients: ReturnType<typeof createTestQueryClient>[] = [];
afterEach(() => {
 for (const client of clients.splice(0)) client.clear();
});
beforeEach(()=>{
 api.history.mockReset().mockResolvedValue({points:[],observations:[],outsideRange:false});
 api.performance.mockReset().mockResolvedValue({items:[{windowDays:7,baselineOn:"2026-10-05",latestOn:"2026-10-14",elapsedDays:9,periodPercent:"33.3333333333333333333333"}]});
 api.instruments.mockReset().mockResolvedValue([{id:"i",name:"Synthetic fund",type:"mutual_fund",quoteCurrency:"USD",countryCode:"US",symbol:"SYN",quoteSource:"agent"}]);
});
it("loads only expanded active details, shares history across accounts, and reuses cache after collapse",async()=>{
 const client=createTestQueryClient();clients.push(client);const view=render(<QueryClientProvider client={client}><InstrumentHoldingsTable groups={[group]} /></QueryClientProvider>);
 expect(api.instruments).not.toHaveBeenCalled();expect(api.history).not.toHaveBeenCalled();expect(api.performance).not.toHaveBeenCalled();
 await userEvent.click(screen.getByRole("button",{name:"Account details"}));
 const buttons=screen.getAllByRole("button",{name:"Show details for Synthetic fund"});
 await userEvent.click(buttons[0]);
 expect(buttons[0]).toHaveAttribute("aria-expanded", "true");
 await act(() => vi.dynamicImportSettled());
 await waitFor(() => expect(api.instruments).toHaveBeenCalledWith(true));
 await waitFor(() => expect(api.performance).toHaveBeenCalledWith("i"));
 await screen.findByText("2026-10-05 → 2026-10-14 · 9 actual days");
 expect(screen.getByText("33.33%")).toBeInTheDocument();
 await userEvent.click(buttons[1]);await waitFor(()=>expect(screen.getAllByText("As of 2026-10-14")).toHaveLength(2));
 expect(api.history).toHaveBeenCalledTimes(1);expect(api.performance).toHaveBeenCalledTimes(1);expect(api.instruments).toHaveBeenCalledTimes(1);
 expect(screen.getAllByText("$90.00")).toHaveLength(2);expect(screen.getAllByText("+\u002410.00")).toHaveLength(2);
 await userEvent.click(buttons[0]);await userEvent.click(buttons[0]);expect(api.history).toHaveBeenCalledTimes(1);
 view.rerender(<QueryClientProvider client={client}><InstrumentHoldingsTable groups={[group]} active={false} /></QueryClientProvider>);
 expect(screen.queryByText("Historical price / NAV performance")).not.toBeInTheDocument();
 expect(api.history).toHaveBeenCalledTimes(1);
});

it("waits for instrument metadata and performance independently before showing fixed observation dates", async () => {
 let resolveInstruments!: (value: unknown) => void;
 let resolvePerformance!: (value: unknown) => void;
 api.instruments.mockImplementationOnce(() => new Promise(resolve => { resolveInstruments = resolve; }));
 api.performance.mockImplementationOnce(() => new Promise(resolve => { resolvePerformance = resolve; }));
 const client = createTestQueryClient();
 clients.push(client);
 render(<QueryClientProvider client={client}><InstrumentHoldingsTable groups={[group]} /></QueryClientProvider>);
 expect(api.instruments).not.toHaveBeenCalled();
 await userEvent.click(screen.getByRole("button", { name: "Show details for Synthetic fund" }));
 const button = screen.getByRole("button", { name: "Hide details for Synthetic fund" });
 expect(button).toHaveAttribute("aria-expanded", "true");
 await act(() => vi.dynamicImportSettled());
 await waitFor(() => expect(api.instruments).toHaveBeenCalledWith(true));
 const detail = within(document.getElementById(button.getAttribute("aria-controls")!)!);
 expect(detail.getByRole("status")).toHaveTextContent("Loading");
 expect(api.performance).not.toHaveBeenCalled();
 expect(api.history).not.toHaveBeenCalled();
 await act(async () => resolveInstruments([{id:"i",name:"Synthetic fund",type:"mutual_fund",quoteCurrency:"USD",quoteSource:"agent"}]));
 await waitFor(() => expect(api.performance).toHaveBeenCalledWith("i"));
 expect(detail.queryByText("2026-10-05 → 2026-10-14 · 9 actual days")).not.toBeInTheDocument();
 expect(detail.queryByText("33.33%")).not.toBeInTheDocument();
 await act(async () => resolvePerformance({items:[{windowDays:7,baselineOn:"2026-10-05",latestOn:"2026-10-14",elapsedDays:9,periodPercent:"33.3333333333333333333333"}]}));
 expect(await detail.findByText("2026-10-05 → 2026-10-14 · 9 actual days")).toBeInTheDocument();
 expect(detail.getByText("33.33%")).toBeInTheDocument();
 expect(detail.getByText("As of 2026-10-14")).toBeInTheDocument();
});

it.each(["insufficient_daily_history", "incomparable_daily_prices", "unsupported_instrument", "unknown_internal_reason"])("explains %s without hiding the chart", async reason => {
 api.performance.mockResolvedValue({items:[],unavailableReason:reason});
 const client=createTestQueryClient(); clients.push(client);
 render(<QueryClientProvider client={client}><InstrumentHoldingsTable groups={[group]} /></QueryClientProvider>);
 await userEvent.click(screen.getByRole("button",{name:"Show details for Synthetic fund"}));
 await act(() => vi.dynamicImportSettled());
 const section=await screen.findByRole("region",{name:"Historical price / NAV performance"});
 expect(section.textContent).not.toContain(reason);
 expect(section.textContent).toMatch(/seven calendar days|cannot be compared|instrument type/);
 expect(screen.queryByText("7-day target")).not.toBeInTheDocument();
 expect(await screen.findByText("No local history is saved yet.")).toBeInTheDocument();
});

it("shows zero annualization, only available windows and amounts before secondary metadata/notes", async () => {
 api.performance.mockResolvedValue({items:[{windowDays:7,baselineOn:"2026-10-05",latestOn:"2026-10-14",elapsedDays:9,periodPercent:"0",annualizedPercent:"0"}]});
 api.instruments.mockResolvedValue([{id:"i",name:"Synthetic fund",type:"bank_investment_product",quoteCurrency:"USD",countryCode:"US",symbol:"SYN",note:"Long synthetic note ".repeat(50)}]);
 const client=createTestQueryClient();clients.push(client);
 render(<QueryClientProvider client={client}><InstrumentHoldingsTable groups={[group]} /></QueryClientProvider>);
 await userEvent.click(screen.getByRole("button",{name:"Show details for Synthetic fund"}));
 await act(() => vi.dynamicImportSettled());
 expect(await screen.findByText("7-day target · Simple annualized NAV change")).toBeInTheDocument();
 expect(screen.getByText("0.00%")).toBeInTheDocument();
 expect(screen.queryByText(/30-day target/)).not.toBeInTheDocument();
 const detail=screen.getByRole("region",{name:/SYN · All included accounts/});
 const content=detail.textContent!;
 expect(content.indexOf("Cost")).toBeLessThan(content.indexOf("Historical price / NAV performance"));
 expect(content.indexOf("Historical price / NAV performance")).toBeLessThan(content.indexOf("Country"));
 const notes=detail.querySelector("details")!;
 expect(notes).not.toHaveAttribute("open");
 await userEvent.click(within(notes).getByText("Note"));
 expect(notes).toHaveAttribute("open");
});
