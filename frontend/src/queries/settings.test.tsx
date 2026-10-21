import { describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { createTestQueryClient } from "@/test/queryClient";
import { Appearance, Language } from "../../bindings/github.com/waltwang/nestworth-go/internal/settings/models";
import { settingsQueryKey, useResetSettings, useSaveSettings, useSettings } from "./settings";

const service = vi.hoisted(() => ({ Load: vi.fn(), Save: vi.fn(), Reset: vi.fn() }));
vi.mock("../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/settings", () => ({ Service: service }));
vi.mock("@wailsio/runtime", () => ({ Call: { ByID: vi.fn(() => { throw new Error("Unexpected native call"); }) } }));
function deferred() {
  let resolve!: () => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<void>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

describe("shared settings patches", () => {
  it.each([false, true])("serializes independent hooks and continues after failure=%s without stale rollback", async (fail) => {
    const client = createTestQueryClient();
    let document = { appearance: "system", language: "en", accent: "mint" };
    service.Load.mockImplementation(async () => ({ ...document }));
    const first = deferred();
    service.Save.mockReset().mockImplementationOnce(async (value) => { await first.promise; document = value; })
      .mockImplementation(async (value) => { document = value; });
    client.setQueryData(settingsQueryKey, document);
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    const { result } = renderHook(() => [useSaveSettings(), useSaveSettings()], { wrapper });
    let a!: Promise<unknown>, b!: Promise<unknown>;
    await act(async () => {
      a = result.current[0].mutateAsync({ language: Language.LanguageZhCN }).catch(error => error);
      b = result.current[1].mutateAsync({ appearance: Appearance.AppearanceDark });
    });
    expect(service.Save).toHaveBeenCalledTimes(1);
    await act(async () => { if (fail) first.reject(new Error("synthetic failure")); else first.resolve(); await Promise.all([a, b]); });
    expect(service.Save).toHaveBeenLastCalledWith({ appearance: "dark", language: fail ? "en" : "zh-CN", accent: "mint" });
    expect(client.getQueryData(settingsQueryKey)).toEqual(document);
    expect(document.appearance).toBe("dark");
  });
  it.each(["save", "reset"] as const)("prevents a read started during %s from replacing the published result", async (operation) => {
    const client = createTestQueryClient();
    const old = { appearance: "dark", language: "en", accent: "mint" };
    const updated = { ...old, language: "zh-CN" };
    let document = old;
    const writeGate = deferred();
    const readGate = deferred();
    service.Load.mockReset().mockImplementation(async () => ({ ...document }));
    service.Save.mockReset().mockImplementation(async () => { await writeGate.promise; document = updated; });
    service.Reset.mockReset().mockImplementation(async () => { await writeGate.promise; document = updated; return updated; });
    client.setQueryData(settingsQueryKey, old);
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    const { result } = renderHook(() => ({ query: useSettings({ enabled: false }), save: useSaveSettings(), reset: useResetSettings() }), { wrapper });
    let write!: Promise<unknown>;
    await act(async () => {
      write = operation === "save" ? result.current.save.mutateAsync({ language: Language.LanguageZhCN }) : result.current.reset.mutateAsync();
    });
    await waitFor(() => expect(operation === "save" ? service.Save : service.Reset).toHaveBeenCalledOnce());
    // This query captures the old document after the write's initial cancel.
    service.Load.mockImplementationOnce(async () => { const captured = { ...document }; await readGate.promise; return captured; });
    let read!: ReturnType<typeof result.current.query.refetch>;
    await act(async () => { read = result.current.query.refetch(); });
    expect(client.isFetching({ queryKey: settingsQueryKey })).toBe(1);
    await act(async () => { writeGate.resolve(); await write; });
    expect(client.getQueryData(settingsQueryKey)).toEqual(updated);
    await act(async () => { readGate.resolve(); await read; });
    expect(client.getQueryData(settingsQueryKey)).toEqual(updated);
  });

});
