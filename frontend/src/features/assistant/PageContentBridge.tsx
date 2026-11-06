import { useEffect, useLayoutEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import * as API from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service";
import { callService } from "@/lib/wails";
import { pollPageOperations } from "./pageOperationBridge";
import { capturePageContent } from "./pageContent";

/** Desktop host bridge, independent of sidebar visibility and local page-guide
 * opt-in. Polls request IDs only; semantic contents are captured only on demand. */
export function PageContentBridge({ viewKey, generation, blocked }: { viewKey: string; generation: string; blocked: () => boolean }) {
  const status = useQuery({
    queryKey: ["aiCredentialStatus"],
    queryFn: () => callService(() => API.Status()),
    refetchInterval: 2000,
    networkMode: "always",
    retry: false,
  });
  const epoch = status.data?.epoch ?? "";
  const available = status.data?.available ?? false;
  const current = useRef({ viewKey, generation, blocked, fullAccess: status.data?.fullAccess === true });
  useLayoutEffect(() => { current.current = { viewKey, generation, blocked, fullAccess: status.data?.fullAccess === true }; }, [viewKey, generation, blocked, status.data?.fullAccess]);
  useEffect(() => {
    if (!epoch || !available) return;
    let stopped = false;
    let timer: number | undefined;
    const poll = async () => {
      let delay = 750;
      try {
        const requests = await callService(() => API.PendingPageReads(epoch));
        for (const request of requests ?? []) {
          if (stopped) break;
          const content = capturePageContent(current.current.viewKey, current.current.generation);
          await callService(() => API.DeliverPageContent(epoch, request.id, content));
          delay = 150;
        }
        await pollPageOperations({
          pending: () => callService(() => API.PendingPageOperations(epoch)),
          claim: id => callService(() => API.ClaimPageOperation(epoch, id)),
          deliver: (id, result) => callService(() => API.DeliverPageOperation(epoch, id, result)),
        }, () => current.current, () => stopped);
      } catch { /* The requesting tool receives a bounded unavailable result. */ }
      finally { if (!stopped) timer = window.setTimeout(() => void poll(), delay); }
    };
    void poll();
    return () => { stopped = true; window.clearTimeout(timer); };
  }, [epoch, available]);
  return null;
}
