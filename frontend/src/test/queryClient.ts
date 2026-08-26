import { QueryClient } from "@tanstack/react-query";

/** createTestQueryClient matches the app's retry: 1 default so tests do
 * not silently depend on TanStack Query's retry: 3. Pass retry: false
 * when a surface must fail on the first rejection (Settings load). */
export function createTestQueryClient(options?: { retry?: number | false }) {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: options?.retry ?? 1 },
      mutations: { retry: 0 },
    },
  });
}
