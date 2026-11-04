"""Exercise the PR review regressions against isolated synthetic HTTP servers."""
import argparse
import json
import re
from pathlib import Path
from playwright.sync_api import sync_playwright, expect

parser = argparse.ArgumentParser()
parser.add_argument("--qa-root", type=Path, required=True)
parser.add_argument("--base-url", default="http://127.0.0.1:18741")
parser.add_argument("--onboarding-url", required=True)
parser.add_argument("--chromium", default="/usr/bin/chromium")
parser.add_argument("--allow-no-sandbox", action="store_true")
args = parser.parse_args()
root = args.qa_root.resolve()
seed = json.loads((root / "analytics/seed/seed-results.json").read_text())
assert seed["fixture_version"] == "analytics-linux-qa-v3" and seed["scenario"] == "complete-cny"
report = {"fixture": "analytics-linux-qa-v3 / complete-cny", "sandbox": "disabled" if args.allow_no_sandbox else "enabled", "cases": [], "pageErrors": []}

with sync_playwright() as p:
    browser = p.chromium.launch(executable_path=args.chromium, headless=True, chromium_sandbox=not args.allow_no_sandbox)
    page = browser.new_page(viewport={"width": 1280, "height": 860})
    page.on("pageerror", lambda error: report["pageErrors"].append(str(error)))
    page.goto(args.base_url)
    nav = page.get_by_role("navigation")
    controls = page.get_by_role("group", name="Navigation history")
    nav.get_by_role("button", name="Accounts", exact=True).click()
    page.get_by_role("button", name=re.compile("AUD Cash.*Bank account")).click()
    for draft in ["Synthetic browser draft A", "Synthetic browser draft B"]:
        page.get_by_role("button", name="Open account reconciliation", exact=True).click()
        resume = page.get_by_role("button", name="Continue session", exact=True)
        start = page.get_by_role("button", name=re.compile("^Start (?:reconciliation|session)$"), exact=True)
        page.get_by_role("group", name="Select accounts to check", exact=True).wait_for()
        if resume.count():
            resume.click()
        else:
            start.click()
            page.get_by_role("dialog").get_by_role("button", name=re.compile("^Start (?:reconciliation|session)$"), exact=True).click()
        page.get_by_role("button", name="Finish session", exact=True).click()
        note = page.get_by_label("Closing explanation", exact=True)
        expect(note).to_have_value("")
        note.fill(draft)
        page.keyboard.press("Escape")
        expect(page.get_by_role("button", name="Finish session", exact=True)).to_be_focused()
        controls.get_by_role("button", name="Go back", exact=True).click()
    nav.get_by_role("button", name="Overview", exact=True).click()
    for workspace, draft in [(1, "Synthetic browser draft A"), (2, "Synthetic browser draft B")]:
        controls.get_by_role("button", name="Recent pages", exact=True).click()
        name = f"Tasks & checks · AUD Cash · Reconciliation · Workspace {workspace}"
        page.get_by_role("menuitem", name=name, exact=True).click()
        page.get_by_role("button", name="Finish session", exact=True).click()
        expect(page.get_by_label("Closing explanation", exact=True)).to_have_value(draft)
        page.keyboard.press("Escape")
        expect(page.get_by_role("button", name="Finish session", exact=True)).to_be_focused()
        nav.get_by_role("button", name="Overview", exact=True).click()
    controls.get_by_role("button", name="Recent pages", exact=True).click()
    page.screenshot(path=str(root / "artifacts/retained-sessions.png"))
    page.keyboard.press("Escape")
    report["cases"].append("Both same-account reconciliation drafts remain reachable after forward truncation and MRU replacement")

    page.reload()
    calls = []
    page.on("request", lambda request: calls.append(request.url + " " + (request.post_data or "")) if "/wails/runtime" in request.url else None)
    nav.get_by_role("button", name="Wealth history", exact=True).click()
    date_button = page.get_by_role("button", name="Historical date", exact=True)
    date_a = date_button.inner_text()
    page.locator("p:visible").filter(has_text=re.compile("^Captured at:")).wait_for()
    captured_a = page.locator("p:visible").filter(has_text=re.compile("^Captured at:")).inner_text()
    original_calls = sum("1841438157" in call for call in calls)
    assert original_calls == 1
    nav.get_by_role("button", name="Accounts", exact=True).click()
    controls.get_by_role("button", name="Recent pages", exact=True).click()
    page.get_by_role("menuitem", name=f"Wealth history · {date_a}", exact=True).click()
    assert sum("1841438157" in call for call in calls) == original_calls
    page.get_by_role("button", name="Previous day", exact=True).click()
    expect(date_button).not_to_have_text(date_a)
    date_b = date_button.inner_text()
    page.locator("p:visible").filter(has_text=re.compile("^Captured at:")).wait_for()
    nav.get_by_role("button", name="Accounts", exact=True).click()
    controls.get_by_role("button", name="Recent pages", exact=True).click()
    page.get_by_role("menuitem", name=f"Wealth history · {date_a}", exact=True).click()
    expect(date_button).to_have_text(date_a)
    expect(page.locator("p:visible").filter(has_text=re.compile("^Captured at:"))).to_have_text(captured_a)
    assert sum("1841438157" in call for call in calls) == 2
    controls.get_by_role("button", name="Go back", exact=True).click()
    controls.get_by_role("button", name="Go back", exact=True).click()
    expect(date_button).to_have_text(date_b)
    controls.get_by_role("button", name="Go back", exact=True).click()
    controls.get_by_role("button", name="Go back", exact=True).click()
    expect(date_button).to_have_text(date_a)
    expect(page.locator("p:visible").filter(has_text=re.compile("^Captured at:"))).to_have_text(captured_a)
    page.screenshot(path=str(root / "artifacts/historical-fork.png"))
    report["cases"].append("Recent and Back restore their selected historical date and original frozen capture without rereading")

    onboarding = browser.new_page()
    onboarding.on("pageerror", lambda error: report["pageErrors"].append(str(error)))
    onboarding.goto(args.onboarding_url)
    form = onboarding.get_by_role("form", name="Onboarding", exact=True)
    form.get_by_label("Household name", exact=True).fill("Synthetic navigation onboarding")
    form.get_by_label("Member 1 name", exact=True).fill("Synthetic member")
    form.get_by_label("Base currency", exact=True).select_option("USD")
    form.get_by_role("button", name="Get started", exact=True).click()
    expect(onboarding.get_by_role("navigation").get_by_role("button", name="Accounts", exact=True)).to_have_attribute("aria-current", "page")
    expect(onboarding.get_by_role("button", name="Go back", exact=True)).to_be_disabled()
    expect(onboarding.get_by_role("button", name="Recent pages", exact=True)).to_be_disabled()
    report["cases"].append("First onboarding lands on Accounts with empty history after real household creation")
    assert not report["pageErrors"], report["pageErrors"]
    browser.close()

(root / "artifacts/revision-results.json").write_text(json.dumps(report, indent=2) + "\n")
print(json.dumps(report, indent=2))
