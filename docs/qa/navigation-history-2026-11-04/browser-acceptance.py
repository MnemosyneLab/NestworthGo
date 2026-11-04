from playwright.sync_api import sync_playwright, expect
from pathlib import Path
import argparse
import json,re

parser = argparse.ArgumentParser(description="Exercise the desktop navigation against an isolated analytics QA server")
parser.add_argument("--qa-root", required=True, type=Path)
parser.add_argument("--base-url", default="http://127.0.0.1:18741")
parser.add_argument("--chromium", default="/usr/bin/chromium")
parser.add_argument("--allow-no-sandbox", action="store_true", help="Explicitly record a browser run without Chromium OS sandboxing")
args = parser.parse_args()
root=args.qa_root.resolve()
(root/"artifacts").mkdir(exist_ok=True)
seed=json.loads((root/'analytics/seed/seed-results.json').read_text())
assert seed.get("fixture_version") == "analytics-linux-qa-v3", "synthetic fixture marker required"
assert seed.get("scenario") == "complete-cny", "this journey expects the complete-cny synthetic fixture"
results={'fixture':'analytics-linux-qa-v3 / complete-cny','sandbox':'disabled (explicit --allow-no-sandbox)' if args.allow_no_sandbox else 'enabled','cases':[],'pageErrors':[],'consoleErrors':[],'failedResponses':[],'knownWarnings':[]}
def passed(name): results['cases'].append({'name':name,'passed':True})
with sync_playwright() as p:
 b=p.chromium.launch(executable_path=args.chromium,headless=True,chromium_sandbox=not args.allow_no_sandbox)
 page=b.new_page(viewport={'width':1280,'height':860})
 page.on('pageerror',lambda e:results['pageErrors'].append(str(e)))
 page.on('console',lambda msg:(results['knownWarnings'] if msg.location.get('url','').endswith('/favicon.ico') else results['consoleErrors']).append({'message':msg.text,'url':msg.location.get('url','')}) if msg.type=='error' else None)
 page.on('response',lambda r:results['failedResponses'].append({'url':r.url,'status':r.status}) if r.status>=400 else None)
 calls=[]
 page.on('request',lambda r:calls.append(r.url+' '+(r.post_data or '')) if '/wails/runtime' in r.url else None)
 page.goto(args.base_url)
 nav=page.get_by_role('navigation')
 controls=page.get_by_role('group',name='Navigation history')
 back=controls.get_by_role('button',name='Go back',exact=True)
 forward=controls.get_by_role('button',name='Go forward',exact=True)
 recent=controls.get_by_role('button',name='Recent pages',exact=True)
 expect(back).to_be_disabled(); expect(forward).to_be_disabled(); expect(recent).to_be_disabled()
 passed('empty history disabled')
 nav.get_by_role('button',name='Accounts',exact=True).click()
 page.get_by_role('button',name=re.compile('AUD Cash.*Bank account')).click()
 expect(page.get_by_role('heading',name='AUD Cash',exact=True)).to_be_visible()
 nav.get_by_role('button',name='Directory',exact=True).click()
 back.click(); expect(page.get_by_role('heading',name='AUD Cash',exact=True)).to_be_visible()
 back.click(); expect(page.get_by_role('checkbox',name='Show archived')).to_be_visible()
 forward.click(); expect(page.get_by_role('heading',name='AUD Cash',exact=True)).to_be_visible()
 passed('same-page account detail and list, Back and Forward')
 back.click(); nav.get_by_role('button',name='Settings',exact=True).click(); expect(forward).to_be_disabled()
 passed('Back then fresh navigation truncates Forward')
 recent.click(); item=page.get_by_role('menuitem',name='Accounts · AUD Cash',exact=True); expect(item).to_be_visible()
 assert len(page.get_by_role('menuitem',name='Accounts · AUD Cash',exact=True).all())==1
 page.keyboard.press('Escape'); expect(recent).to_be_focused()
 recent.press('ArrowDown'); item.click()
 expect(page.get_by_role('heading',name='AUD Cash',exact=True)).to_be_visible()
 back.click(); expect(nav.get_by_role('button',name='Settings',exact=True)).to_have_attribute('aria-current','page')
 passed('Recent restores business context as a new visit; Escape and keyboard reopen')
 nav.get_by_role('button',name='Accounts',exact=True).click()
 archive=page.get_by_role('checkbox',name='Show archived'); archive.check()
 page.get_by_role('button',name=re.compile('AUD Cash.*Bank account')).click()
 back.click(); expect(archive).to_be_checked()
 passed('account list filter retained without per-input navigation steps')
 nav.get_by_role('button',name='Wealth history',exact=True).click()
 page.get_by_role('button',name='Historical date',exact=True).wait_for()
 page.wait_for_function('!document.querySelector("main")?.innerText.includes("Loading historical overview")')
 # Settled historical capture is session-owned, irrespective of the current valuation query cache.
 page.wait_for_timeout(350)
 count=sum('1841438157' in c for c in calls)
 assert count>=1, 'real historical service call required'
 nav.get_by_role('button',name='Accounts',exact=True).click(); back.click()
 page.get_by_role('button',name='Historical date',exact=True).wait_for()
 page.wait_for_timeout(150)
 assert sum('1841438157' in c for c in calls)==count, 'Back must not refreeze capture'
 passed('historical capture reused through Back with no new service call')
 forward.click(); nav.get_by_role('button',name='Wealth history',exact=True).click()
 page.get_by_role('button',name='Historical date',exact=True).wait_for()
 page.wait_for_function('document.querySelectorAll("header h1").length===1')
 passed('independent historical visits have only one active header')
 recent.click(); page.screenshot(path=str(root/'artifacts/desktop-recent.png')); page.keyboard.press('Escape')
 page.set_viewport_size({'width':640,'height':860})
 recent.click(); popup=page.get_by_role('menu'); box=popup.bounding_box()
 assert box and box['x']>=0 and box['x']+box['width']<=640.5
 assert page.evaluate('document.documentElement.scrollWidth<=innerWidth')
 page.screenshot(path=str(root/'artifacts/narrow-recent.png')); page.keyboard.press('Escape')
 passed('640px desktop rail and bounded Recent popup without horizontal overflow')
 # The same canonical native language select remains available in the collapsed rail.
 for code,group_name,back_name,forward_name,recent_name in [('zh-CN','导航历史','后退','前进','最近访问'),('zh-TW','導覽歷史','上一頁','下一頁','最近造訪'),('en','Navigation history','Go back','Go forward','Recent pages')]:
  page.locator('aside select').select_option(code)
  translated=page.get_by_role('group',name=group_name)
  expect(translated.get_by_role('button',name=back_name,exact=True)).to_be_visible()
  expect(translated.get_by_role('button',name=forward_name,exact=True)).to_be_visible()
  translated.get_by_role('button',name=recent_name,exact=True).click()
  expect(page.get_by_role('menu')).to_be_visible()
  page.keyboard.press('Escape')
 passed('English, Simplified Chinese and Traditional Chinese controls and menu')
 page.reload()
 expect(page.get_by_role('button',name='Go back',exact=True)).to_be_disabled()
 expect(page.get_by_role('button',name='Go forward',exact=True)).to_be_disabled()
 expect(page.get_by_role('button',name='Recent pages',exact=True)).to_be_disabled()
 passed('reload clears all navigation and Recent')
 results['runtimeCalls']=len(calls)
 assert not results['pageErrors'],results['pageErrors']
 assert not results['consoleErrors'],results['consoleErrors']
 assert not results['failedResponses'],results['failedResponses']
 b.close()
(root/'artifacts/browser-results.json').write_text(json.dumps(results,ensure_ascii=False,indent=2)+'\n')
print(json.dumps(results,ensure_ascii=False,indent=2))
