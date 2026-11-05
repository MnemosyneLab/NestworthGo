import { afterEach, expect, it } from "vitest";
import { capturePageContent } from "./pageContent";
afterEach(() => { document.body.innerHTML = ""; });
function fixture() {
  document.body.innerHTML = `<aside>assistant transcript secret</aside><main data-assistant-page="return-analysis"><h1>收益分析 Return analysis</h1><section data-assistant-module="filters"><label for="analysis-from">开始日期</label><input id="analysis-from" value="2026-10-01"><select id="analysis-scope"><option value="internal-uuid">Synthetic account</option><option value="other">All</option></select><input id="analysis-filter-instrument" role="combobox" value="Synthetic fund" aria-expanded="false"><button disabled aria-label="Reset">Reset</button><input type="password" value="never-export"><input id="unknown" value="private-notes"><textarea>private textarea</textarea><div data-assistant-private>hidden key material</div></section><div data-assistant-module="trend"><div role="status">Loading…</div><details><summary>Data table</summary><table><tbody><tr><td>12.34 USD</td></tr></tbody></table></details><button aria-pressed="true">Selected view</button><div hidden>retained page</div><canvas></canvas></div></main>`;
}
it("exports scoped semantic text, labels, field properties and descriptive affordances without HTML or private values", () => {
  fixture(); const p = capturePageContent("return-analysis:"); const raw = JSON.stringify(p);
  expect(raw).toContain("收益分析"); expect(raw).toContain("12.34 USD"); expect(raw).toContain("Synthetic account");
  for (const secret of ["internal-uuid", "never-export", "private-notes", "private textarea", "hidden key material", "assistant transcript secret", "retained page", "<main"]) expect(raw).not.toContain(secret);
  expect(p.nodes.find(n => n.id === "filters/analysis-from")).toMatchObject({ name: "开始日期", value: "2026-10-01" });
  expect(p.nodes.find(n => n.id === "filters/analysis-filter-instrument")).toMatchObject({ value: "Synthetic fund", state: ["expanded=false"] });
  expect(p.nodes.find(n => n.name === "Reset")?.state).toContain("disabled=true");
  expect(p.nodes.find(n => n.role === "disclosure")?.state).toContain("expanded=false");
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
  fixture(); const paragraph = document.createElement("p"); paragraph.textContent = "中".repeat(2000); document.querySelector("main")!.append(paragraph);
  const p = capturePageContent("return-analysis:"); expect(p.nodes.filter(n => n.name.includes("中")).map(n => n.name).join("")).toBe(paragraph.textContent);
  paragraph.textContent = "x".repeat(400 * 8193); expect(() => capturePageContent("return-analysis:")).toThrow("page_content_budget");
});
