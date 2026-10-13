import { expect, it, vi } from "vitest";
import { CancellablePromise, objectNames, setTransport } from "@wailsio/runtime";
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

it("cancels the exact generated binding call through the installed runtime transport and tolerates its quiet late completion", async () => {
  const requests: Array<{ object: number; args: unknown }> = [];
  let finishNative!: (value: unknown) => void;
  setTransport({ call: (object, _method, _window, args: unknown) => {
    requests.push({ object, args });
    if (object === objectNames.CancelCall) return Promise.resolve(undefined);
    return new Promise(resolve => { finishNative = resolve; });
  } });
  try {
    const controller = new AbortController();
    const first = callCancellableService(controller.signal, () => Service.HistoricalOverview("2026-08-01", "current"));
    const rejected = expect(first).rejects.toMatchObject({ name: "AbortError" });
    controller.abort();
    await rejected;
    expect(requests).toHaveLength(2);
    expect(requests[0].object).toBe(objectNames.Call);
    expect(requests[1]).toEqual({ object: objectNames.CancelCall, args: { "call-id": (requests[0].args as Record<string, unknown>)["call-id"] } });
    // The IPC adapter returns an empty success for an abandoned context,
    // avoiding Wails beta.28's unhandled late native rejection behavior.
    finishNative({});
    await Promise.resolve();
    const final = callCancellableService(new AbortController().signal, () => Service.HistoricalOverview("2026-08-02", ""));
    finishNative({ final: true });
    await expect(final).resolves.toEqual({ final: true });
    expect(requests.filter(request => request.object === objectNames.CancelCall)).toHaveLength(1);
  } finally {
    setTransport(null);
  }
});
