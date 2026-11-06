import type { QueryClient } from "@tanstack/react-query";

type Authority = { generation: number; pending: number };
const authorities = new WeakMap<QueryClient, Authority>();

function authority(client: QueryClient) {
  let state = authorities.get(client);
  if (!state) {
    state = { generation: 0, pending: 0 };
    authorities.set(client, state);
  }
  return state;
}

/** Human permission changes revoke locally before IPC or query reconciliation.
 * Backend revision checks remain authoritative for newly claimed operations.
 * Scope this ephemeral fence to the desktop query client, never browser storage. */
export function pausePageOperations(client: QueryClient) {
  const state = authority(client);
  state.generation++;
  state.pending++;
  let resumed = false;
  return () => {
    if (resumed) return;
    resumed = true;
    state.pending--;
  };
}

/** A permission change permanently invalidates an already awaited claim, even
 * if reconciliation finishes before its response reaches the frontend. */
export function capturePageOperationAuthority(client: QueryClient) {
  const state = authority(client);
  const generation = state.generation;
  const admitted = state.pending === 0;
  return () => admitted && state.pending === 0 && generation === state.generation;
}
