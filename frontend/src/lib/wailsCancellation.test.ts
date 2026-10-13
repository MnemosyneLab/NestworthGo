import { expect, it, vi } from "vitest";
import { Call, CancelledRejectionError, CancellablePromise, objectNames, setTransport } from "@wailsio/runtime";
import { Service } from "../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/analytics";
import { callCancellableService } from "./wails";

it("uses the installed Wails cancellable promise and keeps cancellation out of the wire-error contract", async () => {
  const controller = new AbortController();
  const native = CancellablePromise.withResolvers<string>();
  const cancel = vi.fn(); native.oncancelled = cancel;
  const result = callCancellableService(controller.signal, () => native.promise);
  const rejected = expect(result).rejects.toMatchObject({ name: "AbortError" });
  controller.abort();
  await rejected;
  expect(cancel).toHaveBeenCalledOnce();
  // A late successful native reply is allowed by Wails after cancellation.
  native.resolve("late");
});

it.each(["throw", "reject"])("survives cancellation delivery failure (%s) and suppresses the abandoned result", async failure => {
  const controller = new AbortController();
  let resolve!: (value: string) => void;
  const cancel = vi.fn(() => {
    if (failure === "throw") throw new Error("delivery failure");
    return Promise.reject(new Error("delivery failure"));
  });
  const pending = Object.assign(new Promise<string>(done => { resolve = done; }), { cancel });
  const result = callCancellableService(controller.signal, () => pending);
  const rejected = expect(result).rejects.toMatchObject({ name: "AbortError" });
  controller.abort(); resolve("late"); await rejected;
  expect(cancel).toHaveBeenCalledOnce();
  await expect(callCancellableService(new AbortController().signal, () => Promise.resolve("final"))).resolves.toBe("final");
});

it("does not dispatch an already-aborted selection", async () => {
  const controller = new AbortController(); controller.abort();
  const call = vi.fn();
  await expect(callCancellableService(controller.signal, call)).rejects.toMatchObject({ name: "AbortError" });
  expect(call).not.toHaveBeenCalled();
});

it("supports AbortSignal without the newer throwIfAborted/reason API", async () => {
  const controller = new AbortController();
  Object.defineProperty(controller.signal, "throwIfAborted", { value: undefined });
  Object.defineProperty(controller.signal, "reason", { value: undefined });
  await expect(callCancellableService(controller.signal, () => Promise.resolve("normal"))).resolves.toBe("normal");
  controller.abort();
  const call = vi.fn();
  await expect(callCancellableService(controller.signal, call)).rejects.toMatchObject({ name: "AbortError" });
  expect(call).not.toHaveBeenCalled();
});

it("handles an abort racing native call creation and removes the handler on success", async () => {
  const controller = new AbortController(); const cancel = vi.fn();
  await expect(callCancellableService(controller.signal, () => {
    controller.abort();
    return Object.assign(Promise.resolve("late"), { cancel });
  })).rejects.toMatchObject({ name: "AbortError" });
  expect(cancel).toHaveBeenCalledOnce();
  const next = new AbortController(); const settledCancel = vi.fn();
  await expect(callCancellableService(next.signal, () => Object.assign(Promise.resolve("final"), { cancel: settledCancel }))).resolves.toBe("final");
  next.abort(); expect(settledCancel).not.toHaveBeenCalled();
});

it("reports a native failure as a real error while its observer is live", async () => {
  await expect(callCancellableService(new AbortController().signal, () => Promise.reject(new Error("unexpected native cancellation"))))
    .rejects.toHaveProperty("wireError.code", "internal");
});

it("normalizes a synchronous dispatch failure for a live observer", async () => {
  await expect(callCancellableService(new AbortController().signal, () => { throw new Error("native dispatch failed"); }))
    .rejects.toHaveProperty("wireError.code", "internal");
});

