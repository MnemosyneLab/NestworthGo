import { createContext, useContext, useId, useMemo, type HTMLAttributes, type ReactNode } from "react";

type Owner = { id: string; view: string; accessIdentity: string };
const SecurityOperationContext = createContext(false);
const BusinessCaptureContext = createContext<Owner | null>(null);

/** This is the application-owned business surface, not the app shell or assistant.
 * Access identity is a freshness fence, never backend permission or navigation authority. */
export function BusinessPageRoot({ view, accessIdentity, occluded = false, children, ...props }: HTMLAttributes<HTMLElement> & { view: string; accessIdentity: string; occluded?: boolean; children: ReactNode }) {
 const owner=useMemo(()=>({id:crypto.randomUUID(),view,accessIdentity}),[view,accessIdentity]);
 return <BusinessCaptureContext.Provider value={owner}><main {...props} hidden={occluded} data-assistant-root="business" data-assistant-view={view} data-assistant-owner={owner.id} data-assistant-identity={accessIdentity} data-assistant-occluded={occluded ? "assistant" : undefined}>{children}</main></BusinessCaptureContext.Provider>;
}

/** Portal publishers leave a nonvisual origin in their actual React/DOM page.
 * A retained hidden route/tab or private origin cannot publish readable content. */
export function useBusinessPortal() {
 const owner=useContext(BusinessCaptureContext); const origin=useId();
 const protectedOperations=useContext(SecurityOperationContext);
 return {
  origin: owner ? <span className="hidden" aria-hidden="true" data-assistant-origin={origin} data-assistant-owner={owner.id} /> : null,
  props: owner ? {"data-assistant-portal":origin,"data-assistant-owner":owner.id,...(protectedOperations ? {"data-assistant-operation":"protected"} : {})} : {},
 };
}

/** Persistent value boundary: hiding/revealing or rendering as text changes nothing. */
export function SecretRegion({children,...props}:HTMLAttributes<HTMLDivElement> & {children:ReactNode}) {
 return <div {...props} data-assistant-private="credential">{children}</div>;
}

/** Public security status stays readable, but its controls are human-only even
 * in Full mode. React context preserves the boundary through shared portals. */
export function SecurityOperationRegion({children,...props}:HTMLAttributes<HTMLDivElement> & {children:ReactNode}) {
 return <SecurityOperationContext.Provider value={true}><div {...props} data-assistant-operation="protected">{children}</div></SecurityOperationContext.Provider>;
}
