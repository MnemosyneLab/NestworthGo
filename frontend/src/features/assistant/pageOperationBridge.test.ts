import { beforeEach, expect, it, vi } from "vitest";
import { pollPageOperations } from "./pageOperationBridge";
const dispatch=vi.hoisted(()=>vi.fn());
vi.mock("./pageOperations",()=>({operatePageControl:dispatch}));
const command={snapshotId:"snapshot",targetRef:"target",action:"activate",expiresAt:"2099-01-01T00:00:00Z"};
const current=()=>({viewKey:"synthetic:",generation:"generation",fullAccess:true,blocked:()=>false});
const api={pending:vi.fn(),claim:vi.fn(),deliver:vi.fn()};
beforeEach(()=>{vi.clearAllMocks();api.pending.mockResolvedValue([{id:"once"}]);api.claim.mockResolvedValue(command);api.deliver.mockResolvedValue(undefined);dispatch.mockReset().mockReturnValue({state:"dispatched",snapshotId:"snapshot"});});
it("never replays after failed delivery and duplicate polling",async()=>{
 api.claim.mockResolvedValueOnce(command).mockRejectedValue(new Error("already claimed"));api.deliver.mockRejectedValue(new Error("response lost"));
 await expect(pollPageOperations(api,current,()=>false)).rejects.toThrow("response lost");
 await expect(pollPageOperations(api,current,()=>false)).rejects.toThrow("already claimed");expect(dispatch).toHaveBeenCalledOnce();
});
it("does not dispatch a canceled claim",async()=>{
 api.claim.mockRejectedValue(new Error("canceled"));await expect(pollPageOperations(api,current,()=>false)).rejects.toThrow("canceled");expect(dispatch).not.toHaveBeenCalled();
});
it("does not dispatch late claim after host stops",async()=>{
 let resolve!:(value:typeof command)=>void;let stopped=false;api.claim.mockImplementation(()=>new Promise(r=>{resolve=r;}));
 const pending=pollPageOperations(api,current,()=>stopped);await vi.waitFor(()=>expect(api.claim).toHaveBeenCalledOnce());stopped=true;resolve(command);await pending;expect(dispatch).not.toHaveBeenCalled();expect(api.deliver).toHaveBeenCalledWith("once",{state:"not_dispatched",snapshotId:"snapshot"});
});
it("rechecks current host authority after awaiting claim",async()=>{
 let resolve!:(value:typeof command)=>void;let fullAccess=true;api.claim.mockImplementation(()=>new Promise(r=>{resolve=r;}));
 const pending=pollPageOperations(api,()=>({...current(),fullAccess}),()=>false);await vi.waitFor(()=>expect(api.claim).toHaveBeenCalledOnce());fullAccess=false;resolve(command);await pending;expect(dispatch.mock.calls[0][3]).toBe(false);
});
it("reports unverified for handler exception",async()=>{
 dispatch.mockImplementation(()=>{throw new Error("handler failed");});await pollPageOperations(api,current,()=>false);expect(api.deliver).toHaveBeenCalledWith("once",{state:"unverified",snapshotId:"snapshot"});expect(dispatch).toHaveBeenCalledOnce();
});
it("does not dispatch expired claim",async()=>{
 api.claim.mockResolvedValue({...command,expiresAt:"2000-01-01T00:00:00Z"});await pollPageOperations(api,current,()=>false);expect(dispatch).not.toHaveBeenCalled();expect(api.deliver).toHaveBeenCalledWith("once",{state:"not_dispatched",snapshotId:"snapshot"});
});

it("does not replay a skipped claim when its acknowledgement is lost",async()=>{
 api.claim.mockResolvedValueOnce({...command,expiresAt:"2000-01-01T00:00:00Z"}).mockRejectedValue(new Error("already claimed"));api.deliver.mockRejectedValue(new Error("response lost"));
 await expect(pollPageOperations(api,current,()=>false)).rejects.toThrow("response lost");
 await expect(pollPageOperations(api,current,()=>false)).rejects.toThrow("already claimed");
 expect(dispatch).not.toHaveBeenCalled();expect(api.deliver).toHaveBeenCalledOnce();
});
