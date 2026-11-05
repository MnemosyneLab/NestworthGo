import { mkdtempSync, writeFileSync, rmSync, mkdirSync } from 'node:fs';
import { join, dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
const repo=resolve(dirname(fileURLToPath(import.meta.url)), '../../..');
const frontend=join(repo,'frontend');
const output=resolve(process.env.AI_BROWSER_OUTPUT || '/tmp/nestworth-ai-live-browser');mkdirSync(output,{recursive:true});
const require=createRequire(join(frontend,'package.json'));
const {createServer}=await import(require.resolve('vite'));
const {default:react}=await import(require.resolve('@vitejs/plugin-react'));
const {default:tailwind}=await import(require.resolve('@tailwindcss/vite'));
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE || 'playwright-core');
const dir=mkdtempSync(join(frontend,'.ai-live-browser-'));
writeFileSync(join(dir,'index.html'),'<html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><div id="root"></div><script type="module" src="./main.tsx"></script></body></html>');
writeFileSync(join(dir,'main.tsx'),`
import React,{useState} from 'react';import{createRoot}from'react-dom/client';import{QueryClient,QueryClientProvider}from'@tanstack/react-query';import '@/i18n';import '@/index.css';import{ConversationTimeline}from'@/features/assistant/ConversationTimeline';
const saved=Array.from({length:8},(_,i)=>({id:'run-'+i,requestId:'request-'+i,prompt:'Synthetic question '+i,state:'succeeded',text:'## Synthetic answer '+i+'\\n\\n'+Array.from({length:8},(_,j)=>'- Synthetic account '+j+': **USD 12.3456**').join('\\n'),created:'2026-11-05T00:00:00Z',model:'mock'}));
const live={id:'run-live',requestId:'request-live',prompt:'Read synthetic totals',state:'running',text:'',created:'2026-11-05T00:00:00Z',model:'mock'};
const initial={id:'synthetic',offset:0,runCount:9,runs:[...saved,live],progress:{runId:live.id,revision:1,text:'## Synthetic preview\\n\\n**Exact USD 12.3456**\\n\\n'+Array.from({length:6},(_,i)=>'- Synthetic item '+i).join('\\n')+'\\n\\n\\x60\\x60\\x60json\\n{"unfinished":"',tools:[{source:'get_financial_context',state:'running',input:'{"disclosure":"minimal"}',output:'',inputTruncated:false,outputTruncated:false}]}};
function App(){const[state,setState]=useState(initial);window.qa={update:()=>setState(s=>({...s,progress:{...s.progress,revision:s.progress.revision+1,text:s.progress.text+' synthetic extra text. '.repeat(10)}})),finish:()=>setState(s=>({...s,runs:s.runs.map(r=>r.id===live.id?{...r,state:'succeeded',text:'## Completed synthetic answer\\n\\n**USD 12.3456**'}:r),progress:undefined})),newSession:()=>setState({...initial,id:'other',runs:[{...live,id:'other-run',prompt:'Other synthetic session',state:'succeeded',text:'Only other session'}],progress:undefined})};return <main className="h-screen bg-background text-foreground"><div id="sidebar" className="ml-auto flex h-full flex-col border-l border-border" style={{width:'400px',maxWidth:'100%'}}><QueryClientProvider client={new QueryClient()}><ConversationTimeline key={state.id} epoch="synthetic" id={state.id} latest={state} busy={false} onCancel={()=>{}}/></QueryClientProvider></div></main>};createRoot(document.getElementById('root')).render(<App/>);
`);
const server=await createServer({configFile:false,root:frontend,resolve:{alias:{'@':join(frontend,'src')}},plugins:[{name:'synthetic-ipc',enforce:'pre',resolveId(id){if(id.endsWith('/wailsapi/airuntime/service'))return '\0synthetic-ipc'},load(id){if(id==='\0synthetic-ipc')return 'export const Messages=async()=>{throw new Error("unexpected history read")};'}},react(),tailwind()],server:{host:'127.0.0.1',port:9357,strictPort:true}});
let browser;
const errors=[];
try{
 await server.listen();browser=await chromium.launch({executablePath:'/usr/bin/chromium',headless:true,args:['--no-sandbox']});
 const results=[];
 for(const theme of ['light','dark']){
  const page=await browser.newPage({viewport:{width:1000,height:800}});
  page.on('pageerror',e=>errors.push(e.message));
  await page.goto('http://127.0.0.1:9357/'+dir.split('/').pop()+'/index.html');
  await page.evaluate(theme=>document.documentElement.classList.toggle('dark',theme==='dark'),theme);
  const region=page.getByRole('region',{name:'Conversation messages'});
  await page.getByText('Synthetic preview',{exact:true}).waitFor();
  const bottom=()=>region.evaluate(el=>Math.abs(el.scrollHeight-el.clientHeight-el.scrollTop)<3);
  await page.waitForFunction(()=>{const el=document.querySelector('[role=region]');return el&&Math.abs(el.scrollHeight-el.clientHeight-el.scrollTop)<3});
  await page.evaluate(()=>window.qa.update());await page.waitForTimeout(100);assert(await bottom());
  await page.getByText(/Financial summary · Running/).click();await page.waitForTimeout(100);assert(await bottom());
  await page.evaluate(()=>document.querySelector('#sidebar').style.width='650px');await page.waitForTimeout(100);assert(await bottom());
  await region.evaluate(el=>el.scrollTop=100);await page.waitForTimeout(50);
  await page.evaluate(()=>{window.qa.update();document.querySelector('#sidebar').style.width='360px'});await page.waitForTimeout(100);assert(Math.abs(await region.evaluate(el=>el.scrollTop)-100)<3);
  await page.getByRole('button',{name:'Latest messages'}).click();await page.waitForTimeout(100);assert(await bottom());
  await page.screenshot({path:join(output,theme+'.png')});
  await page.setViewportSize({width:390,height:760});await page.waitForTimeout(100);assert(await bottom());assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
  await page.evaluate(()=>window.qa.finish());await page.getByText('Completed synthetic answer',{exact:true}).waitFor();assert.equal(await page.getByText('Synthetic preview',{exact:true}).count(),0);
  await page.evaluate(()=>window.qa.newSession());await page.getByText('Only other session',{exact:true}).waitFor();assert.equal(await page.getByText('Completed synthetic answer',{exact:true}).count(),0);
  results.push({theme,followChunks:true,followToolDisclosure:true,followWidth:true,pauseOnScroll:true,resumeLatest:true,narrowNoOverflow:true,sessionIsolation:true,syntheticOnly:true});await page.close();
 }
 assert.deepEqual(errors,[]);writeFileSync(join(output,'results.json'),JSON.stringify({results,errors},null,2));console.log(JSON.stringify({results,errors}));
}finally{await browser?.close();await server.close();rmSync(dir,{recursive:true,force:true})}
