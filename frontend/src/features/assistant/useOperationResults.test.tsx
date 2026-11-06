import { beforeEach, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { createTestQueryClient } from "@/test/queryClient";
import { useOperationResults } from "./useOperationResults";

const api = vi.hoisted(() => ({ Reviews: vi.fn(), Start: vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service", () => api);
beforeEach(() => { vi.clearAllMocks(); api.Reviews.mockResolvedValue([]); });

function mount() {
  const client = createTestQueryClient();
  client.setQueryData(["accounts"], "synthetic");
  client.setQueryData(["aiConversation", "epoch", "conversation"], "saved");
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  const hook = renderHook(({ id, state }) => useOperationResults("epoch", id, false, { id: "run", state }), {
    wrapper, initialProps: { id: "conversation", state: "succeeded" },
  });
  return { client, ...hook };
}

it.each(["ai_full"])("refreshes business data for inspected %s success without dispatching a write", async source => {
  api.Reviews.mockResolvedValue([{ id: "operation", source, state: "succeeded" }]);
  const { client, rerender } = mount();
  await waitFor(() => expect(client.getQueryState(["accounts"])?.isInvalidated).toBe(true));
  expect(client.getQueryState(["aiConversation", "epoch", "conversation"])?.isInvalidated).toBe(false);
  rerender({ id: "another-conversation", state: "succeeded" });
  await waitFor(() => expect(api.Reviews).toHaveBeenCalledWith("epoch", "another-conversation"));
  expect(api.Start).not.toHaveBeenCalled();
});

it.each(["unknown", "committing", "needs_preview", "failed"])("keeps %s inspection inert and refreshes after a new run", async state => {
  api.Reviews.mockResolvedValue([{ id: "operation", source: "ai_full", state }]);
  const { client, rerender } = mount();
  await waitFor(() => expect(client.getQueryData(["aiReviews", "epoch", "conversation", "run:succeeded"])).toBeDefined());
  expect(client.getQueryState(["accounts"])?.isInvalidated).toBe(false);
  api.Reviews.mockResolvedValue([{ id: "operation", source: "ai_full", state: "succeeded" }]);
  rerender({ id: "conversation", state: "running" });
  await waitFor(() => expect(client.getQueryState(["accounts"])?.isInvalidated).toBe(true));
  expect(api.Start).not.toHaveBeenCalled();
});

it("reconciles an unknown result by polling without replay", async () => {
  api.Reviews.mockResolvedValueOnce([{ id: "operation", state: "unknown" }])
    .mockResolvedValue([{ id: "operation", state: "succeeded" }]);
  const { client } = mount();
  await waitFor(() => expect(client.getQueryState(["accounts"])?.isInvalidated).toBe(true), { timeout: 3000 });
  expect(api.Start).not.toHaveBeenCalled();
});

it("retries failed receipt reads without losing operation recovery", async () => {
  api.Reviews.mockRejectedValueOnce(new Error("lost read"))
    .mockResolvedValue([{ id: "operation", state: "succeeded" }]);
  const { client } = mount();
  await waitFor(() => expect(client.getQueryState(["accounts"])?.isInvalidated).toBe(true), { timeout: 3000 });
  expect(api.Start).not.toHaveBeenCalled();
});
