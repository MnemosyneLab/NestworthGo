import type { SemanticPage } from "@/features/assistant/pageContent";

/** Inspect published parent links, including automatically derived child modules. */
export function moduleNodes(page: SemanticPage, module: string) {
 const byID=new Map(page.nodes.map(node=>[node.id,node]));
 return page.nodes.filter(node=>{
  const visited=new Set<string>();let current:typeof node|undefined=node;
  while(current && !visited.has(current.id)){
   if(current.module===module) return true;
   visited.add(current.id);current=byID.get(current.parent);
  }
  return false;
 });
}
