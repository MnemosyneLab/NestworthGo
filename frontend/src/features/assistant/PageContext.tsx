import {
  createContext,
  useCallback,
  useContext,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { PageHost } from "./pageHost";
import { PageContentBridge } from "./PageContentBridge";
import { TrendVerification, type TrendObservation } from "./trendVerification";
import {
  localId,
  type PageContextV1,
  type PagePublication,
  type NavigationResult,
} from "./contracts";

import {
  applyModelNavigation,
  modelNavigationAllowed,
  type ModelPageState,
  type ModelTarget,
} from "./modelNavigation";

type ContextValue = {
  reportTrend: (value: TrendObservation) => void;
  modelStamp: string;
  canModelNavigate: (stamp: string) => boolean;
  navigateModel: (
    stamp: string,
    resolve: () => Promise<ModelTarget>,
  ) => Promise<NavigationResult>;
  enabled: boolean;
  setEnabled: (enabled: boolean) => void;
  viewKey: string;
  context: PageContextV1 | null;
  publish: (publication: PagePublication, refresh?: () => void) => () => void;
  refresh?: () => void;
  navigate: (ref: string, snapshot: PageContextV1) => NavigationResult;
};
const PageContext = createContext<ContextValue | null>(null);

export function AssistantPageProvider({
  viewKey,
  householdId,
  incarnation,
  surface,
  blocked,
  onNavigate,
  children,
}: {
  viewKey: string;
  householdId: string;
  incarnation: string;
  surface: "desktop" | "mobile";
  blocked: () => boolean;
  onNavigate: (
    target: PagePublication["targets"][number]["destination"],
  ) => boolean | void;
  children: ReactNode;
}) {
  const [enabled, setEnabled] = useState(false);
  const principal = useMemo(
    () => ({
      householdId,
      ledgerIncarnation: incarnation,
      assistantSessionId: localId(),
      permissionRevision: localId(),
      surface,
    }),
    [householdId, incarnation, surface],
  );
  const host = useMemo(
    () => new PageHost(principal, Date.now, enabled ? viewKey : ""),
    [principal, viewKey, enabled],
  );
  const [publication, setPublication] = useState<{
    host: PageHost;
    viewKey: string;
    context: PageContextV1 | null;
    refresh?: () => void;
  } | null>(null);
  // Revoke before paint and before a user can act after a route/authority change.
  useLayoutEffect(() => () => host.revoke(), [host]);
  const context =
    enabled && publication?.host === host && publication.viewKey === viewKey
      ? publication.context
      : null;
  const publish = useCallback(
    (value: PagePublication, refresh?: () => void) => {
      if (!enabled || value.viewKey !== viewKey) return () => {};
      const next = { host, viewKey, context: host.publish(value), refresh };
      setPublication(next);
      return () => {
        host.revoke();
        setPublication((previous) => (previous === next ? null : previous));
      };
    },
    [host, viewKey, enabled],
  );
  const navigate = useCallback(
    (ref: string, snapshot: PageContextV1) => {
      const turn = localId();
      return host.navigate(
        {
          version: 1,
          intentId: localId(),
          userTurnId: turn,
          contextId: snapshot.contextId,
          pageVersion: snapshot.pageVersion,
          action: "open_object",
          targetRef: ref,
        },
        turn,
        blocked(),
        onNavigate,
      );
    },
    [host, blocked, onNavigate],
  );
  const modelBinding = useMemo(
    () => ({ host, contextID: context?.contextId, stamp: localId() }),
    [host, context?.contextId],
  );
  const modelStamp = modelBinding.stamp;
  const trendVerification = useMemo(() => new TrendVerification(), []);
  useLayoutEffect(() => () => trendVerification.cancel(), [trendVerification, principal]);
  useLayoutEffect(() => {
    if (viewKey.split(":")[0] !== "return-analysis") trendVerification.cancel();
  }, [viewKey, trendVerification]);
  const reportTrend = useCallback((value: TrendObservation) => {
    if (viewKey.split(":")[0] !== "return-analysis" || blocked()) trendVerification.cancel();
    else trendVerification.observe(value);
  }, [viewKey, blocked, trendVerification]);
  const modelState = useRef<ModelPageState>({
    stamp: "",
    surface,
    blocked,
    dispatch: (target) => onNavigate(target),
  });
  useLayoutEffect(() => {
    modelState.current = {
      stamp: modelStamp,
      surface,
      blocked,
      dispatch: (target) => onNavigate(target),
      verifyTrend: async (target, expected) => {
        const result = trendVerification.begin(expected);
        try { if (onNavigate(target) === false) trendVerification.cancel(); }
        catch { trendVerification.cancel(); }
        return result;
      },
    };
    return () => {
      if (modelState.current.stamp === modelStamp)
        modelState.current = { ...modelState.current, stamp: "" };
    };
  }, [modelStamp, surface, blocked, onNavigate, trendVerification]);
  const canModelNavigate = useCallback(
    (stamp: string) => modelNavigationAllowed(stamp, modelState.current),
    [],
  );
  const navigateModel = useCallback(
    (stamp: string, resolve: () => Promise<ModelTarget>) =>
      applyModelNavigation(stamp, () => modelState.current, resolve),
    [],
  );
  return (
    <PageContext.Provider
      value={{
        reportTrend,
        modelStamp,
        canModelNavigate,
        navigateModel,
        enabled,
        setEnabled,
        viewKey,
        context,
        publish,
        navigate,
        refresh:
          enabled &&
          publication?.host === host &&
          publication.viewKey === viewKey
            ? publication.refresh
            : undefined,
      }}
    >
      {surface === "desktop" && <PageContentBridge viewKey={viewKey} generation={modelStamp} blocked={blocked} />}
      {children}
    </PageContext.Provider>
  );
}

export function useAssistantPage() {
  return useContext(PageContext);
}

export function usePublishAssistantPage(
  value: PagePublication,
  refresh?: () => void,
) {
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
