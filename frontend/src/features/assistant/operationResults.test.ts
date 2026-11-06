import { expect, it } from "vitest";
import { matchesOperation, operationStateKey } from "./operationResults";

const tool = { source: "create_member", input: '{"operationId":"receipt"}', output: "" };
it("uses the host run and tool ordinal instead of confusing repeated same-name tools", () => {
  const result = { id: "receipt", state: "succeeded", runId: "original", toolIndex: 1, toolName: "create_member" };
  expect(matchesOperation(result, "original", 1, tool)).toBe(true);
  expect(matchesOperation(result, "original", 0, tool)).toBe(false);
  expect(matchesOperation(result, "other-run", 1, tool)).toBe(false);
});
it("matches legacy receipts only by exact operation identity", () => {
  const result = { id: "receipt", state: "succeeded", toolName: "create_member" };
  expect(matchesOperation(result, "old-run", 0, tool)).toBe(true);
  expect(matchesOperation(result, "old-run", 0, { ...tool, input: '{"operationId":"another"}' })).toBe(false);
  expect(matchesOperation(result, "old-run", 0, { ...tool, input: '{"operationId":"rece' })).toBe(false);
  expect(matchesOperation(result, "old-run", 0, { ...tool, input: "", output: '{"operationRef":"receipt"}' })).toBe(true);
  expect(matchesOperation(result, "old-run", 0, { ...tool, source: "get_operation" })).toBe(false);
});
it("keeps unknown states explicit", () => {
  expect(operationStateKey("future_state")).toBe("aiChat.operationState.unknown");
  expect(operationStateKey("failed")).toBe("aiChat.operationState.failed");
});
