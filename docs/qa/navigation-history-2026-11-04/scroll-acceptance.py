"""Verify source scroll and archive-filter restoration using the synthetic HTTP app."""
import argparse
import json
import re
from pathlib import Path
from playwright.sync_api import sync_playwright, expect

parser = argparse.ArgumentParser()
parser.add_argument("--qa-root", required=True, type=Path)
parser.add_argument("--base-url", default="http://127.0.0.1:18741")
parser.add_argument("--allow-no-sandbox", action="store_true")
args = parser.parse_args()
root = args.qa_root.resolve()
assert json.loads((root / "analytics/seed/seed-results.json").read_text())["fixture_version"] == "analytics-linux-qa-v3"
with sync_playwright() as p:
    browser = p.chromium.launch(executable_path="/usr/bin/chromium", headless=True, chromium_sandbox=not args.allow_no_sandbox)
    page = browser.new_page(viewport={"width": 1280, "height": 500})
    page.goto(args.base_url)
    nav = page.get_by_role("navigation")
    nav.get_by_role("button", name="Accounts", exact=True).click()
    page.get_by_role("checkbox", name="Show archived", exact=True).check()
    page.get_by_role("button", name=re.compile("US Brokerage.*Brokerage")).click()
    page.get_by_role("heading", name="US Brokerage", exact=True).wait_for()
    main = page.locator("#main-content")
    actual = main.evaluate("element => { element.scrollTop = 170; return element.scrollTop; }")
    assert actual == 170, "fixture must have real overflow; scroll cannot be fabricated"
    page.get_by_role("banner").get_by_role("button", name="Return analysis", exact=True).click()
    page.get_by_role("button", name="Go back", exact=True).click()
    expect(main).to_have_js_property("scrollTop", 170)
    page.screenshot(path=str(root / "artifacts/scroll-restoration.png"))
    page.get_by_role("button", name="Back to accounts", exact=True).click()
    expect(page.get_by_role("checkbox", name="Show archived", exact=True)).to_be_checked()
    browser.close()
report = {"case": "Real account analysis restores source scroll 170 and archive filter", "passed": True, "sandbox": "disabled" if args.allow_no_sandbox else "enabled"}
(root / "artifacts/scroll-results.json").write_text(json.dumps(report, indent=2) + "\n")
print(json.dumps(report, indent=2))
