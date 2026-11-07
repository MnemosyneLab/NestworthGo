import { BusinessPageRoot } from "@/lib/businessCapture";
import { capturePageContent } from "@/features/assistant/pageContent";
import { beforeEach, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { createTestQueryClient } from "@/test/queryClient";
import i18n from "@/i18n";
import { AIModelConfig } from "./AIModelConfig";
import type * as API from "../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service";
const configure = vi.hoisted(() => vi.fn());
const models = vi.hoisted(() => vi.fn());
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service", () => ({ Configure: configure, Models: models }));
beforeEach(async () => { vi.clearAllMocks(); configure.mockResolvedValue(undefined); await i18n.changeLanguage("en"); });
function mount(credentials = "present", enabled = false) {
 const status = { epoch:"host",revision:4,credentials,config:{provider:"openai-responses",model:"legacy",enabled},modelCatalog:[{id:"gpt-6-sol",efforts:["none","high","max"]},{id:"gpt-6.1-sol",efforts:["high","max"]}] } as Awaited<ReturnType<typeof API.Status>>;
 return render(<QueryClientProvider client={createTestQueryClient()}><BusinessPageRoot view="settings:" accessIdentity="synthetic"><AIModelConfig status={status} unavailable={false}/></BusinessPageRoot></QueryClientProvider>);
}
it.each(["en","zh-CN","zh-TW"])("selects several offline models and saves in %s",async locale=>{
 await i18n.changeLanguage(locale);const user=userEvent.setup();mount();
 await user.click(screen.getByLabelText("gpt-6-sol"));await user.click(screen.getByLabelText("gpt-6.1-sol"));await user.click(screen.getByLabelText("legacy"));
 await user.click(screen.getByLabelText(i18n.t("aiChat.enable")));
 await user.click(screen.getByRole("button",{name:i18n.t("aiChat.saveConfig")}));
 await waitFor(()=>expect(configure).toHaveBeenCalledWith("host",4,{provider:"openai-responses",model:"gpt-6-sol",enabledModels:["gpt-6-sol","gpt-6.1-sol"],enabled:true}));
 expect(models).not.toHaveBeenCalled();expect(screen.queryByText("gpt-4.1")).not.toBeInTheDocument();
});
it("keeps legacy separately and supports explicit custom entry without listing",async()=>{
 const user=userEvent.setup();mount();await user.type(screen.getByLabelText("Model ID"),"custom-model");await user.click(screen.getByRole("button",{name:"Add model"}));
 await user.click(screen.getByRole("button",{name:"Save model configuration"}));
 expect(configure).toHaveBeenCalledWith("host",4,expect.objectContaining({enabledModels:["legacy","custom-model"]}));expect(models).not.toHaveBeenCalled();
});
it("requires at least one model and leaves activation unavailable without credentials",async()=>{
 const user=userEvent.setup();mount("missing");expect(screen.getByLabelText("Enable desktop model requests")).toBeDisabled();await user.click(screen.getByLabelText("legacy"));expect(screen.getByRole("button",{name:"Save model configuration"})).toBeDisabled();
});
it("preserves existing configuration on explicit disable",async()=>{
 const user=userEvent.setup();mount("present",true);await user.click(screen.getByRole("button",{name:"Disable AI and stop runs"}));expect(configure).toHaveBeenCalledWith("host",4,{provider:"openai-responses",model:"legacy",enabled:false});
});
it("protects model administration from generic page operations",()=>{
 mount();const page=capturePageContent("settings:");
 for(const name of ["Model ID","gpt-6-sol","Enable desktop model requests","Save model configuration"]){const node=page.nodes.find(n=>n.role!=="text"&&n.name===name)!;expect(node).toBeDefined();expect(node.targetRef).toBeUndefined();expect(node.actions).toEqual([]);expect(node.state).toContain("operation_protected=true");}
});
