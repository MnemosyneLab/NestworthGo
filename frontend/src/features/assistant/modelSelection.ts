// Shared UI shape; capabilities originate in the backend's offline registry.
export type Selection = { model: string; reasoningEffort: string };
export type ModelOption = { id: string; efforts: string[] | null };
export function configuredModels(config?: { model?: string; enabledModels?: string[] | null }) {
  return config?.enabledModels?.length ? config.enabledModels : config?.model && config.model !== "unconfigured" ? [config.model] : [];
}
