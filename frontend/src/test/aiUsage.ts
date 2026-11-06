// New read-only endpoint fixture shared by existing chat/settings integration tests.
export const emptyUsageReport = {
  available: true, incomplete: false, costStatus: "unknown_price",
  prices: [], models: [], groups: [], conversations: [],
  total: { estimatedUsd: "0", cachedUsd: "0", cacheWriteUsd: "0", pricedCalls: 0, unpricedCalls: 0, calls: 0, failed: 0, pending: 0, tokens: 0, missingTokens: 0,
    input: 0, output: 0, cachedInput: 0, cacheWrite: 0, reasoning: 0,
    missingInput: 0, missingOutput: 0, missingCached: 0, missingWrite: 0, missingReasoning: 0,
    audioCalls: 0, audioSeconds: "", missingAudio: 0 },
};
