import { BusinessPageRoot } from "@/lib/businessCapture";
import { capturePageContent } from "@/features/assistant/pageContent";
import { beforeEach, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import { AIModelConfig } from "./AIModelConfig";
import type * as API from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service";
const configure = vi.hoisted(() => vi.fn());
vi.mock(
  "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service",
  () => ({ Configure: configure }),
);
beforeEach(() => {
  vi.clearAllMocks();
  configure.mockResolvedValue(undefined);
});
function mount(credentials = "present", enabled = false) {
  const status = {
    epoch: "host",
    revision: 4,
    credentials,
    config: { provider: "openai-responses", model: "mock", enabled },
  } as Awaited<ReturnType<typeof API.Status>>;
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <BusinessPageRoot view="settings:" accessIdentity="synthetic"><AIModelConfig status={status} unavailable={false} /></BusinessPageRoot>
    </QueryClientProvider>,
  );
}
it("requires explicit desktop opt-in and submits no credential material", async () => {
  const user = userEvent.setup();
  mount();
  expect(screen.getByRole("checkbox")).not.toBeChecked();
  await user.click(
    screen.getByLabelText("Enable desktop model requests"),
  );
  await user.clear(screen.getByLabelText("Model ID"));
  await user.type(screen.getByLabelText("Model ID"), "test-model");
  await user.click(
    screen.getByRole("button", { name: "Save model configuration" }),
  );
  await waitFor(() =>
    expect(configure).toHaveBeenCalledWith("host", 4, {
      provider: "openai-responses",
      model: "test-model",
      enabled: true,
    }),
  );
});
it("leaves activation unavailable without a key and allows explicit shutdown", async () => {
  const user = userEvent.setup();
  const view = mount("missing");
  expect(screen.getByRole("checkbox")).toBeDisabled();
  view.unmount();
  mount("present", true);
  await user.click(
    screen.getByRole("button", { name: "Disable AI and stop runs" }),
  );
  await waitFor(() =>
    expect(configure).toHaveBeenCalledWith("host", 4, {
      provider: "openai-responses",
      model: "mock",
      enabled: false,
    }),
  );
});

it("keeps model configuration readable without granting generic edit or activation targets",()=>{
 mount();const page=capturePageContent("settings:");
 for(const name of ["Model ID","Enable desktop model requests","Save model configuration"]){const node=page.nodes.find(n=>n.role!=="text" && n.name===name)!;expect(node).toBeDefined();expect(node.targetRef).toBeUndefined();expect(node.actions).toEqual([]);expect(node.state).toContain("operation_protected=true");}
 expect(JSON.stringify(page)).toContain("mock");expect(configure).not.toHaveBeenCalled();
});
