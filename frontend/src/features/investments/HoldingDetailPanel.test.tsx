import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { HoldingGroupsList } from "./HoldingGroupsList";
import { useHoldingGroupStore } from "@/stores/holdings";
import type { InstrumentHoldingGroupDTO, InstrumentHoldingsDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics/models";
const api=vi.hoisted(()=>({history:vi.fn(),periods:vi.fn(),instruments:vi.fn()}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument",()=>({Service:{ListInstruments:api.instruments}}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote",()=>({Service:{InstrumentQuoteSeries:api.history,InstrumentQuotePeriods:api.periods}}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history",()=>({Service:{HistoryOrigin:()=>Promise.resolve(null)}}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings",()=>({Service:{Load:()=>Promise.resolve({timezone:"UTC"})}}));
const amounts={quantity:"2",totalCost:{amount:"100.00012",currency:"USD"},currentValue:{amount:"110",currency:"USD"},unrealizedGain:{amount:"10",currency:"USD"}};
const group={instrumentId:"i",type:"mutual_fund",name:"Synthetic fund",symbol:"SYN",quoteCurrency:"USD",quantityUnit:"",archived:false,amounts,holdings:[{holdingId:"h1",accountId:"a",accountName:"Account A",amounts},{holdingId:"h2",accountId:"b",accountName:"Account B",amounts:{...amounts,totalCost:{amount:"90",currency:"USD"}}}]} as InstrumentHoldingsDTO;
const groups=(item=group,key="fund"):InstrumentHoldingGroupDTO[]=>[{key,instruments:[item],subtotals:[]}];
const item=(windowDays:number,periodPercent:string,extra:Record<string,string>={})=>({windowDays,baselineOn:"2026-10-05",latestOn:"2026-10-14",elapsedDays:9,periodPercent,...extra});
const point={quotedAt:"2026-10-14T00:00:00Z",effectiveDate:"2026-10-14",timestampBasis:"date_label",value:"1.5",currency:"USD",sourceKind:"agent",sourceKey:"",priceBasis:"unit_nav",observationKind:"daily_close",delayed:false};
// Compile the real detail/chart module before timing UI interactions. Rendering
// still goes through HoldingGroupsList -> React.lazy -> Suspense, and loading
// code alone must never initiate a service query.
beforeAll(async () => {
 await import("./HoldingDetailPanel");
 expect(api.instruments).not.toHaveBeenCalled();
 expect(api.history).not.toHaveBeenCalled();
 expect(api.periods).not.toHaveBeenCalled();
});
const clients: ReturnType<typeof createTestQueryClient>[] = [];
afterEach(() => {
 for (const client of clients.splice(0)) client.clear();
});
beforeEach(()=>{
 useHoldingGroupStore.getState().reset();
 api.history.mockReset().mockResolvedValue({points:[point],observations:[point],outsideRange:false});
 api.periods.mockReset().mockResolvedValue({items:[item(7,"33.3333333333333333333333"),item(30,"-4.5"),item(365,"12")]});
 api.instruments.mockReset().mockResolvedValue([{id:"i",name:"Synthetic fund",type:"mutual_fund",quoteCurrency:"USD",countryCode:"US",symbol:"SYN",quoteSource:"agent"}]);
});
const mount=(props:Partial<React.ComponentProps<typeof HoldingGroupsList>>={},retry:number|false=1)=>{
 const client=createTestQueryClient({retry});clients.push(client);
 const ui=(extra:Partial<React.ComponentProps<typeof HoldingGroupsList>>={})=><QueryClientProvider client={client}><HoldingGroupsList scope="portfolio" groups={groups()} {...props} {...extra} /></QueryClientProvider>;
 const view=render(ui());
 return {client,view,rerender:(extra:Partial<React.ComponentProps<typeof HoldingGroupsList>>)=>view.rerender(ui(extra))};
};
const open=async(name="Show details for Synthetic fund")=>{
 await userEvent.click(screen.getByRole("button",{name}));
 await act(() => vi.dynamicImportSettled());
 return screen.findByRole("region",{name:/Synthetic fund|SYN/});
};

it("loads only expanded active details, shares history across accounts, and reuses cache after collapse",async()=>{
 const {rerender}=mount();
 expect(api.instruments).not.toHaveBeenCalled();expect(api.history).not.toHaveBeenCalled();expect(api.periods).not.toHaveBeenCalled();
 await userEvent.click(screen.getByRole("button",{name:"Account details"}));
 const buttons=screen.getAllByRole("button",{name:"Show details for Synthetic fund"});
 await userEvent.click(buttons[0]);
 expect(buttons[0]).toHaveAttribute("aria-expanded", "true");
 await act(() => vi.dynamicImportSettled());
 await waitFor(() => expect(api.instruments).toHaveBeenCalledWith(true));
 await waitFor(() => expect(api.periods).toHaveBeenCalledWith("i"));
 await screen.findByText("Oct 5 – 14, 2026");
 await userEvent.click(buttons[1]);await waitFor(()=>expect(screen.getAllByText("Oct 5 – 14, 2026")).toHaveLength(2));
 expect(api.history).toHaveBeenCalledTimes(1);expect(api.periods).toHaveBeenCalledTimes(1);expect(api.instruments).toHaveBeenCalledTimes(1);
 await userEvent.click(buttons[0]);await userEvent.click(buttons[0]);expect(api.history).toHaveBeenCalledTimes(1);
 rerender({active:false});
 expect(screen.queryByRole("region",{name:"Price performance"})).not.toBeInTheDocument();
 expect(api.history).toHaveBeenCalledTimes(1);
});

it("shows the selected period as a large signed headline with a quiet date range and no wordy labels",async()=>{
 mount();
 const detail=within(await open());
 const performance=await detail.findByRole("region",{name:"Price performance"});
 const scoped=within(performance);
 expect(await scoped.findByText("Oct 5 – 14, 2026")).toBeInTheDocument();
 // Default period is 1M (30 days), which fell 4.5%.
 expect(scoped.getByText("-4.50%")).toBeInTheDocument();
 expect(scoped.getByText("Down")).toHaveClass("sr-only");
 expect(performance.querySelector("svg.lucide-arrow-down-right")).not.toBeNull();
 for(const wordy of ["7-day target","Price / NAV change","Historical price / NAV performance","Simple annualized NAV change","Daily close"]) expect(performance).not.toHaveTextContent(wordy);
 await userEvent.click(scoped.getByRole("button",{name:"1W"}));
 expect(await scoped.findByText("+33.33%")).toBeInTheDocument();
 expect(scoped.getByText("Up")).toHaveClass("sr-only");
 expect(performance.querySelector("svg.lucide-arrow-up-right")).not.toBeNull();
 expect(scoped.getByRole("button",{name:"1W"})).toHaveAttribute("aria-pressed","true");
 expect(scoped.getAllByRole("button").filter(button=>button.hasAttribute("aria-pressed")).map(button=>button.textContent)).toEqual(["1W","1M","3M","6M","1Y","All"]);
 await waitFor(()=>expect(api.history).toHaveBeenLastCalledWith("i",expect.stringMatching(/^\d{4}-\d{2}-\d{2}:\d{4}-\d{2}-\d{2}$/),"all"));
 await userEvent.click(scoped.getByRole("button",{name:"All"}));
 await waitFor(()=>expect(api.history).toHaveBeenLastCalledWith("i","all","all"));
});

it("tucks the source filter behind a secondary control and forwards the choice to the chart",async()=>{
 mount();
 const performance=within(await within(await open()).findByRole("region",{name:"Price performance"}));
 expect(performance.queryByRole("button",{name:"Provider"})).not.toBeInTheDocument();
 await userEvent.click(performance.getByRole("button",{name:"Data source"}));
 await userEvent.click(await screen.findByRole("button",{name:"Provider"}));
 await waitFor(()=>expect(api.history).toHaveBeenLastCalledWith("i",expect.any(String),"provider"));
 expect(await performance.findByRole("button",{name:"Data source: Provider"})).toBeInTheDocument();
});

it("keeps the explanation in an info popover and the data table unobtrusive",async()=>{
 mount();
 const performance=within(await within(await open()).findByRole("region",{name:"Price performance"}));
 expect(performance.queryByText(/not your holding return|Intraday quotes are excluded/)).not.toBeInTheDocument();
 await userEvent.click(performance.getByRole("button",{name:"About this figure"}));
 expect(await screen.findByText(/not your holding return|so this is not your holding return/)).toBeInTheDocument();
 const summary=await performance.findByText("View data table");
 expect(summary).toHaveClass("text-xs");
});

it("shows a dash with a short reason when the period has no comparable evidence, while keeping the chart",async()=>{
 api.periods.mockResolvedValue({items:[item(7,"1")]});
 mount();
 const performance=within(await within(await open()).findByRole("region",{name:"Price performance"}));
 expect(await performance.findByText("Not enough history for this period")).toBeInTheDocument();
 expect(performance.queryByText(/%/)).not.toBeInTheDocument();
 expect(await performance.findByText("View data table")).toBeInTheDocument();
});

it.each([
 ["incomparable_daily_prices","Prices can’t be compared here","cannot be compared across this period"],
 ["unsupported_instrument","Not available for this type","unavailable for this instrument type"],
 ["insufficient_daily_history","Not enough history for this period","start of this period"],
 ["unknown_internal_reason","Not enough history for this period","start of this period"],
])("explains %s briefly and never leaks the internal code",async(reason,short,detail)=>{
 api.periods.mockResolvedValue({items:[],unavailableReason:reason});
 mount();
 const performance=within(await within(await open()).findByRole("region",{name:"Price performance"}));
 expect(await performance.findByText(short)).toBeInTheDocument();
 await userEvent.click(performance.getByRole("button",{name:"About this figure"}));
 expect(await screen.findByText(new RegExp(detail))).toBeInTheDocument();
 expect(document.body.textContent).not.toContain(reason);
});

it("shows precious-metal performance like any other priced instrument, with the per-unit price suffix",async()=>{
 const gold={...group,instrumentId:"au",type:"precious_metal",name:"Gold",symbol:"XAU",quantityUnit:"g",quoteCurrency:"CNY"};
 api.instruments.mockResolvedValue([{id:"au",name:"Gold",type:"precious_metal",quoteCurrency:"CNY",symbol:"XAU",quantityUnit:"g",metalTemplate:"gold",quoteSource:"yahoo"}]);
 api.periods.mockResolvedValue({items:[item(30,"6.25")]});
 mount({groups:groups(gold,"precious_metal")});
 await userEvent.click(screen.getByRole("button",{name:"Show details for Gold"}));
 await act(() => vi.dynamicImportSettled());
 const performance=within(await screen.findByRole("region",{name:"Price performance"}));
 expect(await performance.findByText("+6.25%")).toBeInTheDocument();
 expect(api.periods).toHaveBeenCalledWith("au");
 await userEvent.click(performance.getByRole("button",{name:"About this figure"}));
 expect(await screen.findByText(/Futures reference valuation/)).toBeInTheDocument();
});

it("shows annualized change only as a small secondary figure",async()=>{
 api.periods.mockResolvedValue({items:[item(30,"0",{annualizedPercent:"0"})]});
 api.instruments.mockResolvedValue([{id:"i",name:"Synthetic fund",type:"bank_investment_product",quoteCurrency:"USD",countryCode:"US",symbol:"SYN",note:"Long synthetic note ".repeat(50)}]);
 mount();
 const detail=within(await open());
 const performance=within(await detail.findByRole("region",{name:"Price performance"}));
 expect(await performance.findByText("· 0.00% annualized")).toBeInTheDocument();
 expect(performance.getByText("Unchanged")).toHaveClass("sr-only");
 const region=screen.getByRole("region",{name:/SYN · All included accounts/});
 // The row already shows cost, value and gain, so the panel starts with performance and ends with facts.
 const content=region.textContent!;
 expect(content).not.toContain("Cost");
 expect(content.indexOf("Oct 5")).toBeLessThan(content.indexOf("Country"));
 const notes=region.querySelector("details:not(figure details)") as HTMLDetailsElement;
 expect(notes.textContent).toContain("Note");
 expect(notes).not.toHaveAttribute("open");
 await userEvent.click(within(notes).getByText("Note"));
 expect(notes).toHaveAttribute("open");
});

it("offers a friendly empty state when no prices are stored and a way out when only other periods have data",async()=>{
 api.history.mockResolvedValueOnce({points:[],observations:[],outsideRange:false});
 mount();
 const performance=within(await within(await open()).findByRole("region",{name:"Price performance"}));
 expect(await performance.findByText("No price history yet")).toBeInTheDocument();
 expect(performance.getByText("Prices appear here once they are recorded or synced.")).toBeInTheDocument();
});

it("lets the household widen the range when prices exist only outside the period",async()=>{
 api.history.mockImplementation((_id:string,range:string)=>Promise.resolve(range==="all"?{points:[point],observations:[point],outsideRange:false}:{points:[],observations:[],outsideRange:true}));
 mount();
 const performance=within(await within(await open()).findByRole("region",{name:"Price performance"}));
 expect(await performance.findByText("No prices in this period")).toBeInTheDocument();
 await userEvent.click(performance.getByRole("button",{name:"Show all history"}));
 expect(await performance.findByText("View data table")).toBeInTheDocument();
 expect(performance.getByRole("button",{name:"All"})).toHaveAttribute("aria-pressed","true");
});

it("shows the loading skeleton until the chart data arrives, then a retryable error",async()=>{
 let reject!:(error:Error)=>void;
 api.history.mockImplementationOnce(()=>new Promise((_,fail)=>{reject=fail;}));
 mount({},false);
 const performance=within(await within(await open()).findByRole("region",{name:"Price performance"}));
 expect(performance.getByRole("status")).toHaveTextContent("Loading price history");
 await act(async()=>reject(new Error("boom")));
 expect(await performance.findByRole("alert")).toBeInTheDocument();
 await userEvent.click(performance.getByRole("button",{name:"Try again"}));
 expect(await performance.findByText("View data table")).toBeInTheDocument();
});
