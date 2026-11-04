// Real React components with a synthetic desktop IPC adapter, no Wails host or
// provider network. Run from any cwd with Node 24 and PLAYWRIGHT_MODULE set.
import {mkdtempSync,writeFileSync,mkdirSync,rmSync} from 'node:fs';
import {dirname,resolve,basename,join} from 'node:path';
import {pathToFileURL,fileURLToPath} from 'node:url';
import assert from 'node:assert/strict';
const repo=resolve(dirname(fileURLToPath(import.meta.url)),'../../..');
const frontend=join(repo,'frontend');
const {createServer}=await import(pathToFileURL(join(frontend,'node_modules/vite/dist/node/index.js')));
const {default:react}=await import(pathToFileURL(join(frontend,'node_modules/@vitejs/plugin-react/dist/index.js')));
const {default:tailwind}=await import(pathToFileURL(join(frontend,'node_modules/@tailwindcss/vite/dist/index.mjs')));
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const scratch=mkdtempSync(join(frontend,'.ai-qa-'));
const output=resolve(process.env.AI_BROWSER_OUTPUT || '/tmp/nestworth-ai-browser');mkdirSync(output,{recursive:true});
writeFileSync(join(scratch,'index.html'),'<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><div id="root"></div><script type="module" src="./main.tsx"></script></body></html>');
writeFileSync(join(scratch,'main.tsx'),`
import React from 'react';import{createRoot}from'react-dom/client';import{QueryClient,QueryClientProvider}from'@tanstack/react-query';import i18n from '@/i18n';import '@/index.css';
import{useDesktopAssistant}from'@/features/assistant/DesktopAssistant';import{AICredentialsSection}from'@/features/settings/AICredentialsSection';import{AssistantPageProvider,usePublishAssistantPage}from'@/features/assistant/PageContext';
window.__qa=window.__qa||{calls:[],navigated:0};window.__qa.language=l=>i18n.changeLanguage(l);window.__qa.t=k=>i18n.t(k);
const observedAt=Date.now();
function Publisher(){usePublishAssistantPage({viewKey:'accounts:',page:'accounts',scope:{kind:'household',label:'Synthetic household'},timezone:'UTC',basis:'base',currency:'USD',observedAt,state:'available',targets:[{label:'Synthetic checking',destination:{page:'accounts',accountId:'11111111-1111-4111-8111-111111111111'}}],metrics:[]});return null;}
function Content(){const a=useDesktopAssistant();return <div className="flex h-screen min-w-0 bg-background text-foreground"><main className="min-w-0 flex-1 space-y-4 overflow-y-auto p-4"><header>{a.trigger}</header><AICredentialsSection/></main>{a.panel}</div>;}
createRoot(document.getElementById('root')).render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><AssistantPageProvider householdId="synthetic" incarnation="synthetic" surface="desktop" viewKey="accounts:" blocked={()=>window.__qa.blocked} onNavigate={()=>{window.__qa.navigated++;return true;}}><Publisher/><Content/></AssistantPageProvider></QueryClientProvider>);
`);
const server=await createServer({configFile:false,root:frontend,resolve:{alias:{'@':join(frontend,'src')}},plugins:[{name:'synthetic-ai-ipc',enforce:'pre',resolveId(source){if(source.endsWith('/wailsapi/airuntime/service'))return '\0synthetic-ai';},load(id){if(id!=='\0synthetic-ai')return;return `
const status={epoch:'synthetic-host',revision:1,available:false,credentials:'missing',config:{provider:'openai-responses',model:'unconfigured',enabled:false},financialWrites:false};let conversation=null;let pageLease=null;let grant=null;let leaseNumber=0;window.__qa=window.__qa||{calls:[],navigated:0};
export const Status=async()=>({...status,config:{...status.config}});
export const PublishPage=async(epoch,revision,page)=>{pageLease={id:'synthetic-page-'+(++leaseNumber),epoch,revision,page};grant=null;return {...pageLease};};
export const GrantNavigation=async(epoch,revision,conversationId,requestId,leaseId)=>{if(leaseId!==pageLease.id)throw Error('stale');grant={requestId,leaseId};window.__qa.grants=(window.__qa.grants||0)+1;};
export const ConsumeNavigation=async(epoch,conversationId,runId,intentId,leaseId)=>{if(!conversation.navigation||leaseId!==pageLease.id||conversation.navigation.id!==intentId)throw Error('stale');conversation.navigation=null;return {kind:'overview',accountId:''};};
export const SaveCredential=async(_epoch,key)=>{if(key!=='synthetic-browser-key')throw Error('synthetic');status.credentials='present';status.revision++;};
export const DeleteCredential=async()=>{status.credentials='missing';status.available=false;status.config.enabled=false;status.revision++;};
export const Configure=async(_epoch,_revision,config)=>{status.config={...config};status.available=config.enabled;status.revision++;};
export const List=async()=>conversation?[{id:conversation.id,runCount:conversation.runCount}]:[];
export const Create=async()=>{conversation={id:'synthetic-conversation',runs:[],runCount:0,offset:-1};return structuredClone(conversation);};
export const Inspect=async()=>structuredClone(conversation);
export const Messages=async()=>structuredClone(conversation);
export const Start=async request=>{window.__qa.calls.push(request);conversation.runCount++;conversation.offset=conversation.runCount-1;if(!request.pageLeaseId)grant=null;const run={id:'synthetic-run-'+conversation.runCount,requestId:request.requestId,prompt:request.prompt,state:'running',text:'',model:'synthetic-model',created:new Date().toISOString()};conversation.runs=[run];setTimeout(()=>{if(run.state==='running'){if(grant&&grant.requestId===request.requestId){conversation.navigation={id:'synthetic-intent',leaseId:grant.leaseId,requestId:request.requestId,runId:run.id,conversationId:conversation.id};}run.state='succeeded';run.evidence=[{source:'list_accounts',capturedAt:run.created,hasMore:true,missing:'not_summarized'}];run.text='Synthetic checking: 12.34 USD. Backend capture: synthetic source, missing inputs disclosed. This response is a browser fixture.';}},2200);return {...run};};
export const Cancel=async()=>{conversation.runs[0].state='cancelled';conversation.navigation=null;grant=null;};export const Delete=async()=>{conversation=null;};
`; }},react(),tailwind()],server:{host:'127.0.0.1',port:9348,strictPort:true}});
let browser;const results=[];const errors=[];
try{
 await server.listen();browser=await chromium.launch({executablePath:process.env.CHROMIUM_PATH||'/usr/bin/chromium',headless:true,args:['--no-sandbox']});
 for(const [width,language,theme]of [[1440,'en','light'],[768,'zh-CN','dark']]){
 const page=await browser.newPage({viewport:{width,height:1000},hasTouch:width<1280});page.on('pageerror',e=>errors.push(e.message));
 await page.goto('http://127.0.0.1:9348/'+basename(scratch)+'/index.html');await page.waitForFunction(()=>!!window.__qa?.language);
 await page.evaluate(async({language,theme})=>{await window.__qa.language(language);document.documentElement.classList.toggle('dark',theme==='dark');},{language,theme});
 const t=key=>page.evaluate(key=>window.__qa.t(key),key);
 await page.getByLabel(await t('aiCredentials.key'),{exact:true}).fill('synthetic-browser-key');await page.getByRole('button',{name:await t('aiCredentials.save'),exact:true}).click();
 await page.getByLabel(await t('aiChat.model'),{exact:true}).fill('synthetic-model');await page.getByRole('checkbox').check();await page.getByRole('button',{name:await t('aiChat.saveConfig'),exact:true}).click();
 await page.getByRole('button',{name:await t('assistant.open'),exact:true}).click();await page.getByText(await t('aiChat.ready'),{exact:true}).waitFor();
 await page.getByLabel(await t('aiChat.message'),{exact:true}).fill('Read my synthetic current accounts');await page.getByRole('button',{name:await t('aiChat.send'),exact:true}).click();
 await page.getByText(await t('aiChat.run.running'),{exact:true}).waitFor();await page.getByText(/Synthetic checking: 12.34 USD/).waitFor();
 assert.equal(await page.evaluate(()=>window.__qa.calls.length),1);assert.deepEqual(await page.evaluate(()=>window.__qa.calls[0].page),{kind:'accounts',accountId:''});
 assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
 await page.getByText(await t('assistant.evidence'),{exact:true}).click();await page.getByText(await t('aiChat.missingUnknown'),{exact:false}).waitFor();
 await page.screenshot({path:join(output,'chat-'+width+'.png'),fullPage:true});
 // Guarded navigation is an explicit click; it cannot be supplied by a model.
 await page.evaluate(()=>window.__qa.blocked=true);await page.getByRole('button',{name:'Synthetic checking',exact:true}).click();assert.equal(await page.evaluate(()=>window.__qa.navigated),0);
 await page.evaluate(()=>window.__qa.blocked=false);await page.getByRole('button',{name:'Synthetic checking',exact:true}).click();assert.equal(await page.evaluate(()=>window.__qa.navigated),1);
 await page.getByLabel(await t('aiChat.message'),{exact:true}).fill('Second synthetic question');await page.getByRole('button',{name:await t('aiChat.send'),exact:true}).click();await page.getByRole('button',{name:await t('aiChat.stop'),exact:true}).waitFor();await page.context().setOffline(true);await page.getByRole('button',{name:await t('aiChat.stop'),exact:true}).click();await page.getByText(await t('aiChat.run.cancelled'),{exact:true}).waitFor();
 assert.equal(await page.evaluate(()=>window.__qa.calls.length),2);assert.equal(await page.evaluate(()=>window.__qa.grants||0),0);await page.context().setOffline(false);
 await page.getByLabel(await t('aiChat.message'),{exact:true}).fill('Open overview');await page.getByRole('button',{name:await t('aiChat.sendNavigate'),exact:true}).click();await page.getByText(await t('aiChat.navigationOpened'),{exact:true}).waitFor();assert.equal(await page.evaluate(()=>window.__qa.navigated),2);assert.equal(await page.evaluate(()=>window.__qa.grants),1);assert.equal(await page.evaluate(()=>window.__qa.calls.length),3);
 results.push({width,language,theme,explicitActivation:true,selectorOnly:true,completed:true,cancelled:true,offlineHistoryAndStop:true,explicitModelNavigation:true,navigationGuard:true,noOverflow:true,providerCalls:0});await page.close();
 }
 assert.deepEqual(errors,[]);writeFileSync(join(output,'results.json'),JSON.stringify({results,errors},null,2));console.log(JSON.stringify({results,errors}));
}finally{await browser?.close();await server.close();rmSync(scratch,{recursive:true,force:true});}
