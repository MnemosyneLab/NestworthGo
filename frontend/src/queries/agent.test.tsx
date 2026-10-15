import { act, render } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { createTestQueryClient } from "@/test/queryClient";
import { AgentWorkspaceObserver } from "./agent";

const { on, stop } = vi.hoisted(() => ({ on: vi.fn(), stop: vi.fn() }));
vi.mock("@wailsio/runtime", () => ({ Events: { On: on } }));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/agent", () => ({ Service: {} }));
beforeEach(()=>vi.clearAllMocks());
it("invalidates cached account data after an external mutation and unsubscribes", () => {
  on.mockReturnValue(stop);
  const client = createTestQueryClient();
  client.setQueryData(["accounts"], [{ id: "old" }]);
  const view = render(<QueryClientProvider client={client}><AgentWorkspaceObserver /></QueryClientProvider>);
  expect(on).toHaveBeenCalledWith("agent.data.changed", expect.any(Function));
  act(() => on.mock.calls[0][1]());
  expect(client.getQueryState(["accounts"])?.isInvalidated).toBe(true);
  view.unmount();
  expect(stop).toHaveBeenCalledTimes(2);
});

it("refreshes reconciliation alone for a session attestation event",()=>{on.mockReturnValue(stop);const client=createTestQueryClient();const performance=["quote","instrument","synthetic","performance"];client.setQueryData(performance,{returnPercent:"2.5"});client.setQueryData(["accounts"],[{id:"unchanged"}]);client.setQueryData(["reconciliation","sessions"],[]);const view=render(<QueryClientProvider client={client}><AgentWorkspaceObserver/></QueryClientProvider>);const listener=on.mock.calls.find(args=>args[0]==="agent.reconciliation.changed");expect(listener).toBeDefined();act(()=>listener![1]());expect(client.getQueryState(["reconciliation","sessions"])?.isInvalidated).toBe(true);expect(client.getQueryState(performance)?.isInvalidated).toBe(false);expect(client.getQueryState(["accounts"])?.isInvalidated).toBe(false);view.unmount();expect(stop).toHaveBeenCalledTimes(2);});
