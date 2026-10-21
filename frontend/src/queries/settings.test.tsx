import { describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { createTestQueryClient } from "@/test/queryClient";
import { Appearance, Language } from "../../bindings/github.com/waltwang/nestworth-go/internal/settings/models";
import { settingsQueryKey, useSaveSettings } from "./settings";

const service = vi.hoisted(() => ({ Load: vi.fn(), Save: vi.fn() }));
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
});
