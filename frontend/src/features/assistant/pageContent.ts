/** Explicitly opted-in page roots only. This is a semantic DOM adapter, not a
 * general DOM/HTML export. No attributes, URLs, raw option IDs or event handlers
 * cross IPC. Adding a page requires a review of its fields and private regions. */
export type SemanticNode = {
  id: string; parent: string; module: string; role: string; name: string;
  value: string; state: string[]; actions: string[];
};
export type SemanticPage = {
  id: string; page: string; capturedAt: string; nodes: SemanticNode[]; limitations: string[];
};
let lastEncoding = "";
let lastID = "";
const rootIdentities = new WeakMap<Element,string>();
// Explicit reviewed semantic controls, never a prefix-based permission for
// future inputs. IDs are owned by the real page/shared control triggers.
const allowedFields = new Set([
  "analysis-scope", "analysis-scope-id", "analysis-from", "analysis-to",
  "analysis-valuation", "analysis-cash", "analysis-filter-account",
  "analysis-filter-currency", "analysis-filter-asset-class", "analysis-filter-member",
  "analysis-filter-instrument", "return-trend-display",
  "contribution-return-type", "contribution-group-by", "contribution-sort",
  "contribution-limit", "contribution-search",
]);
const roles: Record<string, string> = {
  BUTTON: "button", A: "link", SELECT: "combobox", OPTION: "option",
  INPUT: "textbox", TEXTAREA: "textbox", H1: "heading", H2: "heading", H3: "heading", H4: "heading",
  TABLE: "table", TR: "row", TD: "cell", TH: "columnheader", SUMMARY: "disclosure", IMG: "img",
};
const excluded = 'script,style,svg,canvas,[hidden],[inert],[aria-hidden="true"],[data-assistant-private]';
const semanticRoles = new Set(["region", "heading", "button", "link", "textbox", "combobox", "checkbox", "radio", "switch", "tab", "tablist", "option", "table", "row", "cell", "columnheader", "status", "alert", "group", "img", "slider", "spinbutton"]);
export function capturePageContent(viewKey: string, generation = viewKey): SemanticPage {
  const limitations: string[] = [];
  const nodes: SemanticNode[] = [];
  const root = viewKey === "return-analysis:" ? document.querySelector<HTMLElement>('[data-assistant-page="return-analysis"]') : null;
  // A modal assistant can make the underlying active page inert. Read its
  // reviewed content while describing the overlay; retained hidden routes stay out.
  const page = root && !root.closest('[hidden]') && getComputedStyle(root).display !== "none" ? "return-analysis" : "unsupported";
  if (page === "unsupported") limitations.push("This page is not supported. Only the active return-analysis page is currently available.");
  else {
    limitations.push("Semantic content of the active analysis tab only. Canvas/SVG geometry, hover-only tooltips and portal popovers are not exported. Collapsed disclosures retain their state and available semantic contents. No arbitrary UI control.");
    const overlay = !!root?.closest('[inert], [aria-hidden="true"]');
    if (overlay) limitations.push("The active page is blocked by an overlay; its controls are descriptive only.");
    const used = new Set<string>();
    const add = (n: Omit<SemanticNode, "id">, preferred?: string) => {
      if (nodes.length >= 8192) throw new Error("page_content_budget");
      let id = preferred || `${n.module}/n${nodes.length}`;
      if (used.has(id)) id = `${id}/${nodes.length}`;
      used.add(id); nodes.push({ id, ...n }); return id;
    };
    const text = (value: string, parent: string, module: string) => {
      const codepoints = Array.from(value.replace(/\s+/g, " ").trim());
      for (let i = 0; i < codepoints.length; i += 400) add({ parent, module, role: "text", name: codepoints.slice(i, i + 400).join(""), value: "", state: [], actions: [] });
    };
    const visibleText = (node: Node): string => {
      if (node.nodeType === Node.TEXT_NODE) return node.textContent || "";
      if (!(node instanceof Element) || node.matches(`${excluded},input,textarea,select`)) return "";
      if (getComputedStyle(node).display === "none" || getComputedStyle(node).visibility === "hidden") return "";
      return Array.from(node.childNodes).map(visibleText).join("");
    };
    const walk = (el: Element, parent: string, module: string) => {
      if (el !== root && el.matches(excluded)) return;
      if (getComputedStyle(el).display === "none" || getComputedStyle(el).visibility === "hidden") return;
      module = el.getAttribute("data-assistant-module") || module;
      const roleAttribute = el.getAttribute("role") || "";
      let role = semanticRoles.has(roleAttribute) ? roleAttribute : roles[el.tagName];
      if (el.hasAttribute("data-assistant-module") || el === root) role = "region";
      // Busy containers are semantic even without an explicit ARIA role. Keep
      // both true and false so background query transitions fence old captures.
      if (!role && el.hasAttribute("aria-busy")) role = "group";
      const field = el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement || el instanceof HTMLSelectElement;
      const fieldButton = el instanceof HTMLButtonElement && allowedFields.has(el.id);
      const labelledControl = field || fieldButton;
      const sensitive = field && (!allowedFields.has(el.id) || el instanceof HTMLTextAreaElement || (el instanceof HTMLInputElement && ["password", "hidden", "file"].includes(el.type)));
      if (sensitive) { add({ parent, module, role: "textbox", name: "Protected input", value: "", state: ["redacted"], actions: [] }); return; }
      if (el instanceof HTMLInputElement && ["checkbox", "radio"].includes(el.type)) role = el.type;
      let nextParent = parent;
      if (role) {
        const state: string[] = overlay ? ["inert=true"] : [];
        for (const name of ["expanded", "selected", "checked", "pressed", "invalid", "busy", "readonly", "disabled"]) {
          const value = el.getAttribute(`aria-${name}`); if (value !== null) state.push(`${name}=${value}`);
        }
        if (el.matches(":disabled")) state.push("disabled=true");
        if (el instanceof HTMLInputElement) { if (el.type === "checkbox" || el.type === "radio") state.push(`checked=${el.checked}`); if (el.readOnly) state.push("readonly=true"); }
        if (el instanceof HTMLOptionElement) {state.push(`selected=${el.selected}`); if (el.disabled) state.push("disabled=true");}
        if (el.tagName === "SUMMARY") state.push(`expanded=${(el.parentElement as HTMLDetailsElement).open}`);
        let name = el.getAttribute("aria-label") || (labelledControl ? Array.from((el as HTMLInputElement | HTMLButtonElement | HTMLSelectElement).labels || []).filter(label => root!.contains(label)).map(visibleText).join(" ") : "");
        if (!name && el.hasAttribute("aria-labelledby")) {
          name = (el.getAttribute("aria-labelledby") || "").split(/\s+/).map(id => document.getElementById(id)).filter((label): label is HTMLElement => !!label && !!root?.contains(label)).map(visibleText).join(" ");
        }
        const textNamed = ["button", "link", "tab", "option", "heading", "disclosure", "cell", "columnheader"].includes(role);
        if (!name && textNamed) name = visibleText(el).replace(/\s+/g, " ").trim();
        if (!name && el.tagName === "IMG") name = el.getAttribute("alt") || "";
        if (!name && (role === "region")) name = module;
        if (name.length > 400) { limitations.push("An accessible label was shortened; its remaining text is unavailable."); name = name.slice(0, 400); }
        const value = el instanceof HTMLSelectElement ? Array.from(el.selectedOptions).map(o => o.label).join(", ") : el instanceof HTMLInputElement ? el.value : fieldButton ? visibleText(el).trim() : "";
        if (value.length > 2048) throw new Error("page_content_budget");
        const actions = ["button", "link", "disclosure", "tab"].includes(role) ? ["activate (description only)"] : field ? ["edit/select (description only)"] : [];
        nextParent = add({ parent, module, role, name: name.trim(), value, state, actions }, labelledControl && el.id ? `${module}/${el.id}` : el.hasAttribute("data-assistant-module") ? module : undefined);
      }
      for (const child of el.childNodes) {
        if (child.nodeType === Node.TEXT_NODE) {
          if (!role || !["button", "link", "tab", "option", "heading", "disclosure", "cell", "columnheader"].includes(role)) text(child.textContent || "", nextParent, module);
        }
        else if (child instanceof Element) walk(child, nextParent, module);
      }
    };
    walk(root!, "", "page");
  }
  if (nodes.length > 8192) throw new Error("page_content_budget");
  if (root && !rootIdentities.has(root)) rootIdentities.set(root,crypto.randomUUID());
  const encoding = JSON.stringify({ page, nodes, limitations, generation, rootIdentity: root ? rootIdentities.get(root) : "" });
  if (new TextEncoder().encode(encoding).length > 512 * 1024) throw new Error("page_content_budget");
  if (encoding !== lastEncoding) { lastEncoding = encoding; lastID = crypto.randomUUID(); }
  return { id: lastID, page, capturedAt: new Date().toISOString(), nodes, limitations };
}
