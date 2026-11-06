import { beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { ChangeReviews } from "./ChangeReviews";
const api=vi.hoisted(()=>({Reviews:vi.fn(),ReviewAction:vi.fn()}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service",()=>api);
const review={id:"host-review",digest:"frozen-digest",state:"awaiting_confirmation",kind:"money_added",account:"Synthetic cash",accountInstitution:"Synthetic bank A",accountOwners:["Alice"],accountType:"bank_account",accountCurrency:"USD",amount:"12.3456789",currency:"USD",resultingAmount:"22.3456789",effectiveAt:"2026-09-28T12:00:00Z",reason:"income",note:"Synthetic note"};
beforeEach(()=>{vi.clearAllMocks();api.Reviews.mockResolvedValue([review]);api.ReviewAction.mockResolvedValue(undefined);});
function mount(enabled=true){const client=createTestQueryClient();return render(<QueryClientProvider client={client}><ChangeReviews epoch="epoch" id="conversation" enabled={enabled} busy={false}/></QueryClientProvider>);}
it("renders the exact frozen amounts and confirms only on an explicit keyboard action",async()=>{
 const user=userEvent.setup();mount();
 const confirm=await screen.findByRole("button",{name:"Confirm this ledger change"});
 expect(screen.getByText("12.3456789 USD")).toBeVisible();
 expect(screen.getByText("22.3456789 USD")).toBeVisible();
 expect(api.ReviewAction).not.toHaveBeenCalled();
 confirm.focus();await user.keyboard("{Enter}");
 await waitFor(()=>expect(api.ReviewAction).toHaveBeenCalledExactlyOnceWith("epoch","conversation","host-review","frozen-digest","confirm"));
});
it("a lost response inspects the original review and never replays or cancels an unknown operation",async()=>{
 api.ReviewAction.mockImplementation(async()=>{api.Reviews.mockResolvedValue([{...review,state:"unknown"}]);throw new Error("lost reply");});
 mount();const confirm=await screen.findByRole("button",{name:"Confirm this ledger change"});
 fireEvent.click(confirm);fireEvent.click(confirm);
 await screen.findByText(/Outcome unknown/);
 expect(api.ReviewAction).toHaveBeenCalledTimes(1);
 expect(screen.queryByRole("button",{name:"Confirm this ledger change"})).toBeNull();
 expect(screen.queryByRole("button",{name:"Cancel unsubmitted change"})).toBeNull();
 fireEvent.click(screen.getByRole("button",{name:"Check original result"}));
 await waitFor(()=>expect(api.Reviews.mock.calls.length).toBeGreaterThan(2));
 expect(api.ReviewAction).toHaveBeenCalledTimes(1);
});
it("revoked permission disables confirmation but keeps inspect and cancel available",async()=>{
 mount(false);
 expect(await screen.findByRole("button",{name:"Confirm this ledger change"})).toBeDisabled();
 expect(screen.getByRole("button",{name:"Cancel unsubmitted change"})).toBeEnabled();
 expect(screen.getByRole("button",{name:"Check original result"})).toBeEnabled();
});
it("conflicts require a new preview and never auto-confirm it",async()=>{
 api.Reviews.mockResolvedValue([{...review,state:"needs_preview"}]);
 api.ReviewAction.mockImplementation(async()=>{api.Reviews.mockResolvedValue([{...review,id:"new-review",digest:"new-digest"}]);});
 const user=userEvent.setup();mount();
 await user.click(await screen.findByRole("button",{name:"Refresh preview for review"}));
 await screen.findByRole("button",{name:"Confirm this ledger change"});
 expect(api.ReviewAction).toHaveBeenCalledExactlyOnceWith("epoch","conversation","host-review","frozen-digest","repreview");
});

it("shows distinct frozen identities for same-name accounts and submits the chosen digest",async()=>{
 api.Reviews.mockResolvedValue([review,{...review,id:"other-review",digest:"other-digest",accountInstitution:"Synthetic bank B",accountOwners:["Bob"]}]);
 const user=userEvent.setup();const view=mount();
 await screen.findByText("Synthetic bank B");
 const cards=view.container.querySelectorAll("article");
 expect(cards).toHaveLength(2);
 expect(within(cards[0] as HTMLElement).getByText("Alice")).toBeVisible();
 expect(within(cards[1] as HTMLElement).getByText("Bob")).toBeVisible();
 expect(view.container.textContent).not.toContain("other-review");
 expect(view.container.textContent).not.toContain("other-digest");
 await user.click(within(cards[1] as HTMLElement).getByRole("button",{name:"Confirm this ledger change"}));
 expect(api.ReviewAction).toHaveBeenCalledExactlyOnceWith("epoch","conversation","other-review","other-digest","confirm");
});
it("never enables confirmation of a legacy preview missing frozen identity",async()=>{
 api.Reviews.mockResolvedValue([{...review,accountType:"",accountOwners:[]}]);mount();
 expect(await screen.findByRole("button",{name:"Confirm this ledger change"})).toBeDisabled();
 expect(screen.getByText(/Account identity was not captured/)).toBeVisible();
});

it.each(["No matching institution","没有对应机构","沒有對應機構","Not assigned"])("distinguishes an unassigned institution from a real institution named %s",async(name)=>{
 api.Reviews.mockResolvedValue([{...review,accountInstitution:""},{...review,id:"assigned-review",digest:"assigned-digest",accountInstitution:name}]);
 const user=userEvent.setup();const view=mount();
 await screen.findByText("Assigned");
 const cards=view.container.querySelectorAll("article");
 expect(within(cards[0] as HTMLElement).getByText("Not assigned")).toBeVisible();
 expect(within(cards[0] as HTMLElement).queryByText("Assigned")).toBeNull();
 expect(within(cards[1] as HTMLElement).getByText("Assigned")).toBeVisible();
 expect(within(cards[1] as HTMLElement).getByText(name)).toBeVisible();
 expect(cards[0].textContent).not.toBe(cards[1].textContent);
 await user.click(within(cards[1] as HTMLElement).getByRole("button",{name:"Confirm this ledger change"}));
 expect(api.ReviewAction).toHaveBeenCalledExactlyOnceWith("epoch","conversation","assigned-review","assigned-digest","confirm");
});

it.each(["awaiting_confirmation","needs_preview","unknown","succeeded"])("Full %s is never a human confirmation or automatic replay",async(state)=>{
 api.Reviews.mockResolvedValue([{...review,source:"ai_full",state}]);mount();
 await screen.findByText("Full access · cash and transfers");
 expect(screen.queryByText(/Only Confirm below/)).toBeNull();
 expect(screen.queryByRole("button",{name:"Confirm this ledger change"})).toBeNull();
 expect(screen.queryByRole("button",{name:"Refresh preview for review"})).toBeNull();
 fireEvent.click(screen.getByRole("button",{name:"Check original result"}));
 await waitFor(()=>expect(api.Reviews.mock.calls.length).toBeGreaterThan(1));
 expect(api.ReviewAction).not.toHaveBeenCalled();
});