it.each(["resolve", "reject"])("cancels the exact generated binding call through the installed runtime transport and handles its late %s", async completion => {
  const requests: Array<{ object: number; args: unknown }> = [];
  let finishNative!: (value: unknown) => void;
  let rejectNative!: (reason: unknown) => void;
  let abandoned!: CancellablePromise<unknown>;
  const unhandledEvents: Array<PromiseRejectionEvent> = [];
  const actualReject = Promise.reject.bind(Promise);
  // jsdom runs promises on Node's host, which does not deliver browser
  // unhandledrejection events. Observe the REAL orphan rejection created by
  // Wails and deliver its reason/identity through the browser event contract.
  // Explicit defaultPrevented assertions keep this shim from hiding a failure.
  const rejectionHost = vi.spyOn(Promise, "reject").mockImplementation(reason => {
    const rejected = actualReject(reason);
    if (reason instanceof CancelledRejectionError) {
      void rejected.catch(() => {});
      const event = new Event("unhandledrejection", { cancelable: true }) as PromiseRejectionEvent;
      Object.defineProperties(event, { reason: { value: reason }, promise: { value: rejected } });
      queueMicrotask(() => { window.dispatchEvent(event); unhandledEvents.push(event); });
    }
    return rejected;
  });
  setTransport({ call: (object, _method, _window, args: unknown) => {
    requests.push({ object, args });
    if (object === objectNames.CancelCall) return Promise.resolve(undefined);
    return new Promise((resolve, reject) => { finishNative = resolve; rejectNative = reject; });
  } });
  try {
    const controller = new AbortController();
    const first = callCancellableService(controller.signal, () => {
      const pending = Service.HistoricalOverview("2026-08-01", "current");
      abandoned = pending;
      return pending;
    });
    const rejected = expect(first).rejects.toMatchObject({ name: "AbortError" });
    controller.abort();
    await rejected;
    expect(requests).toHaveLength(2);
    expect(requests[0].object).toBe(objectNames.Call);
    expect(requests[1]).toEqual({ object: objectNames.CancelCall, args: { "call-id": (requests[0].args as Record<string, unknown>)["call-id"] } });
    // Abort wins on the frontend; the native error may already be in transit
    // before Go observes context cancellation and can complete quietly.
    if (completion === "resolve") finishNative({});
    else rejectNative(new Call.RuntimeError(JSON.stringify({ code: "unavailable", message: "native error already in transit" })));
    await new Promise(resolve => setTimeout(resolve, 0));
    if (completion === "reject") {
      expect(unhandledEvents).toHaveLength(1);
      expect(unhandledEvents[0].reason).toBeInstanceOf(CancelledRejectionError);
      expect(unhandledEvents[0].reason.promise).toBe(abandoned);
      expect(unhandledEvents[0].reason.cause).toBeInstanceOf(Call.RuntimeError);
      expect(unhandledEvents[0].defaultPrevented).toBe(true);
    } else expect(unhandledEvents).toHaveLength(0);
    const final = callCancellableService(new AbortController().signal, () => Service.HistoricalOverview("2026-08-02", ""));
    finishNative({ final: true });
    await expect(final).resolves.toEqual({ final: true });
    // A genuine live failure must retain the normal translated wire code.
    const live = callCancellableService(new AbortController().signal, () => Service.HistoricalOverview("2026-08-03", ""));
    const liveError = expect(live).rejects.toHaveProperty("wireError.code", "unavailable");
    rejectNative(new Call.RuntimeError(JSON.stringify({ code: "unavailable", message: "live native failure" })));
    await liveError;
    const dispatch = (reason: unknown) => {
      const event = new Event("unhandledrejection", { cancelable: true }) as PromiseRejectionEvent;
      Object.defineProperty(event, "reason", { value: reason });
      window.dispatchEvent(event);
      return event.defaultPrevented;
    };
    expect(dispatch(new Error("unrelated application failure"))).toBe(false);
    expect(dispatch(new CancelledRejectionError(CancellablePromise.resolve(), new Call.RuntimeError("other cancelled call")))).toBe(false);
    expect(dispatch(new CancelledRejectionError(abandoned, new TypeError("programming failure")))).toBe(false);
    expect(dispatch({ name: "CancelledRejectionError", promise: abandoned, cause: new Call.RuntimeError("lookalike") })).toBe(false);
    expect(dispatch(new CancelledRejectionError(abandoned, new Call.RuntimeError("cancellation callback failed"), "Unhandled exception in oncancelled callback."))).toBe(false);
    expect(dispatch(new CancelledRejectionError(abandoned, new Call.RuntimeError("cancellation callback rejected"), "Unhandled rejection in oncancelled callback."))).toBe(false);
    // Another actual runtime call cancelled outside the historical helper
    // must still expose its orphan rejection to normal browser reporting.
    const other = Service.InstrumentHoldings();
    await other.cancel();
    rejectNative(new Call.RuntimeError("unrelated cancelled native read"));
    await new Promise(resolve => setTimeout(resolve, 0));
    const unrelatedEvent = unhandledEvents[unhandledEvents.length - 1]!;
    expect(unrelatedEvent.reason.promise).toBe(other);
    expect(unrelatedEvent.defaultPrevented).toBe(false);
    expect(unhandledEvents).toHaveLength(completion === "reject" ? 2 : 1);
    expect(requests.filter(request => request.object === objectNames.CancelCall)).toHaveLength(2);
  } finally {
    setTransport(null);
    rejectionHost.mockRestore();
  }
});
