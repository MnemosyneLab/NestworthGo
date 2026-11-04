from playwright.sync_api import sync_playwright, expect
from pathlib import Path
import json, argparse, re, sqlite3, hashlib

parser = argparse.ArgumentParser()
parser.add_argument('--desktop', default='http://127.0.0.1:18861')
parser.add_argument('--root', type=Path, default=Path('/tmp/nestworth-ai-qa'))
parser.add_argument('--allow-no-sandbox', action='store_true')
args = parser.parse_args()
root = args.root.resolve()
out = root / 'artifacts'
out.mkdir(exist_ok=True)
seed = json.loads((root/'analytics/seed/seed-results.json').read_text())
assert seed['fixture_version'] == 'analytics-linux-qa-v3' and seed['scenario'] == 'complete-cny'
results = {'cases': [], 'pageErrors': [], 'consoleErrors': [], 'knownWarnings': [], 'externalRequests': [], 'sandbox': 'enabled'}
def financial_facts():
    connection = sqlite3.connect(f'file:{root}/analytics/data/nestworth.db?mode=ro', uri=True)
    facts = {}
    for table in ['activities','activity_effects','account_values','account_cash_values','holdings','holding_quantity_values','activity_mutation_keys']:
        rows = sorted(repr(tuple(row)) for row in connection.execute(f'SELECT * FROM {table}'))
        facts[table] = {'rows':len(rows),'sha256':hashlib.sha256('\n'.join(rows).encode()).hexdigest()}
    connection.close()
    return facts
results['financialFactsBefore'] = financial_facts()
def passed(name): results['cases'].append({'name': name, 'passed': True})
def watch(page):
    page.on('pageerror', lambda e: results['pageErrors'].append(str(e)))
    def console(m):
        if m.type != 'error': return
        entry={'message':m.text,'url':m.location.get('url','')}
        expected = entry['url'].endswith('/favicon.ico') or (entry['url'].endswith('/mobile-api/v1/session') and '401' in m.text)
        results['knownWarnings' if expected else 'consoleErrors'].append(entry)
    page.on('console', console)
    page.on('request', lambda r: results['externalRequests'].append(r.url) if not r.url.startswith(('http://127.0.0.1:', 'ws://127.0.0.1:', 'data:')) else None)
