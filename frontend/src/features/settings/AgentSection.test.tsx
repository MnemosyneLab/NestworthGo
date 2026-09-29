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
function mount() { const client = createTestQueryClient({ retry: false }); return render(<QueryClientProvider client={client}><AgentSection /></QueryClientProvider>); }
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
    await user.click(screen.getByRole("button", { name: "Disable and revoke access" }));
    await screen.findByText("MCP is off");
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
  });
  it("requires applying directory-write permission and explains its scope", async () => {
    const user = userEvent.setup(); mount();
    await screen.findByRole("button", { name: "Enable MCP" });
    await user.selectOptions(screen.getByLabelText("Agent permissions"), "directory_write");
    expect(enable).not.toHaveBeenCalled();
    expect(screen.getByText(/without another App confirmation/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Enable MCP" }));
    await waitFor(() => expect(enable).toHaveBeenCalledWith("directory_write"));
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
