export interface OperationResult {
  id: string;
  state: string;
  runId?: string;
  toolName?: string;
  toolIndex?: number | null;
}

type Tool = { source: string; input: string; output: string };

/** Prefer host-saved invocation identity. Old traces may identify their exact
 * receipt in the original input/output; never associate by tool name alone. */
export function matchesOperation(result: OperationResult, runId: string, index: number, tool: Tool): boolean {
  if (result.toolName !== tool.source) return false;
  if (result.runId && result.toolIndex != null) return result.runId === runId && result.toolIndex === index;
  if (result.runId && result.runId !== runId) return false;
  try {
    const input = JSON.parse(tool.input);
    if (typeof input?.operationId === "string" && input.operationId.toLowerCase() === result.id) return true;
  } catch { /* A truncated input is not an identity. */ }
  try {
    return JSON.parse(tool.output)?.operationRef === result.id;
  } catch { return false; }
}

export function operationStateKey(state: string): string {
  return ["succeeded", "failed", "cancelled", "committing", "awaiting_confirmation", "needs_preview"].includes(state)
    ? `aiChat.operationState.${state}` : "aiChat.operationState.unknown";
}
