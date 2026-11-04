// Runs the opt-in existing Go synthetic HTTP fixture against the production
// frontend bundle. Requires Go/Node on PATH and a prior frontend production build.
import {spawn} from 'node:child_process';
import {mkdtempSync,readFileSync,writeFileSync,existsSync,mkdirSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join,resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import assert from 'node:assert/strict';
const repo=resolve(dirname(fileURLToPath(import.meta.url)),'../../..');
const {chromium}=await import(process.env.PLAYWRIGHT_MODULE||'playwright');
const output=resolve(process.env.ATLAS_BROWSER_OUTPUT||'/tmp/atlas-http-evidence');
mkdirSync(output,{recursive:true});
const browser=await chromium.launch({executablePath:process.env.CHROMIUM_PATH||'/usr/bin/chromium',headless:true});
const rows=[];
try {
 for(const language of ['en','zh-CN','zh-TW']) for(const theme of ['light','dark']) {
  const directory=mkdtempSync(join(tmpdir(),'atlas-synthetic-http-'));
  const child=spawn('go',['test','./internal/mobileapi','-run','^TestMobileBrowserFixture$','-count=1','-timeout','6m'],{cwd:repo,env:{...process.env,NESTWORTH_MOBILE_BROWSER_FIXTURE:directory},stdio:['ignore','pipe','pipe']});
  let logs='';child.stdout.on('data',data=>logs+=data);child.stderr.on('data',data=>logs+=data);
  const finished=new Promise(resolve=>child.on('exit',(code,signal)=>resolve({code,signal})));
  let page;
  try {
   const connection=join(directory,'connection.json');
   const deadline=Date.now()+60000;
   while(!existsSync(connection)){assert.ok(Date.now()<deadline,'fixture startup timeout: '+logs);await new Promise(resolve=>setTimeout(resolve,100));}
   const {url}=JSON.parse(readFileSync(connection,'utf8'));
   page=await browser.newPage({viewport:{width:360,height:900},colorScheme:theme});
   await page.addInitScript(({language,theme})=>localStorage.setItem('nestworth.mobile.preferences.v1',JSON.stringify({language,theme})),{language,theme});
   let release;
   const gate=new Promise(resolve=>release=resolve);
   let held=false;
   await page.route('**/mobile-api/v1/session',async route=>{if(route.request().method()==='GET'&&!held){held=true;await gate;}await route.continue();});
   await page.goto(url);
   const locale=JSON.parse(readFileSync(join(repo,'frontend/src/i18n/locales',language+'.json'),'utf8'));
   await page.getByRole('status').filter({hasText:locale.common.loading}).waitFor();
   assert.equal(await page.getByText('common.loading',{exact:true}).count(),0);
   release();
   const input=page.locator('#mobile-token');await input.waitFor();
   const responses=[];
   for(let attempt=2;attempt<=6;attempt++) {
    await input.fill('SYNTHETIC_WRONG_TOKEN');
    const pending=page.waitForResponse(response=>response.url().endsWith('/session')&&response.request().method()==='POST');
    await input.press('Enter');
    const response=await pending;const body=await response.json();
    const code=attempt===6?'rate_limited':'invalid_token';
    assert.equal(response.status(),attempt===6?429:401);assert.equal(body.error.code,code);
    assert.equal(body.error.operationState,'unknown');
    await page.getByRole('alert').filter({hasText:locale.mobile.errors[code]}).waitFor();
    const text=await page.getByRole('alert').textContent();
    assert.ok(!/·|rateLimit|errorCode:|error\.field/.test(text));
    assert.equal(await input.inputValue(),'');
    responses.push({attempt,status:response.status(),code,operationState:body.error.operationState});
   }
   const expired=await page.request.get(url+'/mobile-api/v1/session',{headers:{Authorization:'Bearer synthetic-invalid-session'}});
   assert.equal(expired.status(),401);assert.equal((await expired.json()).error.code,'session_expired');
   await page.screenshot({path:join(output,language+'-'+theme+'-429.png')});
   writeFileSync(join(output,language+'-'+theme+'-accessibility.yml'),await page.locator('main').ariaSnapshot());
   rows.push({language,theme,width:360,responses,loadingLabel:locale.common.loading,expiredSessionStatus:401});
  } finally {
   await page?.close();writeFileSync(join(directory,'done'),'done');
   const result=await finished;
   writeFileSync(join(output,language+'-'+theme+'-fixture.log'),logs);
   rmSync(directory,{recursive:true,force:true});
   assert.equal(result.code,0,'fixture failed: '+logs);
  }
 }
 writeFileSync(join(output,'results.json'),JSON.stringify({browser:browser.version(),rows,scope:'Production mobile bundle and real isolated Go HTTP fixture, synthetic data only. No live household, providers or credentials.'},null,2));
 console.log(JSON.stringify({browser:browser.version(),cases:rows.length,output}));
} finally {await browser.close();}
