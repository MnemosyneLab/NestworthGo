import { createContext, useCallback, useContext, useLayoutEffect, useMemo, useState, type ReactNode } from "react";
import { PageHost } from "./pageHost";
import { localId, type PageContextV1, type PagePublication, type NavigationResult } from "./contracts";

type ContextValue = {
  enabled: boolean; setEnabled: (enabled: boolean) => void;
  viewKey: string; context: PageContextV1 | null;
  publish: (publication: PagePublication, refresh?: () => void) => () => void;
  refresh?: () => void;
  navigate: (ref: string, snapshot: PageContextV1) => NavigationResult;
};
const PageContext = createContext<ContextValue | null>(null);

export function AssistantPageProvider({ viewKey, householdId, incarnation, surface, blocked, onNavigate, children }: {
  viewKey: string; householdId: string; incarnation: string; surface: "desktop" | "mobile";
  blocked: () => boolean; onNavigate: (target: PagePublication["targets"][number]["destination"]) => void; children: ReactNode;
}) {
  const [enabled, setEnabled] = useState(false);
  const principal = useMemo(() => ({ householdId, ledgerIncarnation: incarnation,
    assistantSessionId: localId(), permissionRevision: localId(), surface }), [householdId, incarnation, surface]);
  const host = useMemo(() => new PageHost(principal, Date.now, enabled ? viewKey : ""), [principal, viewKey, enabled]);
  const [publication, setPublication] = useState<{ host: PageHost; viewKey: string; context: PageContextV1 | null; refresh?: () => void } | null>(null);
  // Revoke before paint and before a user can act after a route/authority change.
  useLayoutEffect(() => () => host.revoke(), [host]);
  const context = enabled && publication?.host === host && publication.viewKey === viewKey ? publication.context : null;
  const publish = useCallback((value: PagePublication, refresh?: () => void) => {
    if (!enabled || value.viewKey !== viewKey) return () => {};
    const next = { host, viewKey, context: host.publish(value), refresh };
    setPublication(next);
    return () => {
      host.revoke();
      setPublication(previous => previous === next ? null : previous);
    };
  }, [host, viewKey, enabled]);
  const navigate = useCallback((ref: string, snapshot: PageContextV1) => {
    const turn = localId();
    return host.navigate({ version: 1, intentId: localId(), userTurnId: turn, contextId: snapshot.contextId,
      pageVersion: snapshot.pageVersion, action: "open_object", targetRef: ref }, turn, blocked(), onNavigate);
  }, [host, blocked, onNavigate]);
  return <PageContext.Provider value={{ enabled, setEnabled, viewKey, context, publish, navigate,
    refresh: enabled && publication?.host === host && publication.viewKey === viewKey ? publication.refresh : undefined }}>{children}</PageContext.Provider>;
}

export function useAssistantPage() { return useContext(PageContext); }

export function usePublishAssistantPage(value: PagePublication, refresh?: () => void) {
  const context = useContext(PageContext);
  const publish = context?.publish;
  // Capture only this typed, explicitly built DTO; never the query cache, DOM,
  // input fields, settings objects, tokens or arbitrary component props.
  const encoded = context?.enabled ? JSON.stringify(value) : null;
  useLayoutEffect(() => {
    if (!publish || !encoded) return;
    return publish(JSON.parse(encoded) as PagePublication, refresh);
  }, [publish, encoded, refresh]);
}
