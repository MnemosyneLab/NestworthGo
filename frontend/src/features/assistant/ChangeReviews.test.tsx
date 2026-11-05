import { beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { ChangeReviews } from "./ChangeReviews";
const api=vi.hoisted(()=>({Reviews:vi.fn(),ReviewAction:vi.fn()}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service",()=>api);
const review={id:"host-review",digest:"frozen-digest",state:"awaiting_confirmation",kind:"money_added",account:"Synthetic cash",amount:"12.3456789",currency:"USD",resultingAmount:"22.3456789",effectiveAt:"2026-09-28T12:00:00Z",reason:"income",note:"Synthetic note"};
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
