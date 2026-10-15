import { expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { useReconciliationSessions } from "./reconciliation";
const list = vi.hoisted(() => vi.fn());
vi.mock(
  "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation",
  () => ({ Service: { List: list } }),
);
it("limits account-entry history to the selected scope and isolates cached account filters", async () => {
  list.mockResolvedValue([
    { id: "first", accountIds: ["one"], accountNames: ["Synthetic bank"] },
    { id: "second", accountIds: ["two"], accountNames: ["Other bank"] },
  ]);
  const client = createTestQueryClient();
  const { result, rerender } = renderHook(
    ({ accountId }) => useReconciliationSessions(accountId),
    {
      initialProps: { accountId: "one" },
      wrapper: ({ children }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
      ),
    },
  );
  await waitFor(() =>
    expect(result.current.data?.map((row) => row.id)).toEqual(["first"]),
  );
  rerender({ accountId: "two" });
  await waitFor(() =>
    expect(result.current.data?.map((row) => row.id)).toEqual(["second"]),
  );
  expect(client.getQueryData(["reconciliation", "sessions", "one"])).toEqual([
    { id: "first", accountIds: ["one"], accountNames: ["Synthetic bank"] },
  ]);
});
