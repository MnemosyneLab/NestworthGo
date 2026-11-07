import { expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/i18n";
import { ModelSelector } from "./ModelSelector";
const models=["gpt-6-sol","gpt-6.1-sol","custom"];
const catalog=[{id:models[0],efforts:["none","high","max"]},{id:models[1],efforts:["high","max"]}];
it.each(["en","zh-CN","zh-TW"])("offers exact effort options and explicit model changes in %s",async locale=>{
 await i18n.changeLanguage(locale);const user=userEvent.setup();const onChange=vi.fn();const view=render(<ModelSelector models={models} catalog={catalog} selection={{model:models[0],reasoningEffort:"max"}} disabled={false} onChange={onChange}/>);
 await user.selectOptions(screen.getByLabelText(i18n.t("aiChat.model")),models[1]);expect(onChange).toHaveBeenCalledWith({model:models[1],reasoningEffort:""});
 view.rerender(<ModelSelector models={models} catalog={catalog} selection={{model:models[1],reasoningEffort:""}} disabled={false} onChange={onChange}/>);
 expect(screen.queryByRole("option",{name:i18n.t("aiModels.efforts.none")})).not.toBeInTheDocument();await user.selectOptions(screen.getByLabelText(i18n.t("aiModels.reasoning")),"max");expect(onChange).toHaveBeenLastCalledWith({model:models[1],reasoningEffort:"max"});
});
it("retains removed choices and disables unknown effort and busy controls",async()=>{
 await i18n.changeLanguage("en");const change=vi.fn();const view=render(<ModelSelector models={models} catalog={catalog} selection={{model:"removed",reasoningEffort:""}} disabled={false} onChange={change}/>);
 expect(screen.getByLabelText("Model ID")).toHaveValue("removed");expect(screen.getByRole("status")).toHaveTextContent("no longer enabled");expect(change).not.toHaveBeenCalled();
 view.rerender(<ModelSelector models={models} catalog={catalog} selection={{model:"custom",reasoningEffort:""}} disabled onChange={change}/>);expect(screen.getByLabelText("Reasoning effort")).toBeDisabled();expect(screen.getByLabelText("Model ID")).toBeDisabled();
});
