import { createContext, useCallback, useContext, useLayoutEffect, useMemo, useState, type ReactNode } from "react";
import { PageHost } from "./pageHost";
import { localId, type PageContextV1, type PagePublication, type NavigationResult } from "./contracts";

type ContextValue = {
  viewKey: string; context: PageContextV1 | null;
  publish: (publication: PagePublication) => () => void;
  navigate: (ref: string, snapshot: PageContextV1) => NavigationResult;
};
const PageContext = createContext<ContextValue | null>(null);

export function AssistantPageProvider({ viewKey, householdId, incarnation, surface, blocked, onNavigate, children }: {
  viewKey: string; householdId: string; incarnation: string; surface: "desktop" | "mobile";
  blocked: () => boolean; onNavigate: (target: PagePublication["targets"][number]["destination"]) => void; children: ReactNode;
}) {
  const principal = useMemo(() => ({ householdId, ledgerIncarnation: incarnation,
    assistantSessionId: localId(), permissionRevision: localId(), surface }), [householdId, incarnation, surface]);
  const host = useMemo(() => new PageHost(principal, Date.now, viewKey), [principal, viewKey]);
  const [publication, setPublication] = useState<{ host: PageHost; viewKey: string; context: PageContextV1 | null } | null>(null);
  // Revoke before paint and before a user can act after a route/authority change.
  useLayoutEffect(() => () => host.revoke(), [host]);
  const context = publication?.host === host && publication.viewKey === viewKey ? publication.context : null;
  const publish = useCallback((value: PagePublication) => {
    if (value.viewKey !== viewKey) return () => {};
    const next = { host, viewKey, context: host.publish(value) };
    setPublication(next);
    return () => {
      host.revoke();
      setPublication(previous => previous === next ? null : previous);
    };
  }, [host, viewKey]);
  const navigate = useCallback((ref: string, snapshot: PageContextV1) => {
    const turn = localId();
    return host.navigate({ version: 1, intentId: localId(), userTurnId: turn, contextId: snapshot.contextId,
      pageVersion: snapshot.pageVersion, action: "open_object", targetRef: ref }, turn, blocked(), onNavigate);
  }, [host, blocked, onNavigate]);
  return <PageContext.Provider value={{ viewKey, context, publish, navigate }}>{children}</PageContext.Provider>;
}

export function useAssistantPage() { return useContext(PageContext); }

export function usePublishAssistantPage(value: PagePublication) {
  const context = useContext(PageContext);
  const publish = context?.publish;
  // Capture only this typed, explicitly built DTO; never the query cache, DOM,
  // input fields, settings objects, tokens or arbitrary component props.
  const encoded = JSON.stringify(value);
  useLayoutEffect(() => {
    if (!publish) return;
    return publish(JSON.parse(encoded) as PagePublication);
  }, [publish, encoded]);
}
