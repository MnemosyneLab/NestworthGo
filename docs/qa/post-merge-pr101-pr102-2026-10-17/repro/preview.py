#!/usr/bin/env python3
"""Drive the PR101/PR102 preview failure through the real browser UI."""

import argparse
import json
from pathlib import Path

from playwright.sync_api import sync_playwright


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", default="http://127.0.0.1:18734/")
    parser.add_argument("--output", required=True, help="new or existing evidence directory")
    parser.add_argument("--chromium", default="/usr/bin/chromium")
    parser.add_argument(
        "--chromium-sandbox",
        action="store_true",
        help="request Playwright Chromium sandboxing (fails if the container cannot provide it)",
    )
    args = parser.parse_args()
    output = Path(args.output)
    output.mkdir(parents=True, exist_ok=True)

    with sync_playwright() as playwright:
        browser = playwright.chromium.launch(
            headless=True,
            executable_path=args.chromium,
            chromium_sandbox=args.chromium_sandbox,
        )
        page = browser.new_page(viewport={"width": 1440, "height": 1050})
        page.set_default_timeout(10_000)
        bad_responses = []
        console_errors = []
        page_errors = []
        runtime_requests = []
        page.on(
            "response",
            lambda response: bad_responses.append(
                {"status": response.status, "url": response.url, "body": response.text()[:2000]}
            )
            if response.status >= 400
            else None,
        )
        page.on(
            "request",
            lambda request: runtime_requests.append(request.post_data)
            if request.method == "POST" and request.url.endswith("/wails/runtime")
            else None,
        )
        page.on(
            "console",
            lambda message: console_errors.append(message.text) if message.type == "error" else None,
        )
        page.on("pageerror", lambda error: page_errors.append(str(error)))

        page.goto(args.url, wait_until="domcontentloaded")
        page.get_by_role("button", name="Upcoming actions", exact=True).click()
        page.get_by_role("tab", name="Investment review").click()
        page.get_by_role("button", name="Rules", exact=True).click()
        page.get_by_role("button", name="New rule", exact=True).click()

        dialog = page.get_by_role("dialog")
        dialog.get_by_label("Name").fill("Synthetic replay reproduction")
        dialog.locator('select[aria-label="Metric"]').nth(0).select_option("amount")
        dialog.locator('button[aria-label="Add condition to group 1"]').click()
        dialog.locator('select[aria-label="Metric"]').nth(1).select_option("cost")
        dialog.locator('input[aria-label="Value"]').nth(0).fill("900")
        dialog.locator('input[aria-label="Value"]').nth(1).fill("1002")
        dialog.locator('select[aria-label="Currency"]').nth(1).select_option("USD")
        dialog.locator('select[aria-label="Currency handling"]').nth(1).select_option("native")
        dialog.get_by_label("Preview investment").select_option(
            label="Synthetic opened daily NAV fund"
        )

        with page.expect_response(
            lambda response: response.request.method == "POST"
            and response.url.endswith("/wails/runtime")
            and response.status == 422,
            timeout=20_000,
        ) as failed_preview:
            dialog.get_by_role("button", name="Preview", exact=True).click()
        response = failed_preview.value
        result = {
            "url": args.url,
            "chromiumVersion": browser.version,
            "chromiumSandboxRequested": args.chromium_sandbox,
            "previewStatus": response.status,
            "previewResponse": response.text(),
            "previewRequest": next(
                (body for body in reversed(runtime_requests) if body and "2292729084" in body),
                None,
            ),
            "dialogText": dialog.inner_text(),
            "failedResponses": bad_responses,
            "consoleErrors": console_errors,
            "pageErrors": page_errors,
        }
        (output / "preview.json").write_text(json.dumps(result, indent=2) + "\n")
        page.screenshot(path=str(output / "preview-failure.png"), full_page=True)
        browser.close()

    if result["previewStatus"] != 422 or "historical replay reached a negative balance" not in result["previewResponse"]:
        raise SystemExit("The expected preview replay failure was not observed; see preview.json")
    print(json.dumps(result, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
