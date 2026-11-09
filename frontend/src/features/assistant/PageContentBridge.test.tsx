import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { createTestQueryClient } from "@/test/queryClient";
import { BusinessPageRoot } from "@/lib/businessCapture";
import { Button } from "@/components/ui/button";
import { ModelConversation } from "./ModelConversation";
import { PageContentBridge } from "./PageContentBridge";
import { capturePageContent } from "./pageContent";

const api = vi.hoisted(() => ({
  Status: vi.fn(), List: vi.fn(), SetFullAccess: vi.fn(),
  PendingPageReads: vi.fn(), PendingPageOperations: vi.fn(),
  ClaimPageOperation: vi.fn(), DeliverPageOperation: vi.fn(),
}));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service", () => api);

const initialStatus = {
  epoch: "synthetic", revision: 1, available: true, fullAccess: true,
  config: { provider: "openai-responses", model: "mock", enabled: true },
};
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(done => { resolve = done; });
  return { promise, resolve };
}
beforeEach(() => {
  vi.resetAllMocks();
  api.Status.mockResolvedValue(initialStatus);
  api.List.mockResolvedValue([]);
  api.PendingPageReads.mockResolvedValue([]);
  api.PendingPageOperations.mockResolvedValue([]);
  api.DeliverPageOperation.mockResolvedValue(undefined);
});

it.each(["mutation pending", "status pending", "status settled", "mutation rejected"])(
  "refuses a late claim after human permission change with %s", async timing => {
    const claim = deferred<ReturnType<typeof command>>();
    const mutation = deferred<void>();
    const refresh = deferred<typeof initialStatus>();
    api.PendingPageOperations.mockResolvedValueOnce([{ id: "claimed" }]);
    api.ClaimPageOperation.mockReturnValue(claim.promise);
    api.SetFullAccess.mockImplementation(() => timing === "mutation rejected" ? Promise.reject(new Error("synthetic failure")) : mutation.promise);
    const write = vi.fn();
    const present = vi.fn();
    const client = createTestQueryClient();
    render(<QueryClientProvider client={client}>
      <BusinessPageRoot view="synthetic:" accessIdentity="synthetic">
        <Button onClick={write}>Save synthetic money</Button>
        <Button data-assistant-effect="presentation" onClick={present}>Show synthetic details</Button>
      </BusinessPageRoot>
      <PageContentBridge viewKey="synthetic:" generation="generation" blocked={() => false} />
      <ModelConversation />
    </QueryClientProvider>);
    await waitFor(() => expect(api.ClaimPageOperation).toHaveBeenCalledOnce());
    function command(name: string) {
      const capture = capturePageContent("synthetic:", "generation");
      return {
        snapshotId: capture.id, targetRef: capture.nodes.find(node => node.name === name)!.targetRef!,
        action: "activate", expiresAt: new Date(Date.now() + 1900).toISOString(),
      };
    }
    // The backend admitted this claim before the human revocation. Only its IPC
    // response is delayed; no handler has been dispatched yet.
    const operation = command("Save synthetic money");
    api.Status.mockReturnValue(refresh.promise);
    const permission = screen.getByRole("button", { name: /^Permissions/ });
    await userEvent.click(permission);
    await userEvent.click(await screen.findByRole("menuitemradio", { name: /Read-only/ }));
    expect(api.SetFullAccess).toHaveBeenCalledWith("synthetic", 1, false);
    if (timing !== "mutation pending") await act(async () => mutation.resolve());
    const nextStatus = timing === "mutation rejected" ? initialStatus : { ...initialStatus, revision: 2, fullAccess: false };
    if (timing === "status settled" || timing === "mutation rejected") {
      await act(async () => refresh.resolve(nextStatus));
      await waitFor(() => expect(permission).toBeEnabled());
    }
    await act(async () => claim.resolve(operation));
    expect(write).not.toHaveBeenCalled();
    expect(api.DeliverPageOperation).toHaveBeenCalledExactlyOnceWith("synthetic", "claimed", { state: "not_dispatched", snapshotId: operation.snapshotId });
    await act(async () => { mutation.resolve(); refresh.resolve(nextStatus); });
    await waitFor(() => expect(permission).toBeEnabled());

    // Reconciliation releases the local pause. New Read-only presentation
    // operations remain available; only authoritative Full permits a write.
    api.ClaimPageOperation.mockImplementation(async (_epoch, id) => command(id === "presentation" ? "Show synthetic details" : "Save synthetic money"));
    api.PendingPageOperations.mockResolvedValueOnce([{ id: "presentation" }, { id: "fresh-write" }]);
    await waitFor(() => expect(present).toHaveBeenCalledOnce(), { timeout: 2000 });
    await waitFor(() => expect(api.DeliverPageOperation).toHaveBeenCalledWith("synthetic", "fresh-write", expect.objectContaining({ state: timing === "mutation rejected" ? "dispatched" : "blocked" })), { timeout: 2000 });
    expect(write).toHaveBeenCalledTimes(timing === "mutation rejected" ? 1 : 0);
  },
);
