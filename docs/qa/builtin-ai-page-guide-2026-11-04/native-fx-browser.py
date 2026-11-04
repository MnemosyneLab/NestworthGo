from pathlib import Path
from playwright.sync_api import sync_playwright, expect
import argparse, json, re

parser=argparse.ArgumentParser()
parser.add_argument('--root', type=Path, required=True)
parser.add_argument('--output', type=Path, required=True)
args=parser.parse_args()
info=json.loads((args.root/'connection.json').read_text())
args.output.mkdir(parents=True, exist_ok=True)
result={'fixture':'SYNTHETIC MOBILE TEST','browserSandbox':'disabled; cloud sandbox failure recorded in initial acceptance','pageErrors':[],'writeRequests':[]}
with sync_playwright() as p:
 browser=p.chromium.launch(executable_path='/usr/bin/chromium',headless=True,chromium_sandbox=False)
 try:
  page=browser.new_page(viewport={'width':390,'height':844},is_mobile=True,has_touch=True)
  page.on('pageerror',lambda error:result['pageErrors'].append(str(error)))
  page.on('request',lambda request:result['writeRequests'].append(request.url) if request.method not in ['GET','HEAD'] and not request.url.endswith('/session') else None)
  page.goto(info['url']);page.get_by_label('Mobile token',exact=True).fill(info['token']);page.get_by_role('button',name='Sign in',exact=True).click()
  nav=page.get_by_role('navigation',name='Main navigation');nav.get_by_role('button',name='Accounts',exact=True).click()
  with page.expect_response(lambda r:'/mobile-api/v1/accounts/' in r.url and r.status==200) as response:
   page.get_by_role('button',name=re.compile('Synthetic USD savings')).click()
  account=response.value.json()['data'];display=account['displayValue']
  assert account['complete'] is False and account.get('baseValue') is None
  assert display['complete'] is True and display['value']['currency']=='USD' and display['value']['amount']=='100' and display['missingInputs']==[]
  result['wire']={'householdComplete':account['complete'],'householdMissingKinds':[x['kind'] for x in account['missingInputs']],'nativeComplete':display['complete'],'nativeValue':display['value'],'nativeMissingInputs':display['missingInputs']}
  page.get_by_role('button',name='Open AI assistant',exact=True).click()
  dialog=page.get_by_role('dialog',name='AI assistant',exact=True)
  expect(dialog.get_by_text('Local page guide ready',exact=True)).to_be_visible()
  dialog.get_by_label('Visible metric').select_option(label='Synthetic USD savings')
  dialog.get_by_role('button',name='Explain selected metric',exact=True).click()
  conversation=dialog.get_by_label('Page explanations')
  expect(conversation.get_by_text('Available in this snapshot.',exact=True)).to_be_visible()
  expect(conversation.get_by_text('Known subtotal or partial estimate; this is not a complete total.',exact=True)).to_have_count(0)
  conversation.scroll_into_view_if_needed()
  page.screenshot(path=str(args.output/'native-fx-guide-390.png'))
  conversation.get_by_text('Data evidence',exact=True).click()
  expect(conversation.get_by_text('No missing inputs reported for this figure.',exact=True)).to_be_visible()
  expect(conversation.get_by_text('USD',exact=True)).to_be_visible()
  conversation.get_by_text('USD',exact=True).scroll_into_view_if_needed()
  page.screenshot(path=str(args.output/'native-fx-evidence-390.png'))
  assert result['pageErrors']==[] and result['writeRequests']==[]
  result['passed']=True
 finally:
  (args.output/'native-fx-results.json').write_text(json.dumps(result,indent=2)+'\n')
  (args.root/'done').touch()
  browser.close()
