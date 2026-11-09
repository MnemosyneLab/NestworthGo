import { BusinessPageRoot } from "@/lib/businessCapture";
import { capturePageContent } from "@/features/assistant/pageContent";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { AgentSection } from "./AgentSection";

const { status, enable, disable, connection, operations } = vi.hoisted(() => ({ status: vi.fn(), enable: vi.fn(), disable: vi.fn(), connection: vi.fn(), operations: vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/agent", () => ({ Service: { Status: status, Enable: enable, Disable: disable, Connection: connection, RecentOperations: operations } }));
const off = { running: false, mode: "read_only", endpoint: "", instanceId: "test" };
const on = { ...off, running: true, endpoint: "http://127.0.0.1:1234/mcp" };
function mount() { const client = createTestQueryClient({ retry: false }); return render(<QueryClientProvider client={client}><BusinessPageRoot view="settings:" accessIdentity="synthetic"><AgentSection /></BusinessPageRoot></QueryClientProvider>); }
beforeEach(() => {
  vi.clearAllMocks(); status.mockResolvedValue(off); operations.mockResolvedValue([]);
  enable.mockImplementation(async (mode) => { status.mockResolvedValue({ ...on, mode }); return { ...on, mode }; });
  disable.mockImplementation(async () => { status.mockResolvedValue(off); return off; });
  connection.mockResolvedValue({ json: '{"token":"test-secret"}' });
});
describe("AgentSection", () => {
  it("defaults to read only and keeps credentials hidden until requested", async () => {
    const user = userEvent.setup(); mount();
    await user.click(await screen.findByRole("button", { name: "Enable MCP" }));
    expect(enable).toHaveBeenCalledWith("read_only");
    await screen.findByText("MCP is running on this computer");
    expect(connection).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Show connection configuration" }));
    expect(await screen.findByRole("textbox", { name: "MCP connection configuration" })).toHaveValue('{"token":"test-secret"}');
    expect(JSON.stringify(capturePageContent("settings:"))).not.toContain("test-secret");
    await user.click(screen.getByRole("button", { name: "Disable and revoke access" }));
    await screen.findByText("MCP is off");
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });
  it("offers only read and write permission choices and defaults to read only", async () => {
    const user = userEvent.setup(); mount();
    await screen.findByRole("button", { name: "Enable MCP" });
    expect(screen.getByLabelText("Agent permissions")).toHaveValue("read_only");
    expect(screen.getAllByRole("option").map((option) => option.textContent)).toEqual(["Read only", "Read & write"]);
    await user.selectOptions(screen.getByLabelText("Agent permissions"), "read_write");
    expect(screen.getByText(/Ledger changes require a preview before commit/)).toBeInTheDocument();
    expect(screen.getByText(/manage investment reviews/)).toBeInTheDocument();
    expect(enable).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Enable MCP" }));
    await waitFor(() => expect(enable).toHaveBeenCalledWith("read_write"));
  });
  it.each([["read_only", "read_write"], ["read_write", "read_only"]])("keeps a running %s credential at its permission until applied", async (initialMode, nextMode) => {
    status.mockResolvedValue({ ...on, mode: initialMode });
    const user = userEvent.setup(); mount();
    await screen.findByText("MCP is running on this computer");
    expect(screen.getByLabelText("Agent permissions")).toHaveValue(initialMode);
    expect(screen.queryByRole("button", { name: "Apply permissions" })).not.toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText("Agent permissions"), nextMode);
    expect(enable).not.toHaveBeenCalled();
    expect(screen.getByText(/Applying changes replaces the access token/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Apply permissions" }));
    await waitFor(() => expect(enable).toHaveBeenCalledWith(nextMode));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Apply permissions" })).not.toBeInTheDocument());
  });
  it("reports failed activation without displaying connection credentials", async () => {
    enable.mockRejectedValue(new Error("activation failed"));
    const user = userEvent.setup(); mount();
    await user.click(await screen.findByRole("button", { name: "Enable MCP" }));
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(connection).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "Show connection configuration" })).not.toBeInTheDocument();
  });
});

it("keeps MCP authority controls human-only even for Full generic operations",async()=>{
 const { operatePageControl }=await import("@/features/assistant/pageOperations");
 const user=userEvent.setup();mount();await screen.findByRole("button",{name:"Enable MCP"});
 const page=capturePageContent("settings:");
 for(const name of ["Agent permissions","Enable MCP"]){const node=page.nodes.find(n=>n.role!=="text" && n.name===name)!;expect(node).toBeDefined();expect(node.targetRef).toBeUndefined();expect(node.actions).toEqual([]);expect(node.state).toContain("operation_protected=true");}
 // Neither a semantic ID nor a fabricated target confers execution authority.
 const select=page.nodes.find(n=>n.role==="combobox" && n.name==="Agent permissions")!;
 expect(operatePageControl("settings:","settings:",{snapshotId:page.id,targetRef:select.id,action:"select_option",optionRef:page.nodes.find(n=>n.name==="Read & write")?.id ?? "read_write"},()=>undefined).state).toBe("invalid_target");
 expect(enable).not.toHaveBeenCalled();expect(connection).not.toHaveBeenCalled();
 await user.selectOptions(screen.getByLabelText("Agent permissions"),"read_write");
 const changed=capturePageContent("settings:");const button=changed.nodes.find(n=>n.name==="Enable MCP")!;
 expect(operatePageControl("settings:","settings:",{snapshotId:changed.id,targetRef:button.id,action:"activate"},()=>undefined).state).toBe("invalid_target");expect(enable).not.toHaveBeenCalled();
 await user.click(screen.getByRole("button",{name:"Enable MCP"}));await waitFor(()=>expect(enable).toHaveBeenCalledWith("read_write"));
 await screen.findByRole("button",{name:"Show connection configuration"});
 const running=capturePageContent("settings:");expect(running.nodes.find(n=>n.name==="Show connection configuration")?.targetRef).toBeUndefined();
 await user.click(screen.getByRole("button",{name:"Show connection configuration"}));await screen.findByRole("textbox",{name:"MCP connection configuration"});
 expect(JSON.stringify(capturePageContent("settings:"))).not.toContain("test-secret");
});
