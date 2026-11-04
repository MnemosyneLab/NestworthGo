// Synthetic browser adapter for real settings/assistant components; no Wails host.
// Run with Node 24 from the repository root. PLAYWRIGHT_MODULE may point to an
// installed playwright package; Chromium must be installed on the test host.
import { mkdtempSync, writeFileSync, mkdirSync, rmSync } from 'node:fs';
import { dirname, resolve, basename, join } from 'node:path';
import { pathToFileURL, fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';
const repo = resolve(dirname(fileURLToPath(import.meta.url)), '../../..');
const frontend = join(repo, 'frontend');
const { createServer } = await import(pathToFileURL(join(frontend, 'node_modules/vite/dist/node/index.js')));
const { default: react } = await import(pathToFileURL(join(frontend, 'node_modules/@vitejs/plugin-react/dist/index.js')));
const { default: tailwind } = await import(pathToFileURL(join(frontend, 'node_modules/@tailwindcss/vite/dist/index.mjs')));
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');
const scratch = mkdtempSync(join(frontend, '.atlas-qa-'));
const output = resolve(process.env.ATLAS_BROWSER_OUTPUT || '/tmp/atlas-browser-evidence');
mkdirSync(output, { recursive: true });
const status = { version: 1, surface: 'desktop', pageGuide: true, provider: 'unavailable', credentials: 'unavailable', conversations: 'memory', modelRuns: false, financialWrites: false, reason: 'activation_incomplete' };
writeFileSync(join(scratch, 'index.html'), '<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><title>Synthetic Atlas acceptance</title></head><body><div id="root"></div><script type="module" src="./main.tsx"></script></body></html>');
writeFileSync(join(scratch, 'main.tsx'), `
import React from 'react'; import {createRoot} from 'react-dom/client';
import {QueryClient, QueryClientProvider} from '@tanstack/react-query';
import {useTranslation} from 'react-i18next'; import i18n from '@/i18n';
import '@/index.css';
import {MobileAccessSection} from '@/features/settings/MobileAccessSection';
import {useDesktopAssistant} from '@/features/assistant/DesktopAssistant';
import {MobileAssistant} from '@/mobile/MobileAssistant';
import {AssistantPageProvider, usePublishAssistantPage} from '@/features/assistant/PageContext';
const surface = new URLSearchParams(location.search).get('surface') || 'desktop';
window.__qa.language = language => i18n.changeLanguage(language);
window.__qa.t = key => i18n.t(key);
const observedAt = Date.now();
function Publisher() { usePublishAssistantPage({viewKey:'overview:',page:'overview',scope:{kind:'household',label:'Synthetic Atlas household'},timezone:'UTC',basis:'base',currency:'CNY',observedAt,state:'available',targets:[],metrics:[{label:'Synthetic metric',value:'100',unit:'CNY',basis:'net_worth',status:'available',missingReasons:[]}]}); return null; }
function Content() { const {t}=useTranslation(); const assistant=useDesktopAssistant(); return <div className="flex h-screen min-w-0 bg-background text-foreground"><main className="min-w-0 flex-1 space-y-4 overflow-y-auto p-4"><header>{surface==='desktop'?assistant.trigger:<MobileAssistant blocked={false} disconnected={false}/>}</header><p role="status">{t('common.loading')}</p><MobileAccessSection/></main>{surface==='desktop'&&assistant.panel}</div>; }
createRoot(document.getElementById('root')).render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><AssistantPageProvider householdId="synthetic-atlas" incarnation="synthetic-incarnation" surface={surface} viewKey="overview:" blocked={()=>false} onNavigate={()=>{}}><Publisher/><Content/></AssistantPageProvider></QueryClientProvider>);
`);
const server = await createServer({ configFile: false, root: frontend, resolve: { alias: { '@': join(frontend, 'src') } }, plugins: [
  { name: 'synthetic-atlas-services', enforce: 'pre', resolveId(source, importer) {
    if (importer?.endsWith('/MobileAccessSection.tsx') && source.endsWith('/mobile/service')) return '\0atlas-mobile-service';
    if (importer?.endsWith('/DesktopAssistant.tsx') && source.endsWith('/wailsapi/assistant')) return '\0atlas-assistant-service';
  }, load(id) {
    if(id==='\0atlas-mobile-service') return `window.__qa={calls:[],rejectField:'token'}; const status={config:{enabled:true,listen:'192.168.1.20:9443',token:'SYNTHETIC1'},running:false}; export const Status=async()=>status; export const GenerateToken=async()=>'SYNTHETIC2'; export const Configure=async config=>{window.__qa.calls.push({...config});throw new Error(JSON.stringify({code:'validation',field:window.__qa.rejectField,message:'synthetic rejection'}));};`;
    if(id==='\0atlas-assistant-service') return 'export const Service={Status:async()=>('+JSON.stringify(status)+')};';
  } }, react(), tailwind(),
], server: { host: '127.0.0.1', port: 9347, strictPort: true } });
let browser;
const touch=process.env.ATLAS_TOUCH === "1";
const rows=[];
const cases=JSON.parse(process.env.ATLAS_BROWSER_CASES || '[["mobile",360],["mobile",1440],["desktop",768],["desktop",1440]]');
try {
  await server.listen();
  browser = await chromium.launch({executablePath: process.env.CHROMIUM_PATH || '/usr/bin/chromium',headless:true});
  const consoleErrors=[];
  for(const language of ['en','zh-CN','zh-TW']) for(const theme of ['light','dark']) for(const [surface,width] of cases) {
    const page=await browser.newPage({viewport:{width,height:900},colorScheme:theme,hasTouch:touch});
    page.on('pageerror',error=>consoleErrors.push(error.message));
    await page.route('**/mobile-api/v1/assistant/status',route=>route.fulfill({json:{data:{...status,surface:'mobile'}}}));
    await page.goto('http://127.0.0.1:9347/'+basename(scratch)+'/index.html?surface='+surface);
    await page.waitForFunction(()=>!!window.__qa?.language);
    await page.evaluate(async ({language,theme})=>{await window.__qa.language(language);document.documentElement.classList.toggle('dark',theme==='dark');},{language,theme});
    const t=key=>page.evaluate(key=>window.__qa.t(key),key);
    const token=page.locator('#mobile-access-token'); await token.waitFor();
    assert.equal(await page.locator('form').getAttribute('novalidate'),'');
    for(const [field,value] of [['token',''],['token','short'],['token','invalid!token'],['listen',''],['listen','0.0.0.0:9443']]) {
      await page.evaluate(field=>{window.__qa.rejectField=field;},field);
      const input=page.locator(field==='token'?'#mobile-access-token':'#mobile-listen');
      await input.fill(value);
      const before=await page.evaluate(()=>window.__qa.calls.length);
      await input.press('Enter');
      await page.waitForFunction(before=>window.__qa.calls.length===before+1,before);
      const message=await t('mobile.'+field+'Validation');
      await page.getByRole('alert').filter({hasText:message}).waitFor();
      assert.equal(await input.getAttribute('aria-invalid'),'true');
      assert.ok((await input.getAttribute('aria-describedby')).includes(field==='token'?'mobile-token-error':'mobile-listen-error'));
    }
    const calls=await page.evaluate(()=>window.__qa.calls.length);
    const next=language==='en'?'zh-CN':language==='zh-CN'?'zh-TW':'en';
    await page.evaluate(next=>window.__qa.language(next),next);
    await page.getByRole('alert').filter({hasText:await t('mobile.listenValidation')}).waitFor();
    assert.equal(await page.evaluate(()=>window.__qa.calls.length),calls);
    await page.evaluate(language=>window.__qa.language(language),language);
    assert.notEqual(await t('common.loading'),'common.loading');
    const trigger=page.getByRole('button',{name:await t('assistant.open'),exact:true});
    await trigger.focus(); await trigger.press('Enter');
    const explain=page.getByRole('button',{name:await t('assistant.explain'),exact:true}); await explain.waitFor();
    await explain.click();
    const evidence=page.locator('summary').filter({hasText:await t('assistant.evidence')}); await evidence.waitFor();
    await evidence.focus(); await evidence.press('Enter');
    assert.equal(await evidence.evaluate(el=>el.parentElement.open),true);
    await evidence.press('Space'); assert.equal(await evidence.evaluate(el=>el.parentElement.open),false);
    if(touch) { await evidence.tap(); assert.equal(await evidence.evaluate(el=>el.parentElement.open),true); await evidence.tap(); assert.equal(await evidence.evaluate(el=>el.parentElement.open),false); }
    const evidenceBox=await evidence.boundingBox();
    assert.ok(evidenceBox.height >= (surface==='mobile'||width<1280?44:20));
    const review=page.getByRole('button',{name:await t('assistant.review'),exact:true}); await review.click();
    const dialog=page.getByRole('dialog').filter({hasText:await t('assistant.reviewUnavailable')}); await dialog.waitFor();
    const close=dialog.getByRole('button',{name:await t('common.close'),exact:true});
    await page.waitForFunction(el => { const box=el.getBoundingClientRect(); return box.x>=0 && box.right<=innerWidth+1; }, await close.elementHandle());
    const closeBox=await close.boundingBox();
    assert.ok(closeBox.height >= (surface==='mobile'||width<1280?44:32));
    assert.ok(closeBox.x>=0&&closeBox.x+closeBox.width<=width+1);
    const headingBox=await dialog.getByRole('heading').boundingBox();
    assert.ok(headingBox.x+headingBox.width<=closeBox.x+1,'header overlap');
    await page.screenshot({path:join(output,language+'-'+theme+'-'+surface+'-'+width+'-review.png')});
    await close.focus(); await close.press('Enter'); await dialog.waitFor({state:'hidden'});
    await review.waitFor(); assert.equal(await review.evaluate(el=>el===document.activeElement),true);
    let topCloseHeight=null;
    if(surface==='mobile'||width<1280) {
      const top=page.getByRole('button',{name:await t('common.close'),exact:true}); topCloseHeight=(await top.boundingBox()).height;
      assert.ok(topCloseHeight>=44);
      await page.screenshot({path:join(output,language+'-'+theme+'-'+surface+'-'+width+'-evidence.png')});
      if(touch) await top.tap(); else { await top.focus(); await top.press('Escape'); }
      await page.getByRole('dialog').waitFor({state:'hidden'});
      assert.equal(await trigger.evaluate(el=>el===document.activeElement),true);
    } else {
      await page.screenshot({path:join(output,language+'-'+theme+'-'+surface+'-'+width+'-evidence.png')});
    }
    const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth);
    assert.equal(overflow,false,'horizontal document overflow');
    const clipping=await page.locator('[data-testid="assistant-conversation"]').evaluateAll(elements=>elements.filter(el=>el.scrollWidth>el.clientWidth+1).length);
    assert.equal(clipping,0,'assistant horizontal overflow');
    rows.push({language,theme,surface,width,evidenceHeight:evidenceBox.height,reviewCloseHeight:closeBox.height,topCloseHeight,keyboard:true,touchEmulated:touch,localizedValidation:true,languageSwitchWithoutResubmit:true,horizontalOverflow:false});
    await page.close();
  }
  assert.deepEqual(consoleErrors,[]);
  writeFileSync(join(output,'results.json'),JSON.stringify({browser:browser.version(),rows,consoleErrors,scope:'Real shared components with synthetic Wails/status adapters; Chromium keyboard/DOM semantics and layout, not native macOS or actual screen-reader testing.'},null,2));
  console.log(JSON.stringify({browser:browser.version(),cases:rows.length,output}));
} finally { await browser?.close(); await server.close(); rmSync(scratch,{recursive:true,force:true}); }
