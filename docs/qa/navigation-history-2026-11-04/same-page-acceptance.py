"""Real CSS geometry and native scroll clamping with a forced store commit.

Requires same-page-build.mjs's isolated synthetic HTTP app. The native scrollTop
setter is traced, never replaced with a fake layout or clamping implementation.
"""
import argparse
import json
import re
from pathlib import Path
from playwright.sync_api import sync_playwright

parser = argparse.ArgumentParser()
parser.add_argument('--qa-root', required=True, type=Path)
parser.add_argument('--base-url', default='http://127.0.0.1:18743')
parser.add_argument('--allow-no-sandbox', action='store_true')
args = parser.parse_args()
root = args.qa_root.resolve()
assert (root / 'SYNTHETIC_TEST_ONLY').read_text().startswith('Same-page navigation')
results = []
with sync_playwright() as p:
    browser = p.chromium.launch(executable_path='/usr/bin/chromium', headless=True, chromium_sandbox=not args.allow_no_sandbox)
    for route in ['back', 'recent']:
        page = browser.new_page(viewport={'width': 1280, 'height': 900})
        page.goto(args.base_url)
        nav = page.get_by_role('navigation')
        nav.get_by_role('button', name='Accounts', exact=True).click()
        main = page.locator('#main-content')
        short = page.get_by_role('button', name=re.compile('QA 20 Short.*Bank account'))
        short.wait_for()
        if route == 'recent':
            page.get_by_role('button', name=re.compile('QA Long holdings.*Brokerage')).click()
            page.get_by_role('heading', name='QA Long holdings', exact=True).wait_for()
            page.get_by_text('Synthetic holding 39', exact=True).wait_for()
            saved = main.evaluate('(e) => { e.scrollTop=900; return e.scrollTop }')
            assert saved == 900
            nav.get_by_role('button', name='Accounts', exact=True).click()
            short.wait_for()
        else:
            short.evaluate('(e) => e.scrollIntoView({block:"center",behavior:"instant"})')
            saved = main.evaluate('(e) => e.scrollTop')
            assert saved > 500
        if route == 'back':
            short.evaluate('e => e.addEventListener("pointerdown", () => { window.__sourceScroll=document.getElementById("main-content").scrollTop }, {once:true})')
        short.click()
        if route == 'back':
            saved = page.evaluate('window.__sourceScroll')
            assert saved > 500
        page.get_by_role('heading', name='QA 20 Short', exact=True).wait_for()
        page.wait_for_timeout(200)
        short_size = main.evaluate('(e) => ({height:e.scrollHeight,client:e.clientHeight})')
        assert saved > short_size['height'] - short_size['client']
        main.evaluate('''e => {
            let proto = e; let descriptor;
            while (proto && !descriptor) { descriptor=Object.getOwnPropertyDescriptor(proto,'scrollTop'); proto=Object.getPrototypeOf(proto) }
            window.__scrollTrace=[];
            Object.defineProperty(e,'scrollTop',{configurable:true,
                get(){return descriptor.get.call(this)},
                set(value){descriptor.set.call(this,value);window.__scrollTrace.push({requested:value,actual:descriptor.get.call(this),height:this.scrollHeight,client:this.clientHeight})}});
            window.__samePageQA.enable();
        }''')
        if route == 'back':
            page.get_by_role('button', name='Go back', exact=True).click()
            short.wait_for()
        else:
            page.get_by_role('button', name='Recent pages', exact=True).click()
            page.get_by_role('menuitem', name=re.compile('Accounts.*QA Long holdings')).click()
            page.get_by_role('heading', name='QA Long holdings', exact=True).wait_for()
            page.get_by_text('Synthetic holding 39', exact=True).wait_for()
        page.wait_for_timeout(300)
        actual = main.evaluate('(e) => e.scrollTop')
        result = {'route': route, 'saved': saved, 'actual': actual, 'short': short_size,
                  'destination': main.evaluate('(e)=>({height:e.scrollHeight,client:e.clientHeight})'),
                  'writes': page.evaluate('window.__scrollTrace'), 'passed': actual == saved}
        results.append(result)
        page.screenshot(path=str(root / f'{route}-same-page.png'))
        page.close()
    browser.close()
(root / 'same-page-results.json').write_text(json.dumps(results, indent=2)+'\n')
print(json.dumps(results, indent=2))
assert all(result['passed'] for result in results), 'native scroll clamping lost the requested position'
