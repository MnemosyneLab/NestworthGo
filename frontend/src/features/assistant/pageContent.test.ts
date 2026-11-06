import { createElement } from "react";
import { render } from "@testing-library/react";
import { PageChrome, PageChromeProvider } from "@/components/layout/PageChrome";
import { DatePickerControl } from "@/components/ui/date-picker-control";
import { afterEach, expect, it } from "vitest";
import { capturePageContent } from "./pageContent";
afterEach(() => { document.body.innerHTML = ""; });
function fixture() {
  document.body.innerHTML = `<aside>assistant transcript secret</aside><main data-assistant-page="return-analysis"><h1>收益分析 Return analysis</h1><section data-assistant-module="filters"><label for="analysis-from">开始日期</label><input id="analysis-from" value="2026-10-01"><select id="analysis-scope"><option value="internal-uuid">Synthetic account</option><option value="other">All</option></select><input id="analysis-filter-instrument" role="combobox" value="Synthetic fund" aria-expanded="false"><button disabled aria-label="Reset">Reset</button><input id="analysis-cash" type="checkbox" checked><input id="analysis-to" value="2026-10-31" readonly aria-invalid="true"><input type="password" value="never-export"><input id="unknown" value="ordinary financial note"><textarea>ordinary financial textarea</textarea><div data-assistant-private>hidden key material</div></section><div data-assistant-module="trend"><div role="status">Loading…</div><details><summary>Data table</summary><table><tbody><tr><td>12.34 USD</td></tr></tbody></table></details><button aria-pressed="true">Selected view</button><div hidden>retained page</div><canvas></canvas></div></main>`;
}
it("exports scoped semantic text, labels, field properties and descriptive affordances without HTML or private values", () => {
  fixture(); const p = capturePageContent("return-analysis:"); const raw = JSON.stringify(p);
  expect(raw).toContain("ordinary financial note"); expect(raw).toContain("ordinary financial textarea"); expect(raw).toContain("收益分析"); expect(raw).toContain("12.34 USD"); expect(raw).toContain("Synthetic account");
  for (const secret of ["internal-uuid", "never-export", "hidden key material", "assistant transcript secret", "retained page", "<main"]) expect(raw).not.toContain(secret);
  expect(p.nodes.find(n => n.id === "filters/analysis-from")).toMatchObject({ name: "开始日期", value: "2026-10-01" });
  expect(p.nodes.find(n => n.id === "filters/analysis-filter-instrument")).toMatchObject({ value: "Synthetic fund", state: ["expanded=false"] });
  expect(p.nodes.find(n => n.name === "Reset")?.state).toContain("disabled=true");
  expect(p.nodes.find(n => n.role === "disclosure")?.state).toContain("expanded=false");
  expect(p.nodes.find(n=>n.id==="filters/analysis-cash")?.state).toContain("checked=true");
  expect(p.nodes.find(n=>n.id==="filters/analysis-to")?.state).toEqual(expect.arrayContaining(["readonly=true", "invalid=true"]));
  expect(p.limitations.length).toBeGreaterThan(0);
});
it("captures current input properties, filtering state and async results; same-state continuation uses one generation", () => {
  fixture(); const first = capturePageContent("return-analysis:");
  expect(capturePageContent("return-analysis:").id).toBe(first.id);
  (document.getElementById("analysis-from") as HTMLInputElement).value = "2026-10-02";
  const second = capturePageContent("return-analysis:"); expect(second.id).not.toBe(first.id);
  expect(second.nodes.find(n => n.id === "filters/analysis-from")?.value).toBe("2026-10-02");
  document.querySelector('[role="status"]')!.textContent = "No data";
  expect(capturePageContent("return-analysis:").id).not.toBe(second.id);
  expect(capturePageContent("accounts:")).toMatchObject({ page: "unsupported", nodes: [] });
  expect(capturePageContent("return-analysis:").id).not.toBe(second.id);
});
it("splits long text losslessly and rejects excessive content without advertising completeness", () => {
  fixture(); const paragraph = document.createElement("p"); paragraph.textContent = "中".repeat(399) + "🌏" + "中".repeat(1600); document.querySelector("main")!.append(paragraph);
  const p = capturePageContent("return-analysis:"); expect(p.nodes.filter(n => n.name.includes("中")).map(n => n.name).join("")).toBe(paragraph.textContent);
  paragraph.textContent = "x".repeat(400 * 8193); expect(() => capturePageContent("return-analysis:")).toThrow("page_content_budget");
});


it("reads the active page under the assistant modal but excludes retained hidden routes", () => {
 fixture(); const root=document.querySelector("main")!; root.setAttribute("inert", "");
 const page=capturePageContent("return-analysis:"); expect(page.page).toBe("return-analysis");
 expect(page.nodes[0].state).toContain("inert=true"); expect(page.limitations.join(" ")).toContain("overlay");
 root.setAttribute("hidden", ""); expect(capturePageContent("return-analysis:").page).toBe("unsupported");
});


it("revokes a snapshot after route return or root remount even when text and values match", () => {
 fixture(); const first=capturePageContent("return-analysis:","visit-one");
 expect(capturePageContent("return-analysis:","visit-two").id).not.toBe(first.id);
 const second=capturePageContent("return-analysis:","visit-two");
 fixture(); expect(capturePageContent("return-analysis:","visit-two").id).not.toBe(second.id);
});


