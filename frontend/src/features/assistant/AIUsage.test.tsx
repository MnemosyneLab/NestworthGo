import { beforeEach, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { AIUsageSettings, UsageNumbers } from "./AIUsage";
const usage = vi.hoisted(() => vi.fn());
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service", () => ({ Usage: usage }));
const total = { estimatedUsd: "0", cachedUsd: "0", cacheWriteUsd: "0", pricedCalls: 0, unpricedCalls: 2, calls: 2, failed: 1, pending: 0, input: 100, output: 0, cachedInput: 0, cacheWrite: 0, reasoning: 0, missingInput: 1, missingOutput: 2, missingCached: 1, missingWrite: 2, missingReasoning: 2, tokens: 0, missingTokens: 2, audioCalls: 0, audioSeconds: "", missingAudio: 0 };
beforeEach(() => {usage.mockReset();usage.mockResolvedValue({available: true, incomplete: false, costStatus: "unknown_price", total, models: ["actual-model"], groups: [], conversations: [{key: "opaque-id", label: "Synthetic plan", deleted: false}]});});
it("distinguishes unknown from known zero and partial subtotals", () => {
 render(<UsageNumbers total={total} />);
 expect(screen.getByText("100 + ?")).toBeInTheDocument();
 expect(within(screen.getByText("Cached input").parentElement!).getByText("0 + ?")).toBeInTheDocument();
 expect(within(screen.getByText("Output").parentElement!).getByText("—")).toBeInTheDocument();
});
it("passes timezone, model and conversation filters to the host", async () => {
 const user=userEvent.setup();
 render(<QueryClientProvider client={createTestQueryClient()}><AIUsageSettings epoch="epoch" /></QueryClientProvider>);
 await screen.findByText("100 + ?");
 await user.selectOptions(screen.getByLabelText("Group by"),"model");
 await user.selectOptions(screen.getByLabelText("Model"),"actual-model");
 await user.selectOptions(screen.getByLabelText("Conversation"),"opaque-id");
 await waitFor(()=>expect(usage).toHaveBeenLastCalledWith("epoch",expect.objectContaining({groupBy:"model",model:"actual-model",conversationId:"opaque-id"})));
 expect(screen.queryByText("opaque-id")).not.toBeInTheDocument();
 expect(screen.getByText(/Public-price estimates/)).toBeInTheDocument();
});

it("renders nullable generated collections and partial estimated costs", async () => {
 usage.mockResolvedValue({ available: true, incomplete: false, prices: null, models: null, groups: null, conversations: null, total: {...total, estimatedUsd: "0.0032", cachedUsd: "0.00005", cacheWriteUsd: "0.00035", pricedCalls: 1, unpricedCalls: 1} });
 render(<QueryClientProvider client={createTestQueryClient()}><AIUsageSettings epoch="epoch" /></QueryClientProvider>);
 expect(await screen.findByText("1 request unpriced")).toBeInTheDocument();
 expect(screen.getByTitle("0.0032")).toHaveTextContent("+ ?");
});
