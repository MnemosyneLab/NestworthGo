// Frontend-only synthetic layout regression. Real AppShell/pages/shared controls;
// queries and unrelated child features are replaced, with no backend or ledger.
import { mkdtempSync, writeFileSync, mkdirSync, rmSync } from 'node:fs';
import { dirname, resolve, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import assert from 'node:assert/strict';
const repo = resolve(dirname(fileURLToPath(import.meta.url)), '../../..');
const frontend = join(repo, 'frontend');
const { createServer } = await import(pathToFileURL(join(frontend, 'node_modules/vite/dist/node/index.js')));
const { default: react } = await import(pathToFileURL(join(frontend, 'node_modules/@vitejs/plugin-react/dist/index.js')));
const { default: tailwind } = await import(pathToFileURL(join(frontend, 'node_modules/@tailwindcss/vite/dist/index.mjs')));
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const scratch = mkdtempSync(join(frontend, '.narrow-qa-'));
const output = process.env.QA_OUTPUT || '/tmp/nestworth-desktop-narrow';
mkdirSync(output, { recursive: true });
const query = data => `({data:${JSON.stringify(data)},isLoading:false,isPending:false,isError:false,isFetching:false,dataUpdatedAt:0,refetch:()=>{}})`;
const records = ['SyntheticLongUnbrokenAccountNameForLayoutTesting', '合成测试长期储蓄账户名称用于检查窄屏换行'].map((name, i) => ({ account: { id: `synthetic-${i}`, name, accountType: 'bank_account', trackingMode: 'balance', defaultCurrency: 'USD', institutionId: 'synthetic-bank' }, institutionName: 'SyntheticLongInstitutionName', latestValue: { amount: { amount: '1234567890123.45', currency: 'USD' } } }));
const values = records.map(record => ({ account: record.account, baseValue: {amount:'1234567890123.45',currency:'USD'}, complete:true }));
const mocks = new Map([
 ['@/queries/portfolio', `export const useOverview=()=>${query({currency:'USD',accountCount:2,complete:true,assets:'1234567890123.45',liabilities:'123456789.67',netWorth:'1234444433333.78',historyStarted:true})};`],
 ['@/queries/accounts', `export const useAccounts=()=>${query(records)};export const useAccountValuations=()=>${query(values)};export const useCreateAccount=()=>({});`],
 ['@/queries/settings', `export const useSettings=()=>${query({timezone:'UTC'})};export const useSaveSettings=()=>({});`],
 ['@/queries/marketdata', `export const useMarketDataHealth=()=>${query({healthy:true,issueCount:0,issues:[]})};`],
 ['@/queries/directory', `export const useInstitutions=()=>${query([])};`],
 ['@/queries/investments', `export const useInstruments=()=>${query([])};`],
 ['@/queries/useValuationDayBoundary', 'export const useValuationDayBoundary=()=>{};'],
 ['@/features/tasks/useTasksCounts', `export const formatTasksBadge=()=>'';export const useTasksCounts=()=>({upcoming:{},review:{},total:0});`],
 ['@/features/assistant/PageContext', 'export const usePublishAssistantPage=()=>{};export const useAssistantPage=()=>null;'],
]);
for (const [path, name] of [
 ['@/features/accounts/AccountDetail','AccountDetail'], ['@/features/accounts/AccountCreateWizard','AccountCreateWizard'],
 ['./NetWorthTrendCard','NetWorthTrendCard'], ['@/features/assistant/ModelConversation','ModelConversation'], ['./ModelConversation','ModelConversation'],
 ['@/features/data-health/DataHealthIndicator','DataHealthIndicator'], ['@/features/tasks/TasksHomeSummary','TasksHomeSummary'],
 ['@/features/upcoming-actions/OverviewUpcomingActionsCard','OverviewUpcomingActionsCard'], ['@/features/liquidity/OverviewLiquidityCard','OverviewLiquidityCard'],
]) mocks.set(path, `export const ${name}=()=>null;`);
writeFileSync(join(scratch, 'index.html'), '<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head><body><div id="root"></div><script type="module" src="./main.tsx"></script></body></html>');
writeFileSync(join(scratch, 'main.tsx'), `
import React,{useState} from 'react';import{createRoot}from'react-dom/client';import{QueryClient,QueryClientProvider}from'@tanstack/react-query';
import i18n from '@/i18n';import '@/index.css';import{AppShell}from'@/app/AppShell';import{OverviewPage}from'@/features/overview/OverviewPage';import{AccountsPage}from'@/features/accounts/AccountsPage';import{NavigationContext}from'@/app/NavigationContext';
function Fixture(){const[page,setPage]=useState('overview');window.qa={page:setPage,language:l=>i18n.changeLanguage(l),t:k=>i18n.t(k)};return <QueryClientProvider client={new QueryClient()}><NavigationContext.Provider value={{open:target=>setPage(target.page)}}><AppShell activePageId={page} onNavigate={target=>setPage(target.page)} settings={{appearance:'light',language:'en'}} navigationHistory={{canBack:false,canForward:false,back:()=>{},forward:()=>{},recent:[]}}>{page==='overview'?<OverviewPage/>:<AccountsPage onSelectAccount={id=>{window.selectedAccount=id;}}/>}</AppShell></NavigationContext.Provider></QueryClientProvider>};createRoot(document.getElementById('root')).render(<Fixture/>);
`);
const server = await createServer({ configFile:false, root:frontend, resolve:{alias:{'@':join(frontend,'src')}}, plugins:[{
 name:'synthetic-layout', enforce:'pre', resolveId(source,importer){const absolute=source.startsWith('.')&&importer?resolve(dirname(importer),source):source;const key=absolute.startsWith(frontend+'/src/')?'@/'+absolute.slice((frontend+'/src/').length):source;if(mocks.has(key))return '\0mock:'+key;if(mocks.has(source))return '\0mock:'+source;}, load(id){if(id.startsWith('\0mock:'))return mocks.get(id.slice(6));},
}, react(), tailwind()], server:{host:'127.0.0.1',port:9363,strictPort:true}});
let browser; const results=[]; const errors=[];
try {
 await server.listen();browser=await chromium.launch({executablePath:process.env.CHROMIUM_PATH || '/usr/bin/chromium',headless:true,args:['--no-sandbox']});
 for(const width of [320,390,1440]) for(const language of ['en','zh-CN','zh-TW']) for(const theme of ['light','dark']) for(const fontSize of [16,32]) {
  const page=await browser.newPage({viewport:{width,height:1000},reducedMotion:'reduce'});page.on('pageerror',error=>errors.push(error.message));
  await page.goto(`http://127.0.0.1:9363/${scratch.split('/').at(-1)}/index.html`);
  await page.waitForFunction(()=>!!window.qa);await page.evaluate(async({language,theme,fontSize})=>{await window.qa.language(language);document.documentElement.classList.toggle('dark',theme==='dark');document.documentElement.style.fontSize=fontSize+'px';},{language,theme,fontSize});
  await page.locator('[data-testid="overview-net-worth"]').waitFor();await page.evaluate(()=>document.fonts.ready);
  const measure=async(selector)=>page.locator(selector).evaluateAll(elements=>elements.map(el=>{const r=el.getBoundingClientRect();const range=document.createRange();range.selectNodeContents(el);const rects=[...range.getClientRects()];return {text:el.textContent,clipped:el.scrollWidth>el.clientWidth+1 || rects.some(x=>x.left<r.left-1 || x.right>r.right+1),outside:rects.some(x=>x.left<0 || x.right>innerWidth+1)};}));
  assert.equal(await page.locator('[data-testid="overview-net-worth"]').textContent(), 'US$1,234,444,433,333.78');
  const overview=await measure('header h1,header button,[data-testid="overview-net-worth"],[data-testid="overview-assets"],[data-testid="overview-liabilities"]');
  await page.evaluate(()=>window.qa.page('accounts'));await page.locator('[data-testid="accounts-table"]').waitFor();
  for(const record of records) assert(await page.locator('[data-testid="accounts-table"]').getByText(record.account.name,{exact:true}).isVisible());
  assert.equal(await page.locator('[data-testid="accounts-table"] li').count(),2);
  assert.equal(await page.locator('[data-testid="accounts-table"]').getByText('US$1,234,567,890,123.45',{exact:true}).count(),2);
  const badgeLabel=await page.evaluate(()=>window.qa.t('enum.bank_account'));assert.equal(await page.locator('[data-testid="accounts-table"]').getByText(badgeLabel,{exact:true}).count(),2);
  const accounts=await measure('[data-testid="accounts-table"] button > span:not([aria-hidden]),[data-testid="accounts-table"] h2,[data-testid^="accounts-group-subtotal"]');
  const overlaps=await page.locator('[data-testid="accounts-table"] li button').evaluateAll(rows=>rows.some(row=>{const name=row.children[1].getBoundingClientRect(),money=row.children[2].getBoundingClientRect();return name.left<money.right && name.right>money.left && name.top<money.bottom && name.bottom>money.top;}));
  await page.locator('[data-testid="accounts-table"] li button').first().focus();await page.keyboard.press('Enter');assert(await page.evaluate(()=>window.selectedAccount?.startsWith('synthetic-')));
  results.push({width,language,theme,fontSize,overview,accounts,overlaps});
  if(language==='en' && theme==='light'){await page.screenshot({path:join(output,`accounts-${width}-${fontSize}.png`)});await page.evaluate(()=>window.qa.page('overview'));await page.screenshot({path:join(output,`overview-${width}-${fontSize}.png`)});}
  await page.close();
 }
 writeFileSync(join(output,'results.json'),JSON.stringify({results,errors},null,2));
 const failures=results.filter(r=>r.overlaps || [...r.overview,...r.accounts].some(x=>x.clipped || x.outside));
 console.log(JSON.stringify({cases:results.length,failures:failures.length,errors,output}));
 if(!process.env.QA_BASELINE){assert.equal(errors.length,0);assert.equal(failures.length,0);}
} finally {await browser?.close();await server.close();rmSync(scratch,{recursive:true,force:true});}