it("captures the real shared date picker label, displayed value and popup state using a stable control ID", () => {
 render(createElement("main", {"data-assistant-page":"return-analysis"}, createElement("section", {"data-assistant-module":"filters"}, createElement("label",{htmlFor:"analysis-from"},"From date"), createElement(DatePickerControl,{id:"analysis-from",value:"2026-10-01",dateFormat:"iso",onChange:()=>{}}))));
 const date=capturePageContent("return-analysis:").nodes.find(n=>n.id==="filters/analysis-from");
 expect(date).toMatchObject({role:"button",name:"From date",value:"2026-10-01"});
 expect(date?.state).toContain("expanded=false");
});


it("protects persistent credentials while reading ordinary newly added fields", () => {
 document.body.innerHTML = '<main data-assistant-page="return-analysis"><section data-assistant-module="contribution"><input id="contribution-search" type="password" value="protected-password"><textarea data-assistant-private="credential" id="contribution-sort">protected-note</textarea><input id="contribution-secret" value="protected-new-field"></section></main>';
 const page=capturePageContent("return-analysis:");const raw=JSON.stringify(page);
 for(const secret of ["protected-password","protected-note"])expect(raw).not.toContain(secret);
 expect(JSON.stringify(page)).toContain("protected-new-field");
 expect(page.nodes.filter(n=>n.state.includes("redacted"))).toHaveLength(2);
});


it("reads public fields independently of ID prefixes and rejects hidden ancestors", () => {
 document.body.innerHTML = '<div hidden><main data-assistant-page="overview">retained overview</main></div><main data-assistant-page="accounts"><input id="analysis-from" value="not-reviewed-on-accounts"><label for="accounts-show-archived">Archived</label><input id="accounts-show-archived" type="checkbox" checked><div data-assistant-module="account-list">Synthetic bank</div></main>';
 expect(capturePageContent("overview:").page).toBe("unsupported");
 const page=capturePageContent("accounts:");
 expect(JSON.stringify(page)).toContain("not-reviewed-on-accounts");
 expect(page.nodes.find(n=>n.id==="page/accounts-show-archived")?.state).toContain("checked=true");
 expect(capturePageContent("settings:").page).toBe("unsupported");
});


it("captures the actual page toolbar portal without exporting app-shell or assistant content", () => {
 const toolbar=document.createElement("div"); document.body.append(toolbar);
 render(createElement(PageChromeProvider, { activePageId: "overview", target: toolbar, children:
  createElement("main", {"data-assistant-page":"overview"},
   createElement(PageChrome, {pageId:"overview",title:"Synthetic overview title",actions:createElement("button",null,"Open Wealth history")}),
   createElement(PageChrome, {pageId:"overview",actions:createElement("button",null,"Second toolbar action")}),
   createElement("p",null,"Synthetic body")) }));
 const unrelated=document.createElement("aside"); unrelated.textContent="private assistant conversation"; toolbar.append(unrelated);
 const page=capturePageContent("overview:");
 expect(page.nodes.find(n=>n.name==="Synthetic overview title")).toMatchObject({role:"heading",module:"page"});
 expect(page.nodes.find(n=>n.name==="Open Wealth history")).toMatchObject({role:"button",actions:["activate"]});
 expect(page.nodes.filter(n=>n.name==="Second toolbar action")).toHaveLength(1);
 expect(JSON.stringify(page)).not.toContain("private assistant conversation");
});


it("protects account credentials independently of field names and display formats", () => {
 document.body.innerHTML = '<main data-assistant-page="account" data-assistant-view="accounts:synthetic"><input id="account-composition-type" type="password" value="protected-password"><input id="account-composition-detail" type="hidden" value="protected-hidden"><textarea data-assistant-private="credential" id="account-composition-type">protected-note</textarea><input id="account-composition-future" value="protected-new-field"></main>';
 const page=capturePageContent("accounts:synthetic");
 for(const secret of ["protected-password","protected-hidden","protected-note"]) expect(JSON.stringify(page)).not.toContain(secret);
 expect(JSON.stringify(page)).toContain("protected-new-field");
 expect(page.nodes.filter(n=>n.state.includes("redacted"))).toHaveLength(2);
});


it.each([
 ["available-funds", "custom-horizon", "instrument-search"],
 ["portfolio", "instrument-search", "history-filter-account"],
 ["history", "history-filter-from", "liquidity-include-early"],
])("protects marked credentials while reading public controls on %s", (page,reviewed,other)=>{
 document.body.innerHTML=`<div><main data-assistant-page="${page}"><input id="${reviewed}" type="password" value="protected-password"><textarea data-assistant-private="credential" id="${reviewed}">protected-note</textarea><input id="${other}" value="protected-other-page"><input id="unknown-future" value="protected-new-field"></main></div>`;
 const content=capturePageContent(`${page}:`); const raw=JSON.stringify(content);
 expect(content.page).toBe(page);
 for(const value of ["protected-password","protected-note"]) expect(raw).not.toContain(value);
 expect(raw).toContain("protected-other-page"); expect(raw).toContain("protected-new-field");
 expect(content.nodes.filter(n=>n.state.includes("redacted"))).toHaveLength(2);
 document.querySelector("main")!.parentElement!.setAttribute("hidden","");
 expect(capturePageContent(`${page}:`).page).toBe("unsupported");
 document.body.removeAttribute("hidden");
});
