// Synthetic semantic-page fixture with the real desktop host, sidebar and shared
// controls. No database, native app, credentials, provider or financial writes.
import {mkdtempSync,writeFileSync,mkdirSync,rmSync} from 'node:fs';
import {dirname,resolve,join} from 'node:path';
import {pathToFileURL,fileURLToPath} from 'node:url';
import assert from 'node:assert/strict';
const repo=resolve(dirname(fileURLToPath(import.meta.url)),'../../..');
const frontend=join(repo,'frontend');
const {createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')));
const {default:react}=await import(pathToFileURL(join(frontend,'node_modules/@vitejs/plugin-react/dist/index.js')));
const {default:tailwind}=await import(pathToFileURL(join(frontend,'node_modules/@tailwindcss/vite/dist/index.mjs')));
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const scratch=mkdtempSync(join(frontend,'.page-content-qa-'));
const output=resolve(process.env.PAGE_CONTENT_OUTPUT || '/tmp/nestworth-page-content-browser');mkdirSync(output,{recursive:true});
writeFileSync(join(scratch,'index.html'),'<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><div id="root"></div><script type="module" src="./main.tsx"></script></body></html>');
writeFileSync(join(scratch,'main.tsx'),`
import React,{useState}from'react';import{createRoot}from'react-dom/client';import{QueryClient,QueryClientProvider}from'@tanstack/react-query';import i18n from '@/i18n';import '@/index.css';
import{useDesktopAssistant}from'@/features/assistant/DesktopAssistant';import{AssistantPageProvider}from'@/features/assistant/PageContext';import{DatePickerControl}from'@/components/ui/date-picker-control';import{NativeSelect}from'@/components/ui/select';import{FilterableSelect}from'@/components/ui/filterable-select';
window.__pageQA=window.__pageQA||{pending:[],delivered:[],starts:0};window.__pageQA.language=l=>i18n.changeLanguage(l);window.__pageQA.t=k=>i18n.t(k);
function Content(){const a=useDesktopAssistant();const[date,setDate]=useState('2026-10-01');const[scope,setScope]=useState('private-account-id');const[fund,setFund]=useState('private-fund-id');window.__pageQA.setDate=setDate;return <div className="flex h-screen bg-background text-foreground"><main className="min-w-0 flex-1 space-y-4 p-4" data-assistant-page="return-analysis"><header>{a.trigger}</header><h1>收益分析 Return analysis</h1><section data-assistant-module="filters" className="space-y-3"><label htmlFor="analysis-from">From date</label><DatePickerControl id="analysis-from" value={date} dateFormat="iso" onChange={setDate}/><label htmlFor="analysis-scope">Scope</label><NativeSelect id="analysis-scope" value={scope} onChange={e=>setScope(e.target.value)}><option value="private-account-id">Synthetic account</option><option value="portfolio">All assets</option></NativeSelect><label htmlFor="analysis-filter-instrument">Instrument</label><FilterableSelect id="analysis-filter-instrument" value={fund} options={[{value:'private-fund-id',label:'Synthetic fund'},{value:'private-other-id',label:'Synthetic other fund'}]} onValueChange={setFund}/><label htmlFor="analysis-cash">Cash</label><input id="analysis-cash" type="checkbox" defaultChecked/><input type="password" defaultValue="never-export-password"/><textarea defaultValue="never-export-private-note"/><div data-assistant-private>never-export-private-region</div></section><section data-assistant-module="trend"><p role="status">Synthetic current data</p><details><summary>Data table</summary><table><tbody><tr><td>12.34 USD</td></tr></tbody></table></details></section></main>{a.panel}</div>}
createRoot(document.getElementById('root')).render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><AssistantPageProvider householdId="synthetic" incarnation="synthetic" surface="desktop" viewKey="return-analysis:" blocked={()=>false} onNavigate={()=>true}><Content/></AssistantPageProvider></QueryClientProvider>);
`);
const server=await createServer({configFile:false,root:frontend,resolve:{alias:{'@':join(frontend,'src')}},plugins:[{name:'synthetic-page-ipc',enforce:'pre',resolveId(source){if(source.endsWith('/wailsapi/airuntime/service'))return '\0synthetic-page';},load(id){if(id!=='\0synthetic-page')return;return `
window.__pageQA=window.__pageQA||{pending:[],delivered:[],starts:0};
export const Status=async()=>({epoch:'synthetic',revision:1,available:true,credentials:'present',financialWrites:false,config:{provider:'openai-responses',model:'synthetic',enabled:true}});
export const PendingPageReads=async()=>window.__pageQA.pending.map(id=>({id}));
export const DeliverPageContent=async(_epoch,id,content)=>{window.__pageQA.delivered.push(content);window.__pageQA.pending=window.__pageQA.pending.filter(item=>item!==id)};
export const PublishPage=async()=>({id:'synthetic-lease'});export const List=async()=>[];export const Inspect=async()=>null;export const Messages=async()=>null;export const Create=async()=>({id:'synthetic-conversation'});export const Start=async()=>{window.__pageQA.starts++;};export const GrantNavigation=async()=>{};export const ConsumeNavigation=async()=>({kind:'overview',accountId:''});export const Cancel=async()=>{};export const Delete=async()=>{};
`; }},react(),tailwind()],server:{host:'127.0.0.1',port:9359,strictPort:true}});
let browser;const results=[];const errors=[];
try{
 await server.listen();browser=await chromium.launch({executablePath:process.env.CHROMIUM_PATH||'/usr/bin/chromium',headless:true,args:['--no-sandbox']});
 for(const [width,language]of [[1440,'en'],[390,'zh-CN'],[390,'zh-TW']]){
  const page=await browser.newPage({viewport:{width,height:1000}});page.on('pageerror',e=>errors.push(e.message));
  await page.goto('http://127.0.0.1:9359/'+scratch.split('/').at(-1)+'/index.html');await page.waitForFunction(()=>!!window.__pageQA?.setDate);await page.evaluate(l=>window.__pageQA.language(l),language);
  const request=async()=>{const before=await page.evaluate(()=>window.__pageQA.delivered.length);await page.evaluate(()=>window.__pageQA.pending.push(crypto.randomUUID()));await page.waitForFunction(n=>window.__pageQA.delivered.length>n,before);return page.evaluate(()=>window.__pageQA.delivered.at(-1));};
  const closed=await request();assert.equal(closed.page,'return-analysis');assert(closed.nodes.some(n=>n.id==='filters/analysis-from'&&n.name==='From date'&&n.value==='2026-10-01'));assert(closed.nodes.some(n=>n.id==='filters/analysis-scope'&&n.value==='Synthetic account'));
  const encoded=JSON.stringify(closed);for(const secret of ['never-export-password','never-export-private-note','never-export-private-region','private-account-id','private-fund-id','<main'])assert(!encoded.includes(secret),secret);
  await page.getByRole('button',{name:await page.evaluate(()=>window.__pageQA.t('assistant.open'))}).click();
  if(width<1280)await page.getByRole('dialog').waitFor();
  const open=await request();assert.equal(open.page,'return-analysis');assert(open.nodes.some(n=>n.name==='12.34 USD'));
  if(width<1280)await page.keyboard.press('Escape');else await page.getByRole('button',{name:await page.evaluate(()=>window.__pageQA.t('assistant.collapse'))}).click();
  await page.getByLabel('Scope',{exact:true}).selectOption('portfolio');await page.getByLabel('Cash',{exact:true}).uncheck();await page.evaluate(()=>window.__pageQA.setDate('2026-10-02'));
  const changed=await request();assert.notEqual(changed.id,open.id);assert(changed.nodes.some(n=>n.id==='filters/analysis-from'&&n.value==='2026-10-02'));assert(changed.nodes.some(n=>n.id==='filters/analysis-scope'&&n.value==='All assets'));assert(changed.nodes.some(n=>n.id==='filters/analysis-cash'&&n.state.includes('checked=false')));assert.equal(await page.evaluate(()=>window.__pageQA.starts),0);
  results.push({width,language,nodes:closed.nodes.length,modules:[...new Set(closed.nodes.map(n=>n.module))],closedSidebarReadable:true,modalPageReadable:width<1280,latestState:true,sensitiveCanariesExcluded:true,rawHTMLExcluded:true,modelStarts:0});await page.close();
 }
 assert.deepEqual(errors,[]);writeFileSync(join(output,'results.json'),JSON.stringify({results,uncaughtBrowserErrors:errors},null,2)+'\n');console.log(JSON.stringify(results));
}finally{await browser?.close();await server.close();rmSync(scratch,{recursive:true,force:true});}
