import assert from 'node:assert/strict';
import { chromium } from '/opt/codex/runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright-core/index.mjs';

const outputDir = '/workspace/NestworthGo-pr98-pr99-evidence';
const browser = await chromium.launch({ headless: true, executablePath: '/usr/bin/chromium', args: ['--no-sandbox'] });
const context = await browser.newContext({ viewport: { width: 390, height: 844 }, reducedMotion: 'reduce' });
const page = await context.newPage();
const errors = [];
const results = { viewport: { width: 390, height: 844 }, synthetic_only: true, app_navigation_stubbed: false };
page.on('pageerror', error => errors.push(`pageerror: ${error.message}`));
page.on('console', message => { if (message.type() === 'error') errors.push(`console: ${message.text()}`); });
await context.tracing.start({ screenshots: true, snapshots: true, sources: true });
try {
  await page.route('**/wails/custom.js', route => route.fulfill({ status: 200, contentType: 'application/javascript', body: '' }));
  await page.goto('http://127.0.0.1:4178/preview/pr98-pr99-browser.html', { waitUntil: 'networkidle' });
  await page.getByTestId('overview-net-worth').waitFor({ state: 'visible' });

  const reviewEntry = page.getByRole('button', { name: 'Open investment review' });
  await reviewEntry.click();
  await page.getByRole('tab', { name: 'Investment review' }).click();
  await page.getByRole('heading', { name: 'Synthetic First Holding' }).waitFor({ state: 'visible' });
  const search = page.getByRole('textbox', { name: 'Search investments' });
  await search.fill('Synthetic');
  await page.getByRole('combobox', { name: 'Type' }).selectOption('mutual_fund');
  await page.getByRole('button', { name: 'To review' }).click();
  await page.waitForFunction(() => window.__syntheticCalls?.some(call => call.methodID === 3406455045 && call.args?.[0]?.filter?.status === 'pending' && call.args?.[0]?.filter?.instrumentType === 'mutual_fund' && call.args?.[0]?.filter?.query === 'Synthetic'));
  const next = page.getByRole('button', { name: 'Next' });
  await next.click();
  await page.getByRole('heading', { name: 'Synthetic Second Holding' }).waitFor({ state: 'visible' });
  await page.waitForFunction(() => window.__syntheticCalls?.some(call => call.methodID === 3406455045 && call.args?.[0]?.cursor === 'synthetic-page-two' && call.args?.[0]?.filter?.status === 'pending' && call.args?.[0]?.filter?.instrumentType === 'mutual_fund' && call.args?.[0]?.filter?.query === 'Synthetic'));
  results.page_two_request = await page.evaluate(() => {
    const request = [...(window.__syntheticCalls ?? [])].reverse().find(call => call.methodID === 3406455045 && call.args?.[0]?.cursor === 'synthetic-page-two');
    return request?.args?.[0];
  });
  results.filters_visible_on_page_two = {
    search: await search.inputValue(),
    type: await page.getByRole('combobox', { name: 'Type' }).inputValue(),
    pendingPressed: await page.getByRole('button', { name: 'To review' }).getAttribute('aria-pressed'),
    secondPageHeading: await page.getByRole('heading', { name: 'Synthetic Second Holding' }).innerText(),
  };
  await page.screenshot({ path: `${outputDir}/07-review-phone-390.png`, fullPage: true });

  await page.getByRole('button', { name: 'More actions for Synthetic Second Holding' }).click();
  await page.getByRole('menuitem', { name: 'View holdings' }).click();
  await page.getByRole('tab', { name: 'Holdings' }).waitFor({ state: 'visible' });
  await page.getByRole('heading', { name: 'SYN2', level: 2 }).waitFor({ state: 'visible' });
  await page.getByTestId('instrument-detail-page').getByText('$36.00').waitFor({ state: 'visible' });
  results.detail = { instrument: await page.getByRole('heading', { name: 'SYN2', level: 2 }).innerText(), holdingsSelected: await page.getByRole('tab', { name: 'Holdings' }).getAttribute('aria-selected'), syntheticHoldingVisible: true };

  const beforeHiddenInvalidate = await page.evaluate(() => (window.__syntheticCalls ?? []).filter(call => call.methodID === 3406455045).length);
  await page.evaluate(() => window.__invalidateReview?.());
  await page.waitForTimeout(250);
  const afterHiddenInvalidate = await page.evaluate(() => (window.__syntheticCalls ?? []).filter(call => call.methodID === 3406455045).length);
  assert.equal(afterHiddenInvalidate, beforeHiddenInvalidate, 'review must not refetch while detail route is active');
  results.hidden_route_review_calls = { before_invalidate: beforeHiddenInvalidate, after_invalidate: afterHiddenInvalidate };

  await page.getByRole('button', { name: /Back to/ }).click();
  await page.getByRole('heading', { name: 'Synthetic Second Holding' }).waitFor({ state: 'visible' });
  await assertVisibleFocus(page, 'upcoming-actions-heading');
  results.first_back_focus = await focusState(page);
  assert.equal(await search.inputValue(), 'Synthetic');
  assert.equal(await page.getByRole('combobox', { name: 'Type' }).inputValue(), 'mutual_fund');
  assert.equal(await page.getByRole('button', { name: 'To review' }).getAttribute('aria-pressed'), 'true');
  assert.equal(await page.getByRole('heading', { name: 'Synthetic Second Holding' }).isVisible(), true);

  await page.getByRole('button', { name: /Back to/ }).click();
  await page.getByTestId('overview-net-worth').waitFor({ state: 'visible' });
  await assertVisibleFocus(page, '');
  assert.equal(await page.getByRole('button', { name: 'Open investment review' }).evaluate(el => document.activeElement === el), true, 'second Back must focus the visible review entry trigger');
  results.second_back_focus = await focusState(page);
  results.page_errors = errors;
  assert.deepEqual(errors, [], 'browser reported page or console errors');
  console.log(JSON.stringify(results, null, 2));
} finally {
  await context.tracing.stop({ path: `${outputDir}/pr98-pr99-second-page.trace.zip` });
  await browser.close();
}

async function assertVisibleFocus(targetPage, expectedId) {
  try {
    if (expectedId) {
      await targetPage.waitForFunction(id => document.activeElement?.id === id, expectedId);
    } else {
      await targetPage.waitForFunction(() => document.activeElement?.matches('button') && document.activeElement.textContent?.trim() === 'Open investment review');
    }
  } catch (error) {
    console.log(`focus assertion failed for ${expectedId || 'Open investment review'}: ${JSON.stringify(await focusState(targetPage))}`);
    throw error;
  }
  const state = await focusState(targetPage);
  assert.equal(state.hiddenAncestor, false, 'focused element must have no hidden/inert/aria-hidden ancestor');
  assert.equal(state.visible, true, 'focused element must be visible');
}
async function focusState(targetPage) {
  return targetPage.evaluate(() => {
    const element = document.activeElement;
    return {
      tag: element?.tagName ?? null,
      id: element?.id ?? '',
      text: element?.textContent?.trim() ?? '',
      ariaLabel: element?.getAttribute('aria-label') ?? '',
      hiddenAncestor: Boolean(element?.closest('[hidden], [inert], [aria-hidden="true"]')),
      visible: Boolean(element && element.getClientRects().length && getComputedStyle(element).visibility !== 'hidden'),
    };
  });
}
