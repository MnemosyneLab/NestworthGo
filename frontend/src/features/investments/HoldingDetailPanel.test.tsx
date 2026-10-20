import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeAll, beforeEach, expect, it, vi } from "vitest";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { HoldingGroupsList } from "./HoldingGroupsList";
import { useHoldingGroupStore } from "@/stores/holdings";
import type { InstrumentHoldingGroupDTO, InstrumentHoldingsDTO } from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics/models";
const api=vi.hoisted(()=>({series:vi.fn(),returns:vi.fn(),instruments:vi.fn()}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/instrument",()=>({Service:{ListInstruments:api.instruments}}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/quote",()=>({Service:{InstrumentQuotePerformanceSeries:api.series,InstrumentQuoteReturns:api.returns}}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/history",()=>({Service:{HistoryOrigin:()=>Promise.resolve(null)}}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings",()=>({Service:{Load:()=>Promise.resolve({timezone:"UTC"})}}));
const amounts={quantity:"2",totalCost:{amount:"100.00012",currency:"USD"},currentValue:{amount:"110",currency:"USD"},unrealizedGain:{amount:"10",currency:"USD"}};
const group={instrumentId:"i",type:"mutual_fund",name:"Synthetic fund",symbol:"SYN",quoteCurrency:"USD",quantityUnit:"",archived:false,amounts,holdings:[{holdingId:"h1",accountId:"a",accountName:"Account A",amounts},{holdingId:"h2",accountId:"b",accountName:"Account B",amounts:{...amounts,totalCost:{amount:"90",currency:"USD"}}}]} as InstrumentHoldingsDTO;
const groups=(item=group,key="fund"):InstrumentHoldingGroupDTO[]=>[{key,instruments:[item],subtotals:[]}];
const item=(windowDays:number,periodPercent:string,extra:Record<string,string>={})=>({windowDays,baselineOn:"2026-10-05",latestOn:"2026-10-14",elapsedDays:9,periodPercent,...extra});
const point={quotedAt:"2026-10-14T00:00:00Z",effectiveDate:"2026-10-14",timestampBasis:"date_label",value:"1.5",currency:"USD",sourceKind:"agent",sourceKey:"",priceBasis:"unit_nav",observationKind:"close",delayed:false,breakBefore:false};
const earlier={...point,quotedAt:"2026-10-05T00:00:00Z",effectiveDate:"2026-10-05",value:"1.4"};
const seriesAnswer=(windowDays:number,extra:Record<string,unknown>={})=>{
 const found=defaultItems.find(entry=>entry.windowDays===windowDays);
 return {windowDays,from:"2026-10-05",to:"2026-10-14",item:found??null,unavailableReason:found?"":"insufficient_daily_history",points:[earlier,point],...extra};
};
const defaultItems=[item(7,"33.3333333333333333333333"),item(30,"-4.5"),item(365,"12")];
// Compile the real detail/chart module before timing UI interactions. Rendering
// still goes through HoldingGroupsList -> React.lazy -> Suspense, and loading
// code alone must never initiate a service query.
beforeAll(async () => {
 await import("./HoldingDetailPanel");
 expect(api.instruments).not.toHaveBeenCalled();
 expect(api.series).not.toHaveBeenCalled();
 expect(api.returns).not.toHaveBeenCalled();
});
const clients: ReturnType<typeof createTestQueryClient>[] = [];
afterEach(() => {
 for (const client of clients.splice(0)) client.clear();
});
beforeEach(()=>{
 useHoldingGroupStore.getState().reset();
 api.series.mockReset().mockImplementation((_id:string,days:number)=>Promise.resolve(seriesAnswer(days)));
 api.returns.mockReset().mockResolvedValue({items:defaultItems});
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

it("loads only expanded active details, shares performance across accounts, and reuses cache after collapse",async()=>{
 const {rerender}=mount();
 expect(api.instruments).not.toHaveBeenCalled();expect(api.series).not.toHaveBeenCalled();expect(api.returns).not.toHaveBeenCalled();
 await userEvent.click(screen.getByRole("button",{name:"Account details"}));
 const buttons=screen.getAllByRole("button",{name:"Show details for Synthetic fund"});
 await userEvent.click(buttons[0]);
 expect(buttons[0]).toHaveAttribute("aria-expanded", "true");
 await act(() => vi.dynamicImportSettled());
 await waitFor(() => expect(api.instruments).toHaveBeenCalledWith(true));
 await waitFor(() => expect(api.returns).toHaveBeenCalledWith("i"));
 await waitFor(() => expect(api.series).toHaveBeenCalledWith("i",30));
 await screen.findAllByText("2026-10-05 – 2026-10-14");
 await userEvent.click(buttons[1]);await waitFor(()=>expect(screen.getAllByRole("region",{name:"Price performance"})).toHaveLength(2));
 expect(api.series).toHaveBeenCalledTimes(1);expect(api.returns).toHaveBeenCalledTimes(1);expect(api.instruments).toHaveBeenCalledTimes(1);
 await userEvent.click(buttons[0]);await userEvent.click(buttons[0]);expect(api.series).toHaveBeenCalledTimes(1);
 rerender({active:false});
 expect(screen.queryByRole("region",{name:"Price performance"})).not.toBeInTheDocument();
 expect(api.series).toHaveBeenCalledTimes(1);
});

it("shows the selected period as a large signed headline with the calculation dates and no wordy labels",async()=>{
 mount();
 const detail=within(await open());
 const performance=await detail.findByRole("region",{name:"Price performance"});
 const scoped=within(performance);
 expect(await scoped.findByText("2026-10-05 – 2026-10-14")).toBeInTheDocument();
 // Default period is 1M (30 days), which fell 4.5%.
 expect(scoped.getByText("-4.50%")).toBeInTheDocument();
 expect(scoped.getByText("Down")).toHaveClass("sr-only");
 expect(performance.querySelector("svg.lucide-arrow-down-right")).not.toBeNull();
 for(const wordy of ["7-day target","Price / NAV change","Historical price / NAV performance","Simple annualized NAV change"]) expect(performance).not.toHaveTextContent(wordy);
 await userEvent.click(scoped.getByRole("button",{name:"1W"}));
 expect(await scoped.findByText("+33.33%")).toBeInTheDocument();
 expect(scoped.getByText("Up")).toHaveClass("sr-only");
 expect(performance.querySelector("svg.lucide-arrow-up-right")).not.toBeNull();
 expect(scoped.getByRole("button",{name:"1W"})).toHaveAttribute("aria-pressed","true");
 expect(scoped.getAllByRole("button").filter(button=>button.hasAttribute("aria-pressed")).map(button=>button.textContent)).toEqual(["1W","1M","3M","6M","1Y","All"]);
 await waitFor(()=>expect(api.series).toHaveBeenLastCalledWith("i",7));
 await userEvent.click(scoped.getByRole("button",{name:"All"}));
 await waitFor(()=>expect(api.series).toHaveBeenLastCalledWith("i",0));
});

it("takes the headline and the chart from one daily response so both cover the same interval",async()=>{
 api.series.mockResolvedValue(seriesAnswer(30,{item:item(30,"7.14",{baselineOn:"2026-10-05",latestOn:"2026-10-14"}),points:[earlier,point]}));
 mount();
 const performance=within(await within(await open()).findByRole("region",{name:"Price performance"}));
 expect(await performance.findByText("2026-10-05 – 2026-10-14")).toBeInTheDocument();
 // The accessible data table lists the same daily points the headline interval spans.
 const rows=within(await performance.findByRole("table")).getAllByRole("row").slice(1);
 expect(rows.map(row=>within(row).getAllByRole("cell")[3].textContent)).toEqual(["2026-10-14","2026-10-05"]);
});

it("has no source filter because the chart only shows eligible daily closes",async()=>{
 mount();
 const performance=within(await within(await open()).findByRole("region",{name:"Price performance"}));
 await performance.findByText("2026-10-05 – 2026-10-14");
 expect(performance.queryByRole("button",{name:/Data source/})).not.toBeInTheDocument();
});

it("keeps the explanation in an info popover and the data table unobtrusive",async()=>{
 mount();
 const performance=within(await within(await open()).findByRole("region",{name:"Price performance"}));
 expect(performance.queryByText(/not your holding return|Intraday quotes are excluded/)).not.toBeInTheDocument();
 await userEvent.click(performance.getByRole("button",{name:"About this figure"}));
 expect(await screen.findByText(/not your holding return/)).toBeInTheDocument();
 const summary=await performance.findByText("View data table");
 expect(summary).toHaveClass("text-xs");
});

it("shows a dash with a short reason when the period has no comparable evidence, while keeping the chart",async()=>{
 mount();
 await userEvent.click(screen.getByRole("button",{name:"Show details for Synthetic fund"}));
 await act(() => vi.dynamicImportSettled());
 const performance=within(await screen.findByRole("region",{name:"Price performance"}));
 await userEvent.click(await performance.findByRole("button",{name:"3M"}));
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
 api.series.mockResolvedValue(seriesAnswer(30,{item:null,unavailableReason:reason}));
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
 api.series.mockResolvedValue(seriesAnswer(30,{item:item(30,"6.25")}));
 mount({groups:groups(gold,"precious_metal")});
 await userEvent.click(screen.getByRole("button",{name:"Show details for Gold"}));
 await act(() => vi.dynamicImportSettled());
 const performance=within(await screen.findByRole("region",{name:"Price performance"}));
 expect(await performance.findByText("+6.25%")).toBeInTheDocument();
 expect(api.series).toHaveBeenCalledWith("au",30);
 expect(api.returns).toHaveBeenCalledWith("au");
 await userEvent.click(performance.getByRole("button",{name:"About this figure"}));
 expect(await screen.findByText(/Futures reference valuation/)).toBeInTheDocument();
});

it("lists available fixed-window returns in a separate section with their real dates and hides the rest",async()=>{
 api.returns.mockResolvedValue({items:[item(7,"1.5",{baselineOn:"2026-10-07",latestOn:"2026-10-14"}),item(90,"-2",{baselineOn:"2026-07-16",latestOn:"2026-10-14"}),item(365,"12",{baselineOn:"2025-10-14",latestOn:"2026-10-14"})]});
 mount();
 const detail=within(await open());
 const returns=within(await detail.findByRole("region",{name:"Returns by period"}));
 const entries=returns.getAllByRole("listitem");
 expect(entries.map(entry=>within(entry).getByText(/-day$/).textContent)).toEqual(["7-day","90-day","365-day"]);
 expect(entries[0]).toHaveTextContent("+1.50%");
 expect(entries[0]).toHaveTextContent("2026-10-07 – 2026-10-14");
 expect(entries[1]).toHaveTextContent("-2.00%");
 expect(entries[1]).toHaveTextContent("2026-07-16 – 2026-10-14");
 expect(returns.queryByText("30-day")).not.toBeInTheDocument();
 expect(returns.queryByText("180-day")).not.toBeInTheDocument();
 expect(returns.queryByText(/All|all history/i)).not.toBeInTheDocument();
 // It is a sibling section, not part of the chart panel.
 expect(within(await detail.findByRole("region",{name:"Price performance"})).queryByText("365-day")).not.toBeInTheDocument();
});

it("never shows an all-history window in the returns section, even if one slips through",async()=>{
 api.returns.mockResolvedValue({items:[item(0,"50"),item(30,"3")]});
 mount();
 const returns=within(await within(await open()).findByRole("region",{name:"Returns by period"}));
 expect(returns.getAllByRole("listitem")).toHaveLength(1);
 expect(returns.queryByText("+50.00%")).not.toBeInTheDocument();
});

it("hides the returns section without sufficient history",async()=>{
 api.returns.mockResolvedValue({items:[],unavailableReason:"insufficient_daily_history"});
 mount();
 const detail=within(await open());
 await detail.findByRole("region",{name:"Price performance"});
 await waitFor(()=>expect(api.returns).toHaveBeenCalled());
 expect(detail.queryByRole("region",{name:"Returns by period"})).not.toBeInTheDocument();
});

it("shows annualized change only for the window that has one, as a small secondary figure in the returns section",async()=>{
 api.returns.mockResolvedValue({items:[item(30,"0",{annualizedPercent:"0"}),item(90,"1")]});
 api.instruments.mockResolvedValue([{id:"i",name:"Synthetic fund",type:"bank_investment_product",quoteCurrency:"USD",countryCode:"US",symbol:"SYN",note:"Long synthetic note ".repeat(50)}]);
 mount();
 const detail=within(await open());
 const returns=within(await detail.findByRole("region",{name:"Returns by period"}));
 expect(await returns.findByText("0.00% annualized")).toBeInTheDocument();
 expect(returns.getAllByText(/annualized/)).toHaveLength(1);
 expect(returns.getByText("Unchanged")).toHaveClass("sr-only");
 const performance=within(await detail.findByRole("region",{name:"Price performance"}));
 expect(performance.queryByText(/annualized/)).not.toBeInTheDocument();
 const region=screen.getByRole("region",{name:/SYN · All included accounts/});
 // The row already shows cost, value and gain, so the panel starts with performance and ends with facts.
 const content=region.textContent!;
 expect(content).not.toContain("Cost");
 expect(content.indexOf("2026-10-05")).toBeLessThan(content.indexOf("Country"));
 const notes=region.querySelector("details:not(figure details)") as HTMLDetailsElement;
 expect(notes.textContent).toContain("Note");
 expect(notes).not.toHaveAttribute("open");
 await userEvent.click(within(notes).getByText("Note"));
 expect(notes).toHaveAttribute("open");
});

it("offers a friendly empty state when no daily prices are stored",async()=>{
 api.series.mockResolvedValue(seriesAnswer(30,{item:null,points:[]}));
 mount();
 const performance=within(await within(await open()).findByRole("region",{name:"Price performance"}));
 expect(await performance.findByText("No daily price history yet")).toBeInTheDocument();
 expect(performance.getByText("Record or sync daily closing prices. Timestamped manual prices are available in Market Data.")).toBeInTheDocument();
});

it("shows the loading skeleton until the performance arrives, then a retryable error",async()=>{
 let reject!:(error:Error)=>void;
 api.series.mockImplementationOnce(()=>new Promise((_,fail)=>{reject=fail;}));
 mount({},false);
 const performance=within(await within(await open()).findByRole("region",{name:"Price performance"}));
 expect(performance.getByRole("status")).toHaveTextContent("Loading price history");
 await act(async()=>reject(new Error("boom")));
 expect(await performance.findByRole("alert")).toBeInTheDocument();
 await userEvent.click(performance.getByRole("button",{name:"Try again"}));
 expect(await performance.findByText("View data table")).toBeInTheDocument();
});

function pendingDetailRead<T>() {
 let resolve!:(value:T)=>void;
 let reject!:(error:Error)=>void;
 const cancel=vi.fn();
 const promise=Object.assign(new Promise<T>((yes,no)=>{resolve=yes;reject=no;}),{cancel});
 return {promise,resolve,reject,cancel};
}

it.each(["portfolio","account"] as const)("handles expansion, collapse and switching instruments without hiding live errors (%s)",async(scope)=>{
 const seriesReads:Array<ReturnType<typeof pendingDetailRead<ReturnType<typeof seriesAnswer>>>>=[];
 const returnReads:Array<ReturnType<typeof pendingDetailRead<{items:ReturnType<typeof item>[]}>>>=[];
 api.series.mockImplementation(()=>{const read=pendingDetailRead<ReturnType<typeof seriesAnswer>>();seriesReads.push(read);return read.promise;});
 api.returns.mockImplementation(()=>{const read=pendingDetailRead<{items:ReturnType<typeof item>[]}>();returnReads.push(read);return read.promise;});
 const second={...group,instrumentId:"second",name:"Second synthetic fund",symbol:"SECOND",holdings:[{...group.holdings![0],holdingId:"second-holding"}]};
 const first={...group,holdings:[group.holdings![0]]};
 api.instruments.mockResolvedValue([first,second].map(entry=>({id:entry.instrumentId,name:entry.name,type:"bank_investment_product",quoteCurrency:"USD",countryCode:"US",symbol:entry.symbol,quoteSource:"agent"})));
 mount({scope,accountId:"a",accountName:"Account A",groups:[{key:"bank_wealth",instruments:[first,second],subtotals:[]}]},false);

 // Initial expansion mounts the shared detail, chart and return hooks.
 await open();
 await waitFor(()=>expect(seriesReads).toHaveLength(1));
 expect(returnReads).toHaveLength(1);
 expect(screen.getByRole("status")).toHaveTextContent("Loading price history");
 await userEvent.click(screen.getByRole("button",{name:"Hide details for Synthetic fund"}));
 expect(seriesReads[0].cancel).toHaveBeenCalledOnce();
 expect(returnReads[0].cancel).toHaveBeenCalledOnce();
 expect(screen.queryByRole("region",{name:"Price performance"})).not.toBeInTheDocument();

 await userEvent.click(screen.getByRole("button",{name:"Show details for Second synthetic fund"}));
 await act(() => vi.dynamicImportSettled());
 await screen.findByRole("region",{name:/SECOND/});
 await waitFor(()=>expect(seriesReads).toHaveLength(2));
 expect(returnReads).toHaveLength(2);
 expect(api.series).toHaveBeenLastCalledWith("second",30);
 const performance=within(screen.getByRole("region",{name:"Price performance"}));
 await userEvent.click(performance.getByRole("button",{name:"1W"}));
 await waitFor(()=>expect(seriesReads).toHaveLength(3));
 expect(seriesReads[1].cancel).toHaveBeenCalledOnce();
 expect(api.series).toHaveBeenLastCalledWith("second",7);
 await act(async()=>{
  // Abandoned failures must not become the active row's error.
  seriesReads[0].reject(new Error('{"code":"internal","message":"an unexpected error occurred"}'));
  returnReads[0].reject(new Error('{"code":"internal","message":"an unexpected error occurred"}'));
  seriesReads[1].resolve(seriesAnswer(30));
  seriesReads[2].resolve(seriesAnswer(7,{item:item(7,"1.5")}));
  returnReads[1].resolve({items:[item(7,"1.5",{annualizedPercent:"12.34"})]});
 });
 expect(await performance.findByText("+1.50%")).toBeInTheDocument();
 expect(performance.getByRole("table")).toBeInTheDocument();
 const returns=within(await screen.findByRole("region",{name:"Returns by period"}));
 expect(returns.getByText("7-day")).toBeInTheDocument();
 expect(returns.getByText("12.34% annualized")).toBeInTheDocument();
 expect(screen.queryByRole("alert")).not.toBeInTheDocument();
 await userEvent.click(screen.getByRole("button",{name:"Hide details for Second synthetic fund"}));
 expect(seriesReads[2].cancel).not.toHaveBeenCalled();
 expect(returnReads[1].cancel).not.toHaveBeenCalled();

 // On returning, live chart and return errors stay visible and retryable.
 await open();
 await waitFor(()=>expect(seriesReads).toHaveLength(4));
 expect(returnReads).toHaveLength(3);
 await act(async()=>{
  seriesReads[3].reject(new Error('{"code":"internal","message":"an unexpected error occurred"}'));
  returnReads[2].reject(new Error('{"code":"internal","message":"an unexpected error occurred"}'));
 });
 const livePerformance=within(screen.getByRole("region",{name:"Price performance"}));
 expect(await livePerformance.findByRole("alert")).toBeInTheDocument();
 const liveReturns=within(screen.getByRole("region",{name:"Returns by period"}));
 expect(liveReturns.getByText("Couldn’t load returns")).toBeInTheDocument();
 await userEvent.click(livePerformance.getByRole("button",{name:"Try again"}));
 await userEvent.click(liveReturns.getByRole("button",{name:"Try again"}));
 await waitFor(()=>expect(seriesReads).toHaveLength(5));
 expect(returnReads).toHaveLength(4);
 await act(async()=>{
  seriesReads[4].resolve(seriesAnswer(30));
  returnReads[3].resolve({items:[item(7,"1.5",{annualizedPercent:"12.34"})]});
 });
 expect(await livePerformance.findByRole("table")).toBeInTheDocument();
 expect(await screen.findByText("12.34% annualized")).toBeInTheDocument();
 expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});


it("retains three daily chart points when the monthly return lacks a baseline", async () => {
 const points=[earlier,{...point,effectiveDate:"2026-10-12",quotedAt:"2026-10-12T00:00:00Z",value:"1.45"},point];
 api.series.mockResolvedValue(seriesAnswer(30,{item:null,unavailableReason:"insufficient_daily_history",points}));
 mount();
 const performance=within(await within(await open()).findByRole("region",{name:"Price performance"}));
 expect(await performance.findByText("Not enough history for this period")).toBeInTheDocument();
 const rows=within(await performance.findByRole("table")).getAllByRole("row").slice(1);
 expect(rows).toHaveLength(3);
 expect(performance.queryByText("No daily price history yet")).not.toBeInTheDocument();
});
