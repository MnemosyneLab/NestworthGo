// Shared UI shape; capabilities originate in the backend's offline registry.
export type Selection = { model: string; reasoningEffort: string };
export type ModelOption = { id: string; efforts: string[] | null };
export function configuredModels(config?: { model?: string; enabledModels?: string[] | null }) {
  return config?.enabledModels?.length ? config.enabledModels : config?.model && config.model !== "unconfigured" ? [config.model] : [];
}

/** Efforts a user can pick: the backend catalog without "none". */
export const selectableEfforts = (efforts?: string[] | null) => (efforts ?? []).filter(effort => effort !== "none");
/** The effort actually used: the saved one when selectable, otherwise "low" (or the first selectable effort).
 * Models without selectable efforts keep the saved value untouched. */
export function effectiveEffort(efforts: string[] | null | undefined, current: string) {
  const list = selectableEfforts(efforts);
  if (!list.length || list.includes(current)) return current;
  return list.includes("low") ? "low" : list[0];
}
