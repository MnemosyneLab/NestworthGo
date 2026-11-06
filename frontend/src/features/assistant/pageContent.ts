/** Semantic presentation only: app-owned business roots and their published portals.
 * Page names/field IDs are not read permissions. Secret boundaries are persistent. */
export type SemanticNode = { id:string; parent:string; module:string; role:string; name:string; value:string; state:string[]; actions:string[]; targetRef?:string };
export type SemanticPage = { id:string; page:string; capturedAt:string; nodes:SemanticNode[]; limitations:string[] };
let lastEncoding="", lastID="";
const controlIdentities=new WeakMap<Element,string>();
let capturedControls=new Map<string,HTMLElement>();
export function capturedPageControl(ref:string) { return capturedControls.get(ref); }
export function controlOperations(el:Element):string[] {
 if(el instanceof HTMLInputElement){
  if(["checkbox","radio"].includes(el.type)) return ["activate"];
  if(["text","search","email","tel","url","number","date","time","datetime-local","month","week","range"].includes(el.type)) return ["set_value"];
  return [];
 }
 if(el instanceof HTMLTextAreaElement) return ["set_value"];
 if(el instanceof HTMLSelectElement && !el.multiple) return ["select_option"];
 if(el instanceof HTMLButtonElement || el.tagName==="SUMMARY" || ["tab","option","menuitem","menuitemcheckbox","menuitemradio","checkbox","radio","switch"].includes(el.getAttribute("role") || "")) return ["activate"];
 // Links can launch external URLs/native integrations; retain controlled navigation.
 return [];
}
const rootIdentities=new WeakMap<Element,string>();
// Only public, truncated fields enter this weak cache. Compare the complete live
// property so edits outside the exported preview still revoke old snapshots.
const fieldVersions=new WeakMap<Element,{value:string;revision:string}>();
function fieldPreview(value:string){
 let bytes=0,units=0,preview="";
 for(const point of value){const code=point.codePointAt(0)!;const size=code<=0x7f ? 1 : code<=0x7ff ? 2 : code<=0xffff ? 3 : 4;if(bytes+size>2048)break;preview+=point;bytes+=size;units+=point.length;}
 return {value:preview,truncated:units<value.length};
}
const roles:Record<string,string>={BUTTON:"button",A:"link",SELECT:"combobox",OPTION:"option",INPUT:"textbox",TEXTAREA:"textbox",H1:"heading",H2:"heading",H3:"heading",H4:"heading",H5:"heading",H6:"heading",TABLE:"table",TR:"row",TD:"cell",TH:"columnheader",SUMMARY:"disclosure",IMG:"img",SECTION:"region",ARTICLE:"article",FORM:"form",NAV:"navigation",UL:"list",OL:"list",LI:"listitem",DL:"list",DT:"term",DD:"definition"};
const semanticRoles=new Set([...Object.values(roles),"tab","tablist","tabpanel","status","alert","group","slider","spinbutton","checkbox","radio","radiogroup","switch","dialog","alertdialog","menu","menuitem","menuitemcheckbox","menuitemradio","tooltip","progressbar","separator","tree","treeitem","grid","rowheader","listbox"]);
const excluded='script,style,svg,canvas,[hidden],[aria-hidden="true"],[data-assistant-origin]';
const namedText=new Set(["button","link","tab","option","heading","disclosure","cell","columnheader","rowheader","menuitem","menuitemcheckbox","menuitemradio","term","definition"]);
const privateNode=(el:Element)=>!!el.closest('[data-assistant-private]');
function visibleWithin(el:Element,boundary:Element,styles:WeakMap<Element,CSSStyleDeclaration>) {
 if(privateNode(el)) return false;
 for(let current:Element|null=el;current;current=current.parentElement){
  const root=current===boundary;
  const occluded=root && current.getAttribute("data-assistant-occluded")==="assistant";
  const css=styles.get(current) || getComputedStyle(current); styles.set(current,css);
  if(!occluded && (current.hasAttribute("hidden") || (!root && current.getAttribute("aria-hidden")==="true") || css.display==="none" || css.visibility==="hidden")) return false;
  if(root) return true;
 }
 return false;
}
export function capturePageContent(viewKey:string,generation=viewKey):SemanticPage {
 const nodes:SemanticNode[]=[];
 const controls=new Map<string,HTMLElement>();
 const styles=new WeakMap<Element,CSSStyleDeclaration>();
 const visibility=new WeakMap<Element,WeakMap<Element,boolean>>();
 const visible=(el:Element,boundary:Element)=>{
  let cache=visibility.get(boundary); if(!cache){cache=new WeakMap();visibility.set(boundary,cache);}
  const found=cache.get(el);if(found!==undefined)return found;
  const result=visibleWithin(el,boundary,styles);cache.set(el,result);return result;
 };
 const limits=new Set<string>();
 const valueRevisions:string[]=[];
 const businessRoots=Array.from(document.querySelectorAll<HTMLElement>('[data-assistant-root="business"]'));
 // Legacy explicitly marked component roots remain useful in isolated consumers;
 // the unified shell root takes precedence and cannot fall back on identity mismatch.
 const roots=businessRoots.length ? businessRoots : Array.from(document.querySelectorAll<HTMLElement>('[data-assistant-page]'));
 const [route,object=""]=viewKey.split(":");
 const matching=roots.filter(el=>el.dataset.assistantView ? el.dataset.assistantView===viewKey : !object && el.dataset.assistantPage===route);
 const root=matching.length===1 ? matching[0] : undefined;
 const ancestorHidden=root && Array.from((function*(){for(let parent=root.parentElement;parent;parent=parent.parentElement)yield parent;})()).some(parent=>{const css=styles.get(parent) || getComputedStyle(parent);styles.set(parent,css);return parent.hasAttribute("hidden") || css.display==="none" || css.visibility==="hidden";});
 const available=!!root && !ancestorHidden && visible(root,root);
 const page=available ? root!.dataset.assistantPage || route : "unsupported";
 if(!available) limits.add("No current app-owned business root matches the current view/object access identity.");
 else {
  limits.add("Mounted semantic content only; no HTML, URL attributes or execution authority. Hidden routes/tabs, secret regions, unmounted virtual rows and unloaded business pages are excluded. Node pagination does not load business records.");
  limits.add("Only app-owned published portals are included. Custom/unowned popups, canvas/SVG geometry and hover-only chart content may be unavailable; mounted semantic chart tables remain readable. Descriptive actions grant no clicks, navigation or financial writes.");
  if(root!.closest('[inert],[aria-hidden="true"]')) limits.add("The business surface is blocked by an overlay; controls are descriptive, not proof of current operability.");
  if(root!.dataset.assistantOccluded==="assistant") limits.add("The current business surface is occluded by the assistant; its underlying state remains readable, not proof of visible or operable controls.");
  const exposedAncestors=(el:Element,aria=true)=>{
   for(let current:Element|null=el;current;current=current.parentElement){
    const css=styles.get(current) || getComputedStyle(current);styles.set(current,css);
    if(privateNode(current) || current.hasAttribute("hidden") || aria && current.getAttribute("aria-hidden")==="true" || css.display==="none" || css.visibility==="hidden") return false;
   }
   return true;
  };
  const owner=root!.dataset.assistantOwner;
  const portals=Array.from(document.querySelectorAll<HTMLElement>('[data-assistant-portal]')).filter(el=>owner && el.dataset.assistantOwner===owner);
  if(portals.length>64) throw new Error("page_content_budget");
  const ownedBoundaries:Element[]=[root!];
  const origins=new Map<string,HTMLElement|null>();
  for(const marker of document.querySelectorAll<HTMLElement>('[data-assistant-origin]')) if(marker.dataset.assistantOwner===owner){const key=marker.dataset.assistantOrigin!;origins.set(key,origins.has(key) ? null : marker);}
  const pending=new Set(portals);
  // Nested popovers may originate inside a sheet. Establish provenance in order;
  // hidden retained tabs/routes and private ancestors cannot publish a portal.
  for(let depth=0;depth<16 && pending.size;depth++){
   let added=false;
   for(const portal of pending){
    const origin=origins.get(portal.dataset.assistantPortal || "");
    const source=origin?.parentElement;
    if(source && ownedBoundaries.some(boundary=>visible(source,boundary)) && visible(portal,portal) && exposedAncestors(portal,false)) {ownedBoundaries.push(portal);pending.delete(portal);added=true;}
   }
   if(!added) break;
  }
  // Ownership can survive a parent's ARIA modal occlusion so nested dialogs can
  // prove their source, but only exposed destinations are read or resolve names.
  const boundaries:Element[]=[root!,...ownedBoundaries.slice(1).filter(portal=>exposedAncestors(portal))];
  // Component-only toolbar publishers predate the unified root. In the real shell
  // all publishers carry the ownership/origin metadata above, including toolbars.
  if(!owner){
   const chromePage=page==="account" ? "accounts" : page;
   for(const chrome of document.querySelectorAll<HTMLElement>('[data-assistant-chrome]')) if(chrome.dataset.assistantChrome===chromePage && visible(chrome,chrome) && !chrome.closest('[hidden]') && !root!.contains(chrome) && exposedAncestors(chrome)) boundaries.push(chrome);
  }
  const safe=(el:Element)=>boundaries.some(boundary=>visible(el,boundary));
  const used=new Set<string>(); let elements=0,moduleNumber=0;
  const add=(node:Omit<SemanticNode,"id">,preferred?:string)=>{
   if(nodes.length>=8192) throw new Error("page_content_budget");
   let id=preferred || `${node.module}/n${nodes.length}`; if(used.has(id)) id=`${id}/${nodes.length}`;
   used.add(id); nodes.push({id,...node}); return id;
  };
  const text=(value:string,parent:string,module:string)=>{
   if(value.length>512*1024) throw new Error("page_content_budget");
   const points=Array.from(value.replace(/\s+/g," ").trim());
   for(let i=0;i<points.length;i+=400) add({parent,module,role:"text",name:points.slice(i,i+400).join(""),value:"",state:[],actions:[]});
  };
  const visibleText=(node:Node,depth=0):string=>{
   if(depth>128) throw new Error("page_content_budget");
   if(node.nodeType===Node.TEXT_NODE) return node.parentElement && safe(node.parentElement) ? (node.textContent || "").slice(0,2049) : "";
   if(!(node instanceof Element) || !safe(node) || node.matches(`${excluded},input,textarea,select`)) return "";
   let result="";
   for(const child of node.childNodes){result+=visibleText(child,depth+1);if(result.length>2048) break;}
   return result.slice(0,2049);
  };
  const nameOf=(el:Element,field:boolean)=>{
   let name=el.getAttribute("aria-label") || "";
   if(!name && field) name=Array.from((el as HTMLInputElement).labels || []).filter(safe).map(label=>visibleText(label)).join(" ");
   if(!name && el.hasAttribute("aria-labelledby")) name=(el.getAttribute("aria-labelledby") || "").split(/\s+/).map(id=>document.getElementById(id)).filter((label):label is HTMLElement=>!!label && safe(label)).map(label=>visibleText(label)).join(" ");
   return name;
  };
  const walk=(el:Element,parent:string,module:string,depth=0)=>{
   if(++elements>32768 || depth>128) throw new Error("page_content_budget");
   if(privateNode(el)){add({parent,module,role:"group",name:"Protected credential region",value:"",state:["redacted"],actions:[]});return;}
   if(el!==root && el.matches(excluded) || !safe(el)) return;
   const roleAttribute=el.getAttribute("role") || "";
   let role=semanticRoles.has(roleAttribute) ? roleAttribute : roles[el.tagName];
   const card=el.getAttribute("data-slot")==="card";
   if(card && !role) role="region";
   const hint=el.getAttribute("data-assistant-module");
   if(el===root){module="page";role="region";}
   else if(hint && /^[a-z][a-z0-9/-]{0,119}$/.test(hint)) {module=hint;role ||= "region";}
   else if(["region","article","form","dialog","alertdialog","tabpanel","navigation"].includes(role)) {
    const heading=card ? Array.from(el.querySelectorAll("h1,h2,h3,h4,h5,h6")).find(child=>child.closest('[data-slot="card"]')===el && safe(child)) : Array.from(el.children).find(child=>/^H[1-6]$/.test(child.tagName));
    const label=nameOf(el,false) || (heading ? visibleText(heading) : "");
    const suffix=Array.from(label.replace(/[^\p{L}\p{N}-]+/gu,"-").replace(/^-|-$/g,"")).slice(0,24).join("");
    module=`section-${++moduleNumber}${suffix ? `-${suffix}` : ""}`;
   }
   if(!role && el.hasAttribute("aria-busy")) role="group";
   const field=el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement || el instanceof HTMLSelectElement;
   const fieldButton=el instanceof HTMLButtonElement && el.hasAttribute("data-assistant-control");
   const labelled=field || el instanceof HTMLButtonElement;
   if(el instanceof HTMLInputElement && ["password","hidden","file"].includes(el.type)){add({parent,module,role:"textbox",name:"Protected input",value:"",state:["redacted"],actions:[]});return;}
   if(el instanceof HTMLInputElement){if(["checkbox","radio"].includes(el.type)) role=el.type;else if(el.type==="range") role="slider";else if(el.type==="number") role="spinbutton";}
   let nextParent=parent;
   if(role){
    const state=new Set<string>();
    if(el.closest('[inert]') || el.closest('[aria-hidden="true"]') || root!.dataset.assistantOccluded==="assistant" && root!.contains(el)) state.add("inert=true");
    for(const key of ["expanded","selected","checked","pressed","invalid","busy","readonly","disabled","sort","current","valuenow","valuemin","valuemax"]){const value=el.getAttribute(`aria-${key}`);if(value!==null) state.add(`${key}=${value}`);}
    if(el.matches(":disabled")) state.add("disabled=true");
    if(el instanceof HTMLInputElement){if(["checkbox","radio"].includes(el.type)) state.add(`checked=${el.checked}`);if(el.readOnly) state.add("readonly=true");}
    if(el instanceof HTMLTextAreaElement && el.readOnly) state.add("readonly=true");
    if(el instanceof HTMLOptionElement) state.add(`selected=${el.selected}`);
    if(el.tagName==="SUMMARY") state.add(`expanded=${(el.parentElement as HTMLDetailsElement).open}`);
    let name=nameOf(el,labelled);
    if(!name && el instanceof HTMLOptionElement) name=el.getAttribute("label") || "";
    if(!name && namedText.has(role)) name=visibleText(el).replace(/\s+/g," ").trim();
    if(!name && el.tagName==="IMG") name=el.getAttribute("alt") || "";
    if(!name && ["region","article","form","dialog","alertdialog"].includes(role)){const heading=card ? Array.from(el.querySelectorAll("h1,h2,h3,h4,h5,h6")).find(child=>child.closest('[data-slot="card"]')===el && safe(child)) : Array.from(el.children).find(child=>/^H[1-6]$/.test(child.tagName)); name=heading ? visibleText(heading) : module;}
    if(name.length>400){limits.add("An accessible label was shortened; its remaining text is unavailable.");name=name.slice(0,400);}
    const selected=el instanceof HTMLSelectElement ? Array.from(el.selectedOptions) : [];
    const eligible=selected.filter(option=>safe(option));
    if(eligible.length!==selected.length){state.add("value_redacted=true");limits.add("A selected option is private or hidden; that selection value is unavailable.");}
    const rawValue=el instanceof HTMLSelectElement ? eligible.map(option=>nameOf(option,false) || option.getAttribute("label") || visibleText(option).trim()).join(", ") : el instanceof HTMLInputElement && ["radio","checkbox"].includes(el.type) ? el.type==="radio" ? name : "" : el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement ? el.value : fieldButton ? el.getAttribute("data-assistant-value") || visibleText(el).trim() : "";
    const preview=fieldPreview(rawValue);
    if(preview.truncated){
     state.add("value_truncated=true");
     limits.add("A public field value is a Unicode-safe preview limited to 2048 UTF-8 bytes; its remaining text is unavailable. The underlying business value is unchanged.");
     let prior=fieldVersions.get(el);if(!prior || prior.value!==rawValue){prior={value:rawValue,revision:crypto.randomUUID()};fieldVersions.set(el,prior);}
     valueRevisions.push(prior.revision);
    }
    const value=preview.value;
    if(state.size>12 || Array.from(state).some(s=>s.length>2048 || new TextEncoder().encode(s).length>2048)) throw new Error("page_content_budget");
    const actions=controlOperations(el);
    let targetRef:string|undefined;
    if((actions.length || el instanceof HTMLOptionElement) && el instanceof HTMLElement){
     if(!controlIdentities.has(el)) controlIdentities.set(el,crypto.randomUUID());
     targetRef=controlIdentities.get(el)!; controls.set(targetRef,el);
    }
    // Prefer authored business IDs, never generated instance IDs/UUIDs as UI labels.
    const authoredNamespace=el.closest('[data-assistant-module]')?.getAttribute("data-assistant-module") || "page";
    const namespace=/^[a-z][a-z0-9/-]{0,119}$/.test(authoredNamespace) ? authoredNamespace : "page";
    const stableID=labelled && /^[a-z][a-z0-9-]{0,95}$/.test(el.id) && !/[0-9a-f]{8}-[0-9a-f]{4}-/.test(el.id) ? `${namespace}/${el.id}` : hint && module===hint ? module : undefined;
    nextParent=add({parent,module,role,name:name.trim(),value,state:[...state],actions,...(targetRef ? {targetRef} : {})},stableID);
   }
   for(const child of el.childNodes){if(child.nodeType===Node.TEXT_NODE){if(!field && (!role || !namedText.has(role))) text(child.textContent || "",nextParent,module);}else if(child instanceof Element) walk(child,nextParent,module,depth+1);}
  };
  // Reject pathological structure before expensive style/name resolution. This
  // walk never reads attribute/text values and skips inactive/private subtrees.
  let rawVisits=0; const visited=new WeakSet<Element>();
  const preflight=(el:Element,depth=0)=>{
   if(visited.has(el) || privateNode(el) || el!==root && el.matches(excluded)) return;
   if(++rawVisits>32768 || depth>128) throw new Error("page_content_budget");
   visited.add(el); for(const child of el.children) preflight(child,depth+1);
  };
  for(const boundary of boundaries) preflight(boundary);
  walk(root!,"","page");
  for(const boundary of boundaries.slice(1)) if(!root!.contains(boundary) && !boundaries.slice(1).some(other=>other!==boundary && other.contains(boundary))) walk(boundary,nodes[0].id,"page");
 }
 if(root && !rootIdentities.has(root)) rootIdentities.set(root,crypto.randomUUID());
 const limitations=[...limits];
 const encoding=JSON.stringify({page,nodes,limitations,generation,viewKey,valueRevisions,owner:root?.dataset.assistantOwner,rootIdentity:root ? rootIdentities.get(root) : ""});
 if(new TextEncoder().encode(encoding).length>512*1024) throw new Error("page_content_budget");
 if(encoding!==lastEncoding){lastEncoding=encoding;lastID=crypto.randomUUID();}
 capturedControls=controls;
 return {id:lastID,page,capturedAt:new Date().toISOString(),nodes,limitations};
}
