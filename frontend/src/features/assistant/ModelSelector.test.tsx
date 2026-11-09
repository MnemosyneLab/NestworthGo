import { expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/i18n";
import { ModelSelector } from "./ModelSelector";
import { effectiveEffort, selectableEfforts } from "./modelSelection";
const models=["gpt-6-sol","gpt-6.1-sol","custom"];
const catalog=[{id:models[0],efforts:["none","high","max"]},{id:models[1],efforts:["high","max"]}];
it.each(["en","zh-CN","zh-TW"])("offers exact effort options and explicit model changes in %s",async locale=>{
 await i18n.changeLanguage(locale);const user=userEvent.setup();const onChange=vi.fn();const view=render(<ModelSelector models={models} catalog={catalog} selection={{model:models[0],reasoningEffort:"max"}} disabled={false} onChange={onChange}/>);
 const trigger=()=>screen.getByRole("button",{name:new RegExp(`^${i18n.t("aiChat.model")}: `)});
 expect(trigger()).toHaveAccessibleName(`${i18n.t("aiChat.model")}: ${models[0]} · ${i18n.t("aiModels.reasoning")}: ${i18n.t("aiModels.efforts.max")}`);
 await user.click(trigger());
 expect(await screen.findByRole("button",{name:models[0]})).toHaveAttribute("aria-pressed","true");
 await user.click(screen.getByRole("button",{name:models[1]}));expect(onChange).toHaveBeenCalledWith({model:models[1],reasoningEffort:"high"});
 view.rerender(<ModelSelector models={models} catalog={catalog} selection={{model:models[1],reasoningEffort:""}} disabled={false} onChange={onChange}/>);
 expect(trigger()).toHaveAccessibleName(`${i18n.t("aiChat.model")}: ${models[1]} · ${i18n.t("aiModels.reasoning")}: ${i18n.t("aiModels.efforts.high")}`);
 expect(screen.queryByRole("button",{name:i18n.t("aiModels.efforts.none")})).not.toBeInTheDocument();
 expect(screen.queryByRole("button",{name:i18n.t("aiModels.default")})).not.toBeInTheDocument();
 expect(screen.getByRole("button",{name:i18n.t("aiModels.efforts.high")})).toHaveAttribute("aria-pressed","true");
 await user.click(screen.getByRole("button",{name:i18n.t("aiModels.efforts.max")}));expect(onChange).toHaveBeenLastCalledWith({model:models[1],reasoningEffort:"max"});
 await waitFor(()=>expect(screen.queryByRole("button",{name:i18n.t("aiModels.efforts.max")})).not.toBeInTheDocument());
});
it("retains removed choices and disables unknown effort and busy controls",async()=>{
 await i18n.changeLanguage("en");const user=userEvent.setup();const change=vi.fn();const view=render(<ModelSelector models={models} catalog={catalog} selection={{model:"removed",reasoningEffort:""}} disabled={false} onChange={change}/>);
 expect(screen.getByRole("button",{name:"Model ID: removed"})).toBeEnabled();expect(screen.getByRole("status")).toHaveTextContent("no longer enabled");expect(change).not.toHaveBeenCalled();
 await user.click(screen.getByRole("button",{name:"Model ID: removed"}));
 expect(await screen.findByRole("button",{name:"removed"})).toBeDisabled();expect(screen.queryByRole("group",{name:"Reasoning effort"})).not.toBeInTheDocument();
 view.rerender(<ModelSelector models={models} catalog={catalog} selection={{model:"custom",reasoningEffort:""}} disabled onChange={change}/>);expect(screen.getByRole("button",{name:"Model ID: custom"})).toBeDisabled();
});
it("defaults an unset or none effort to low and never offers none",()=>{
 expect(effectiveEffort(["none","low","high"],"")).toBe("low");expect(effectiveEffort(["none","low","high"],"none")).toBe("low");
 expect(effectiveEffort(["high","max"],"")).toBe("high");expect(effectiveEffort(["none","low"],"low")).toBe("low");
 expect(effectiveEffort(null,"")).toBe("");expect(selectableEfforts(["none","low"])).toEqual(["low"]);
});
