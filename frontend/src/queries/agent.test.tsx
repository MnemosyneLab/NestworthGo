import { act, render } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import { createTestQueryClient } from "@/test/queryClient";
import { AgentWorkspaceObserver } from "./agent";

const { on, stop } = vi.hoisted(() => ({ on: vi.fn(), stop: vi.fn() }));
vi.mock("@wailsio/runtime", () => ({ Events: { On: on } }));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/agent", () => ({ Service: {} }));
it("invalidates cached account data after an external mutation and unsubscribes", () => {
  on.mockReturnValue(stop);
  const client = createTestQueryClient();
  client.setQueryData(["accounts"], [{ id: "old" }]);
  const view = render(<QueryClientProvider client={client}><AgentWorkspaceObserver /></QueryClientProvider>);
  expect(on).toHaveBeenCalledWith("agent.data.changed", expect.any(Function));
  act(() => on.mock.calls[0][1]());
  expect(client.getQueryState(["accounts"])?.isInvalidated).toBe(true);
  view.unmount();
  expect(stop).toHaveBeenCalledOnce();
});
