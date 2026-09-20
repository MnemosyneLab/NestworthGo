import { expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SnapshotRepair } from "./SnapshotRepair";
const rebuild = vi.hoisted(() => vi.fn());
vi.mock("@/queries/history", () => ({ useRebuildHistoricalSnapshots: () => ({ mutateAsync: rebuild }) }));
it("retries only the failed 31-day chunk and remaining days", async () => {
  rebuild.mockResolvedValueOnce({}).mockRejectedValueOnce(new Error("Rebuild interrupted")).mockResolvedValue({});
  render(<SnapshotRepair focus={{ rangeStart: "2026-01-01", rangeEnd: "2026-03-05" }} onClose={vi.fn()} />);
  await userEvent.click(screen.getByRole("button", { name: "Rebuild snapshots" }));
  await screen.findByRole("alert");
  expect(rebuild.mock.calls.map(([request]) => request)).toEqual([
    { startDate: "2026-01-01", endDate: "2026-01-31" },
    { startDate: "2026-02-01", endDate: "2026-03-03" },
  ]);
  await userEvent.click(screen.getByRole("button", { name: "Try again" }));
  await waitFor(() => expect(rebuild).toHaveBeenCalledTimes(4));
  expect(rebuild.mock.calls.slice(2).map(([request]) => request)).toEqual([
    { startDate: "2026-02-01", endDate: "2026-03-03" },
    { startDate: "2026-03-04", endDate: "2026-03-05" },
  ]);
});
