#!/usr/bin/env python3
"""Opt-in Chromium + real HTTP/SQLite restart acceptance; synthetic data only.

Build frontend first, then run with Go on PATH:
  python3 tools/mobile-confirm-reconnect.py --output /tmp/mobile-reconnect-evidence
Requires Python Playwright and a Chromium executable. Nothing binds beyond loopback.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time

from playwright.sync_api import sync_playwright, expect


def wait_file(path, process):
    deadline = time.monotonic() + 90
    while not path.exists():
        if process.poll() is not None:
            raise RuntimeError("synthetic fixture exited; inspect fixture.log")
        if time.monotonic() > deadline:
            raise TimeoutError(f"waiting for {path.name}")
        time.sleep(0.1)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--chromium", default="/usr/bin/chromium")
    parser.add_argument("--expect-lockout", action="store_true", help="Verify the unfixed baseline only; never sends a successful financial confirmation")
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    args.output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="nestworth-synthetic-reconnect-") as private:
        control = Path(private)
        with (args.output / "fixture.log").open("w") as log:
            env = {**os.environ, "NESTWORTH_MOBILE_BROWSER_FIXTURE": private}
            fixture = subprocess.Popen(["go", "test", "./internal/mobileapi", "-run", "^TestMobileBrowserFixture$", "-count=1", "-v"], cwd=root, env=env, stdout=log, stderr=subprocess.STDOUT)
            try:
                wait_file(control / "connection.json", fixture)
                connection = json.loads((control / "connection.json").read_text())
                with sync_playwright() as p:
                    browser = p.chromium.launch(executable_path=args.chromium, headless=True, args=["--no-sandbox"])
                    context = browser.new_context(viewport={"width": 390, "height": 844}, locale="en-US")
                    page = context.new_page()
                    writes = []
                    errors = []
                    page.on("pageerror", lambda error: errors.append(str(error)))
                    page.on("request", lambda request: writes.append(request.post_data) if request.url.endswith("/record-confirmations") else None)
                    page.goto(connection["url"] + "/mobile.html")
                    page.get_by_label("Mobile token", exact=True).fill(connection["token"])
                    page.get_by_role("checkbox").check()
                    page.get_by_role("button", name="Sign in", exact=True).click()
                    page.get_by_role("button", name="Quick record", exact=True).click()
                    account = page.get_by_role("combobox", name="Account", exact=True)
                    account.select_option(label="Synthetic bank")
                    page.get_by_role("textbox", name="Amount", exact=True).fill("1.2345")
                    page.get_by_role("button", name="Preview", exact=True).click()
                    expect(page.get_by_role("button", name="Confirm", exact=True)).to_be_enabled()
                    page.screenshot(path=str(args.output / "01-reviewed.png"))
                    (control / "restart").touch()
                    wait_file(control / "restarted", fixture)
                    with page.expect_response(lambda r: r.url.endswith("/record-confirmations")) as refused:
                        page.get_by_role("button", name="Confirm", exact=True).click()
                    assert refused.value.status == 403, refused.value.status
                    error = refused.value.json()["error"]
                    if args.expect_lockout:
                        assert error["code"] == "forbidden" and error["operationState"] == "unknown"
                        expect(page.get_by_role("button", name="Reconnect", exact=True)).to_be_disabled()
                        assert len(writes) == 1
                        page.screenshot(path=str(args.output / "02-baseline-lockout.png"))
                        (args.output / "result.json").write_text(json.dumps({"result": "BASELINE_REPRODUCED", "oldCSRF": "403 forbidden/permission/unknown", "reconnectDisabled": True, "confirmationAttempts": 1, "successfulConfirmations": 0}, indent=2) + "\n")
                        browser.close()
                    else:
                        assert error["code"] == "csrf_expired" and error["operationState"] == "unknown"
                        expect(page.get_by_role("button", name="Reconnect", exact=True)).to_be_enabled()
                        original = writes[0]
                        page.get_by_role("button", name="Reconnect", exact=True).click()
                        expect(page.get_by_role("button", name="Reconnect", exact=True)).to_be_enabled()
                        assert writes == [original], "Reconnect dispatched a confirmation"
                        expect(page.get_by_role("textbox", name="Amount", exact=True)).to_have_value("1.2345")
                        page.screenshot(path=str(args.output / "02-reconnected-unknown.png"))
                        with page.expect_response(lambda r: r.url.endswith("/record-confirmations")) as unavailable:
                            page.get_by_role("button", name="Retry same submission", exact=True).click()
                        assert unavailable.value.status == 409
                        assert unavailable.value.json()["error"]["field"] == "planId"
                        assert writes == [original, original]
                        page.get_by_role("button", name="Check original result", exact=True).click()
                        expect(page.get_by_role("button", name="Cancel", exact=True)).to_be_disabled()
                        page.get_by_role("button", name="Discard recovery information", exact=True).click()
                        expect(page.get_by_role("alertdialog")).to_be_visible()
                        page.get_by_role("button", name="I checked on desktop", exact=True).click()
                        expect(page.get_by_role("textbox", name="Amount", exact=True)).to_have_value("1.2345")
                        expect(page.get_by_role("textbox", name="Amount", exact=True)).to_be_enabled()
                        assert len(writes) == 2
                        page.get_by_role("button", name="Preview", exact=True).click()
                        page.get_by_role("button", name="Confirm", exact=True).click()
                        expect(page.get_by_text("Saved successfully", exact=True)).to_be_visible()
                        assert len(writes) == 3
                        assert json.loads(writes[2])["operationId"] != json.loads(original)["operationId"]
                        page.screenshot(path=str(args.output / "03-explicit-new-confirmation.png"))
                        assert not errors, errors
                        browser.close()
                        (args.output / "result.json").write_text(json.dumps({"result": "PASS", "evidence": "Chromium 390px, real loopback HTTP + SQLite, actual listener/registry restart", "oldCSRF": "403 csrf_expired/unknown", "freshCSRF": "409 stale_preview/planId/unknown", "automaticConfirms": 0, "sameSubmissionBodies": writes[0] == writes[1], "explicitNewConfirmation": 1, "pageErrors": errors}, indent=2) + "\n")
            finally:
                (control / "done").touch()
                try:
                    fixture.wait(timeout=20)
                except subprocess.TimeoutExpired:
                    fixture.terminate()
                    fixture.wait(timeout=10)
            if fixture.returncode != 0:
                raise RuntimeError("synthetic fixture failed; inspect fixture.log")


if __name__ == "__main__":
    main()
