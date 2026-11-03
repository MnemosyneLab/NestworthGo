// Synthetic browser check: run after generating frontend bindings.
// Requires Playwright (PLAYWRIGHT_MODULE may point to its installed index.mjs)
// and Chromium (CHROMIUM_PATH may point to the executable). No backend is run.
import assert from "node:assert/strict";
import { mkdtemp, writeFile, rm } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repo = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../..");
const frontend = path.join(repo, "frontend");
const { createServer } = await import(pathToFileURL(path.join(frontend, "node_modules/vite/dist/node/index.js")));
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || "playwright");
const harness = await mkdtemp(path.join(frontend, ".pr195-browser-"));
let server;
let browser;
try {
  await writeFile(path.join(harness, "index.html"), '<div id="root"></div><script type="module" src="./main.tsx"></script>');
  await writeFile(path.join(harness, "main.tsx"), `
import React from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ContributionTab } from "@/features/insights/ContributionTab";
import { useAnalysisStore } from "@/stores/analysis";
import "@/i18n";
import "@/index.css";
useAnalysisStore.getState().setFilters({ from: "2026-08-01", to: "2026-08-31" });
createRoot(document.getElementById("root")!).render(<QueryClientProvider client={new QueryClient({defaultOptions:{queries:{retry:false}}})}><main style={{padding:24}}><ContributionTab session={useAnalysisStore.getState()} /></main></QueryClientProvider>);
`);
  server = await createServer({ configFile: path.join(frontend, "vite.config.ts"), root: frontend, server: { port: 9435, host: "127.0.0.1", strictPort: true } });
  await server.listen();
  browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH, headless: true, args: ["--no-sandbox"] });
  const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));
  // Intercept every generated service module: no financial data or Wails host.
  await page.route(/\/bindings\/.*\/service\.(?:js|ts)(?:\?.*)?$/, async route => {
    const source = `
const available = { available: true, status: "ok", ratedDays: 1, totalDays: 1 };
export async function HistoryOrigin() { return {id:"synthetic-origin",timezone:"UTC",startedAt:"2026-01-01T00:00:00Z"}; }
export async function ListAccounts() { return []; }
export async function ListInstruments() { return []; }
export async function Contribution(_request, _metric, _group, ordering) {
  window.sortRequests = [...(window.sortRequests || []), ordering];
  if (ordering !== "amount_desc") await new Promise(resolve => setTimeout(resolve, 500));
  const rows = Array.from({length:100}, (_,index) => ({...available,key:String(index).padStart(3,"0"),label:"Synthetic " + String(index).padStart(3,"0"),amount:index === 99 ? null : {amount:String(index - 40),currency:"USD"},rate:null}));
  if (ordering === "amount_desc") rows.sort((a,b) => a.amount ? b.amount ? Number(b.amount.amount)-Number(a.amount.amount) : -1 : 1);
  if (ordering === "amount_asc") rows.sort((a,b) => a.amount ? b.amount ? Number(a.amount.amount)-Number(b.amount.amount) : 1 : -1);
  return {...available,rows,summary:{total:{amount:"891",currency:"USD"},positive:{amount:"1711",currency:"USD"},negative:{amount:"-820",currency:"USD"},amountStatus:"partial"}};
}
export async function ContributionItem() { throw new Error("unused synthetic detail"); }
`;
    await route.fulfill({ contentType: "text/javascript", body: source });
  });
  await page.goto(`http://127.0.0.1:9435/${path.basename(harness)}/index.html`);
  await page.getByTestId("contribution-ranking").waitFor();
  const sort = page.getByRole("combobox", { name: "Sort", exact: true });
  await sort.scrollIntoViewIfNeeded();
  await sort.focus();
  const before = await page.evaluate(() => {
    window.originalSort = document.getElementById("contribution-sort");
    window.originalList = document.querySelector('ul[aria-label="Contribution"]');
    return { scrollY, height: document.documentElement.scrollHeight };
  });
  assert(before.scrollY > 0, "check must start below the top of the page");
  await sort.selectOption("amount_asc");
  await page.waitForFunction(() => window.sortRequests.includes("amount_asc"));
  const measure = () => page.evaluate(() => ({
    scrollY, height: document.documentElement.scrollHeight,
    sameSort: document.getElementById("contribution-sort") === window.originalSort,
    sameList: document.querySelector('ul[aria-label="Contribution"]') === window.originalList,
    focused: document.activeElement === window.originalSort,
  }));
  const pending = await measure();
  assert(pending.sameSort && pending.sameList && pending.focused, "sorting must retain nodes and focus while pending");
  assert.equal(pending.scrollY, before.scrollY, "pending sort must preserve scroll");
  assert.equal(pending.height, before.height, "pending sort must preserve page height");
  await page.waitForFunction(() => document.querySelector('[data-testid="contribution"]')?.getAttribute("aria-busy") === "false");
  const after = await measure();
  assert(after.sameSort && after.sameList && after.focused, "completed sort must retain nodes and focus");
  assert.equal(after.scrollY, before.scrollY, "completed sort must preserve scroll");
  assert.match(await page.locator('ul[aria-label="Contribution"] > li').first().innerText(), /Synthetic 099/, "backend unavailable-first ascending order must be retained");
  assert.deepEqual(errors, [], "browser should have no uncaught errors");
  console.log(JSON.stringify({ browser: await browser.version(), syntheticRows: 100, before, pending, after, result: "PASS" }, null, 2));
} finally {
  await browser?.close();
  await server?.close();
  await rm(harness, { recursive: true, force: true });
}