with sync_playwright() as p:
    try:
        browser = p.chromium.launch(executable_path='/usr/bin/chromium', headless=True, chromium_sandbox=True)
    except Exception as e:
        results['sandboxFailure'] = str(e)[:1200]
        assert args.allow_no_sandbox, 'OS sandbox failed; explicit fallback required'
        results['sandbox'] = 'disabled (explicit --allow-no-sandbox after recorded failure)'
        browser = p.chromium.launch(executable_path='/usr/bin/chromium', headless=True, chromium_sandbox=False)
    context = browser.new_context(viewport={'width':1440,'height':960})
    page = context.new_page(); watch(page)
    try:
        page.goto(args.desktop)
        expect(page.get_by_test_id('overview-net-worth')).to_be_visible()
        page.locator('aside').first.get_by_role('combobox').select_option('en')
        page.get_by_role('button', name='Open AI assistant', exact=True).click()
        panel = page.get_by_role('complementary', name='AI assistant')
        expect(panel.get_by_text('Local page guide ready', exact=True)).to_be_visible()
        panel.get_by_role('button', name='Explain selected metric').click()
        expect(panel.get_by_text('Several numbers may match. Choose the metric you mean.')).to_be_visible()
        panel.get_by_label('Visible metric').select_option(label='Net worth')
        panel.get_by_role('button', name='Explain selected metric').focus()
        page.keyboard.press('Enter')
        expect(panel.get_by_text('Local page guide · no model')).to_be_visible()
        panel.get_by_text('Data evidence', exact=True).click()
        expect(panel.get_by_text('Saved snapshot, not a live balance. It cannot authorize a later action.')).to_be_visible()
        expect(panel.get_by_label('Message', exact=True)).to_be_disabled()
        box = panel.bounding_box(); main = page.locator('#main-content').bounding_box()
        assert box['x'] >= main['x'] + main['width'] - 1 and box['width'] == 400
        page.screenshot(path=str(out/'desktop-guide.png'))
        passed('desktop real-service page evidence, metric disambiguation, keyboard and dock geometry')
        panel.get_by_role('button', name='Expand workspace').click()
        expect(page.locator('#main-content')).to_be_hidden()
        panel.get_by_role('button', name='Return to page').click()
        expect(page.locator('#main-content')).to_be_visible()
        passed('expanded workspace retains the business page')
        context.set_offline(True)
        expect(panel.get_by_text('Disconnected. Actions are paused; reconnect does not replay them.')).to_be_visible()
        expect(panel.get_by_role('button', name='Explain selected metric')).to_be_disabled()
        context.set_offline(False)
        expect(panel.get_by_text('Local page guide ready', exact=True)).to_be_visible()
        assert panel.get_by_text('Local page guide · no model', exact=True).count() == 1
        passed('desktop disconnect pauses without replay')
        nav = page.get_by_role('navigation', name='Main navigation')
        nav.get_by_role('button', name='Accounts', exact=True).click()
        candidates = panel.get_by_text('Open an account', exact=True).locator('..').get_by_role('button')
        expect(candidates.first).to_be_visible()
        candidates.first.click()
        history = page.get_by_role('group', name='Navigation history')
        expect(history.get_by_role('button', name='Go back', exact=True)).to_be_enabled()
        history.get_by_role('button', name='Go back', exact=True).click()
        expect(panel.get_by_text('Open an account', exact=True)).to_be_visible()
        passed('account navigation uses existing Back history')
        panel.get_by_role('button', name='Financial review', exact=True).click()
        expect(page.get_by_role('button', name='AI confirmation unavailable')).to_be_disabled()
        page.keyboard.press('Escape')
        expect(panel.get_by_role('button', name='Financial review', exact=True)).to_be_focused()
        panel.get_by_role('button', name='Collapse assistant').click()
        expect(page.get_by_role('button', name='Open AI assistant', exact=True)).to_be_focused()
        passed('separate closed financial review and focus restoration')
        page.set_viewport_size({'width':640,'height':860})
        page.get_by_role('button', name='Open AI assistant', exact=True).click()
        dialog = page.get_by_role('dialog', name='AI assistant', exact=True)
        expect(dialog).to_be_visible()
        page.wait_for_function("() => { const e=document.getElementById('desktop-assistant'); const r=e?.getBoundingClientRect(); return r && r.x>=0 && r.right<=window.innerWidth+1; }")
        box = dialog.bounding_box()
        assert box['x'] >= 0 and box['x'] + box['width'] <= 641
        title_box = dialog.get_by_role('heading', name='AI assistant', exact=True).bounding_box()
        assert title_box['x'] >= box['x'] and title_box['x'] + title_box['width'] <= 641
        page.screenshot(path=str(out/'narrow-guide.png'))
        page.keyboard.press('Escape')
        expect(page.get_by_role('button', name='Open AI assistant', exact=True)).to_be_focused()
        passed('640px shared Sheet bounds and Escape focus')
        page.set_viewport_size({'width':1440,'height':960})
        for locale, title, open_label, collapse in [('zh-CN','AI 助手','打开 AI 助手','收起助手'), ('zh-TW','AI 助手','開啟 AI 助手','收合助手')]:
            page.locator('aside').first.get_by_role('combobox').select_option(locale)
            page.get_by_role('button', name=open_label, exact=True).click()
            localized = page.get_by_role('complementary', name=title)
            expect(localized.get_by_role('heading', name=title, exact=True)).to_be_visible()
            page.screenshot(path=str(out/f'desktop-{locale}.png'))
            localized.get_by_role('button', name=collapse, exact=True).click()
            passed(f'desktop {locale} localized controls')
        mobile_info = json.loads((root/'mobile/connection.json').read_text())
        phone_context = browser.new_context(viewport={'width':390,'height':844}, is_mobile=True, has_touch=True)
        phone = phone_context.new_page(); watch(phone)
        phone.goto(mobile_info['url'])
        phone.get_by_label('Mobile token', exact=True).fill(mobile_info['token'])
        phone.get_by_role('button', name='Sign in', exact=True).click()
        expect(phone.get_by_role('navigation', name='Main navigation').get_by_role('button', name='Quick record', exact=True)).to_be_visible()
        phone.get_by_role('button', name='Open AI assistant', exact=True).click()
        mobile_dialog = phone.get_by_role('dialog', name='AI assistant', exact=True)
        expect(mobile_dialog.get_by_text('Local page guide ready', exact=True)).to_be_visible()
        mobile_dialog.get_by_label('Visible metric').select_option(label='Net worth')
        mobile_dialog.get_by_role('button', name='Explain selected metric').click()
        expect(mobile_dialog.get_by_text('Local page guide · no model')).to_be_visible()
        phone.wait_for_function("() => { const e=document.getElementById('mobile-assistant'); const r=e?.getBoundingClientRect(); return r && r.x>=0 && r.right<=window.innerWidth+1; }")
        title_box = mobile_dialog.get_by_role('heading', name='AI assistant', exact=True).bounding_box()
        assert title_box['x'] >= 0 and title_box['x']+title_box['width'] <= 390
        results['mobileControlHeights'] = {name: mobile_dialog.get_by_label(name, exact=True).bounding_box()['height'] for name in ['Visible metric']}
        for name in ['Refresh page evidence','Explain selected metric']:
            results['mobileControlHeights'][name] = mobile_dialog.get_by_role('button',name=name,exact=True).bounding_box()['height']
        assert all(height >= 44 for height in results['mobileControlHeights'].values())
        phone.screenshot(path=str(out/'mobile-guide-390.png'))
        mobile_dialog.get_by_role('button', name='Financial review', exact=True).click()
        expect(phone.get_by_role('button', name='AI confirmation unavailable')).to_be_disabled()
        review = phone.get_by_role('dialog', name='Financial review', exact=True)
        phone.wait_for_function("(id) => {const r=document.getElementById(id)?.getBoundingClientRect();return r&&r.x>=0&&r.right<=window.innerWidth+1;}", arg=review.get_attribute('id'))
        phone.screenshot(path=str(out/'mobile-review-390.png'))
        phone.keyboard.press('Escape')
        expect(phone.get_by_role('dialog', name='Financial review', exact=True)).to_be_hidden()
        phone.keyboard.press('Escape')
        expect(phone.get_by_role('dialog', name='AI assistant', exact=True)).to_be_hidden()
        expect(phone.get_by_role('navigation', name='Main navigation').get_by_role('button', name='Quick record', exact=True)).to_be_visible()
        passed('authenticated mobile guide and separate closed review preserve recording')
        phone.get_by_role('navigation', name='Main navigation').get_by_role('button', name='Accounts', exact=True).click()
        phone.get_by_role('button', name='Open AI assistant', exact=True).click()
        mobile_dialog = phone.get_by_role('dialog', name='AI assistant', exact=True)
        mobile_dialog.get_by_text('Open an account', exact=True).locator('..').get_by_role('button').first.click()
        expect(phone.get_by_role('dialog')).to_have_count(0)
        expect(phone.get_by_role('navigation', name='Main navigation').get_by_role('button', name='Quick record', exact=True)).to_be_visible()
        passed('mobile semantic account navigation dismisses conversation and shows business page')
        phone.set_viewport_size({'width':320,'height':700})
        phone.get_by_role('button', name='Open AI assistant', exact=True).click()
        mobile_dialog = phone.get_by_role('dialog', name='AI assistant', exact=True)
        expect(mobile_dialog).to_be_visible()
        phone.wait_for_function("() => { const e=document.getElementById('mobile-assistant'); const r=e?.getBoundingClientRect(); return r && r.x>=0 && r.right<=window.innerWidth+1; }")
        box = mobile_dialog.bounding_box()
        assert box['x'] >= 0 and box['x'] + box['width'] <= 321
        phone.screenshot(path=str(out/'mobile-guide-320.png'))
        phone_context.set_offline(True)
        expect(mobile_dialog.get_by_text('Disconnected. Actions are paused; reconnect does not replay them.')).to_be_visible()
        phone_context.set_offline(False)
        passed('320px mobile bounds and offline state')
        phone.keyboard.press('Escape')
        expect(phone.get_by_role('dialog', name='AI assistant', exact=True)).to_be_hidden()
        phone.get_by_role('button', name='Settings', exact=True).click()
        for locale, open_label in [('zh-CN','打开 AI 助手'), ('zh-TW','開啟 AI 助手')]:
            phone.locator('#mobile-settings').get_by_role('combobox').first.select_option(locale)
            phone.get_by_role('button', name=open_label, exact=True).click()
            localized = phone.get_by_role('dialog', name='AI 助手', exact=True)
            phone.wait_for_function("() => {const r=document.getElementById('mobile-assistant')?.getBoundingClientRect();return r&&r.x>=0&&r.right<=window.innerWidth+1;}")
            expect(localized.get_by_role('heading', name='AI 助手', exact=True)).to_be_visible()
            phone.screenshot(path=str(out/f'mobile-{locale}-320.png'))
            phone.keyboard.press('Escape')
            expect(localized).to_be_hidden()
            passed(f'320px mobile {locale} localized controls')
        results['financialFactsAfter'] = financial_facts()
        assert results['financialFactsBefore'] == results['financialFactsAfter'], 'page guide changed ledger facts'
        passed('desktop financial facts remain byte-identical across guide/navigation/review interactions')
        assert not results['pageErrors'], results['pageErrors']
        assert not results['consoleErrors'], results['consoleErrors']
        assert not results['externalRequests'], results['externalRequests']
    except Exception as e:
        results['failure'] = str(e)
        page.screenshot(path=str(out/'failure.png'))
        raise
    finally:
        (out/'browser-results.json').write_text(json.dumps(results, ensure_ascii=False, indent=2)+'\n')
        browser.close()
