"""Run only against same-page-build.mjs's fresh synthetic fixture and HTTP host."""
import argparse
import json
import re
from pathlib import Path
from playwright.sync_api import sync_playwright, expect

parser = argparse.ArgumentParser()
parser.add_argument('--qa-root', required=True, type=Path)
parser.add_argument('--base-url', default='http://127.0.0.1:18744')
parser.add_argument('--allow-no-sandbox', action='store_true')
args = parser.parse_args()
root = args.qa_root.resolve()
assert (root / 'SYNTHETIC_TEST_ONLY').read_text().startswith('Same-page navigation')
results = {'fixture': 'Same-page navigation synthetic QA / 61 accounts / 40 holdings',
           'sandbox': 'disabled explicitly' if args.allow_no_sandbox else 'enabled',
           'cases': [], 'pageErrors': [], 'consoleErrors': []}
artifacts = root / 'artifacts'
artifacts.mkdir(exist_ok=True)

def passed(name):
    results['cases'].append({'name': name, 'passed': True})

with sync_playwright() as p:
    browser = p.chromium.launch(executable_path='/usr/bin/chromium', headless=True,
                                chromium_sandbox=not args.allow_no_sandbox)
    page = browser.new_page(viewport={'width': 1280, 'height': 900})
    page.on('pageerror', lambda e: results['pageErrors'].append(str(e)))
    page.on('console', lambda m: results['consoleErrors'].append(m.text) if m.type == 'error' and not m.location.get('url', '').endswith('/favicon.ico') else None)
    calls = []
    page.on('request', lambda r: calls.append(r.url + ' ' + (r.post_data or '')) if '/wails/runtime' in r.url else None)
    page.goto(args.base_url)
    nav = page.get_by_role('navigation', name='Main navigation', exact=True)
    controls = page.get_by_role('group', name='Navigation history', exact=True)
    back = controls.locator('button').nth(0)
    forward = controls.locator('button').nth(1)
    expect(back).to_be_disabled()
    expect(forward).to_be_disabled()
    passed('empty history has disabled traversal')
    nav.get_by_role('button', name='Accounts', exact=True).click()
    archive = page.get_by_role('checkbox', name='Show archived', exact=True)
    archive.check()
    main = page.locator('#main-content')
    short = page.get_by_role('button', name=re.compile('QA 20 Short.*Bank account'))
    short.scroll_into_view_if_needed()
    short.evaluate('e => e.addEventListener("pointerdown", () => { window.__savedScroll=document.getElementById("main-content").scrollTop }, {once:true})')
    short.click()
    expect(page.get_by_role('heading', name='QA 20 Short', exact=True)).to_be_visible()
    saved = page.evaluate('window.__savedScroll')
    assert saved > 0
    breadcrumb = page.get_by_role('button', name='Back to accounts', exact=True)
    expect(breadcrumb).to_have_text('Accounts')
    expect(back).to_have_accessible_name('Go back to Accounts')
    assert main.get_by_role('button', name=re.compile('Go back to')).count() == 0
    # Both paths restore the original filtered list and established main-content focus fallback.
    breadcrumb.click()
    expect(archive).to_be_checked()
    expect(main).to_be_focused()
    assert main.evaluate('e => e.scrollTop') == saved
    forward.click()
    expect(page.get_by_role('heading', name='QA 20 Short', exact=True)).to_be_visible()
    back.click()
    expect(forward).to_be_focused()
    passed('list/detail breadcrumb and Back/Forward retain filter, real scroll and focus fallback')
    short.click()
    nav.get_by_role('button', name='Settings', exact=True).click()
    controls.get_by_role('button', name='Recent pages', exact=True).click()
    page.get_by_role('menuitem', name='Accounts · QA 20 Short', exact=True).click()
    expect(page.get_by_role('heading', name='QA 20 Short', exact=True)).to_be_visible()
    back.click()
    expect(nav.get_by_role('button', name='Settings', exact=True)).to_have_attribute('aria-current', 'page')
    passed('Recent opens an object as a new visit with Back to selection source')
    nav.get_by_role('button', name='Wealth history', exact=True).click()
    exit_action = page.get_by_role('button', name='View current overview', exact=True)
    exit_action.wait_for()
    page.get_by_role('button', name='Historical date', exact=True).wait_for()
    page.get_by_role('status', name='Reconstructing wealth history…', exact=True).wait_for(state='hidden')
    page.wait_for_timeout(400)
    capture_reads = sum('1841438157' in call for call in calls)
    assert capture_reads >= 1
    exit_action.click()
    expect(nav.get_by_role('button', name='Overview', exact=True)).to_have_attribute('aria-current', 'page')
    expect(main).to_be_focused()
    expect(back).to_have_accessible_name(re.compile('Go back to Wealth history'))
    back.click()
    expect(exit_action).to_be_visible()
    page.wait_for_timeout(200)
    assert sum('1841438157' in call for call in calls) == capture_reads
    forward.click()
    expect(nav.get_by_role('button', name='Overview', exact=True)).to_have_attribute('aria-current', 'page')
    passed('history entered from Settings targets current Overview and restores its frozen capture without a service re-read')
    nav.get_by_role('button', name='Accounts', exact=True).click()
    page.get_by_role('button', name=re.compile('QA Long holdings.*Brokerage')).click()
    expect(page.get_by_role('heading', name='QA Long holdings', exact=True)).to_be_visible()
    page.set_viewport_size({'width': 640, 'height': 860})
    for language, group_name, list_name, parent_name in [('zh-CN', '导航历史', '返回账户列表', '账户'), ('zh-TW', '導覽歷史', '返回帳戶列表', '帳戶'), ('en', 'Navigation history', 'Back to accounts', 'Accounts')]:
        page.locator('aside select').select_option(language)
        translated = page.get_by_role('group', name=group_name, exact=True)
        expect(translated.locator('button').nth(0)).to_be_visible()
        expect(page.get_by_role('button', name=list_name, exact=True)).to_have_text(parent_name)
        assert translated.locator('button').nth(0).get_attribute('title') == translated.locator('button').nth(0).get_attribute('aria-label')
        # Business name and parent text remain visible at the desktop rail width.
        expect(page.get_by_role('heading', name='QA Long holdings', exact=True)).to_be_visible()
        assert page.evaluate('document.documentElement.scrollWidth <= innerWidth')
        page.screenshot(path=str(artifacts / f'account-640-{language}.png'))
        translated.locator('button').nth(2).click()
        popup = page.get_by_role('menu')
        box = popup.bounding_box()
        assert box and box['x'] >= 0 and box['x'] + box['width'] <= 640.5
        page.keyboard.press('Escape')
        expect(translated.locator('button').nth(2)).to_be_focused()
        passed(f'{language}: 640px breadcrumb, destination labels and keyboard Recent without overflow')
    page.reload()
    expect(page.get_by_role('button', name='Go back', exact=True)).to_be_disabled()
    passed('reload clears history')
    assert not results['pageErrors'], results['pageErrors']
    assert not results['consoleErrors'], results['consoleErrors']
    browser.close()
(artifacts / 'browser-results.json').write_text(json.dumps(results, ensure_ascii=False, indent=2) + '\n')
