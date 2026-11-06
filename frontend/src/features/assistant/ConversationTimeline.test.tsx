import i18n from "@/i18n";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { createTestQueryClient } from "@/test/queryClient";
import { ConversationTimeline } from "./ConversationTimeline";

const api = vi.hoisted(() => ({ Inspect: vi.fn(), Messages: vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service", () => api);
afterEach(() => vi.unstubAllGlobals());
const run = { id: "run", requestId: "request", prompt: "Synthetic question", state: "running", text: "", created: "2026-11-05T00:00:00Z", model: "mock" };
const initial = { id: "conversation", updated: "2026-11-05T00:00:00Z", offset: 0, runCount: 1, runs: [run], progress: { runId: "run", revision: 1, text: "**Synthetic preview**", tools: [{ source: "get_financial_context", state: "running", input: '{"disclosure":"minimal"}', output: "", inputTruncated: false, outputTruncated: false }] } };
it("renders per-step details and provisional Markdown, then removes cancelled preview", () => {
  const client = createTestQueryClient();
  const props = { epoch: "epoch", id: "conversation", busy: false, onCancel: vi.fn() };
  const view = (latest: unknown) => <QueryClientProvider client={client}><ConversationTimeline {...props} latest={latest as Parameters<typeof ConversationTimeline>[0]["latest"]} /></QueryClientProvider>;
  const { container, rerender } = render(view(initial));
  expect(screen.getByText(/Generating.*incomplete preview/)).toBeVisible();
  expect(container.querySelector("strong")).toHaveTextContent("Synthetic preview");
  const tool = screen.getByText(/Financial summary · Running/).closest("details")!;
  fireEvent.click(tool.querySelector("summary")!);
  expect(screen.getByText('{"disclosure":"minimal"}')).toBeVisible();
  rerender(view({ ...initial, progress: { ...initial.progress, revision: 2, tools: [{ ...initial.progress.tools[0], state: "succeeded", output: '{"account":"Synthetic","amount":"12.3456"}' }] } }));
  expect(screen.getByText(/Financial summary · Completed/)).toBeVisible();
  expect(screen.getByText('{"account":"Synthetic","amount":"12.3456"}')).toBeVisible();
  rerender(view({ ...initial, progress: { ...initial.progress, revision: 3, tools: [{ ...initial.progress.tools[0], state: "succeeded", input: "", inputTruncated: true, output: "", outputTruncated: true }] } }));
  expect(screen.getAllByText(/Display budget exhausted/)).toHaveLength(2);
  expect(screen.queryByText(/Showing a truncated prefix/)).toBeNull();
  expect(screen.queryByText("No output received.")).toBeNull();
  rerender(view({ ...initial, runs: [{ ...run, state: "cancelled", code: "cancelled" }], progress: { ...initial.progress, revision: 3, text: "" } }));
  expect(screen.queryByText("Synthetic preview")).toBeNull();
  expect(screen.getByText("Stopped")).toBeVisible();
});
it("renders internal page-read status and actual details from the new page bridge", () => {
  const client = createTestQueryClient();
  const pageTool = { ...initial.progress.tools[0], source: "read_page_content", state: "succeeded", input: '{"module":"filters","limit":1}', output: '{"page":"return-analysis","nodes":[{"value":"Synthetic USD"}],"hasMore":true}' };
  const latest = { ...initial, progress: undefined, runs: [{ ...run, state: "succeeded", text: "**Synthetic page reply**", tools: [pageTool], evidence: [{source:"read_page_content",capturedAt:"2026-11-05T00:00:00Z",hasMore:true,missing:"reported"}] }] };
  render(<QueryClientProvider client={client}><ConversationTimeline epoch="epoch" id="conversation" latest={latest as Parameters<typeof ConversationTimeline>[0]["latest"]} busy={false} onCancel={vi.fn()} /></QueryClientProvider>);
  const summary = screen.getByText("Page content · Completed");
  fireEvent.click(summary.closest("summary")!);
  expect(screen.getByText(pageTool.input)).toBeVisible();
  expect(screen.getByText(pageTool.output)).toBeVisible();
  expect(screen.queryByText("Ledger read")).toBeNull();
});

it("follows resizing and chunks only while near the bottom", () => {
  let resize = () => {};
  const disconnect = vi.fn();
  vi.stubGlobal("ResizeObserver", class { constructor(callback: () => void) { resize = callback; } observe() {} disconnect = disconnect; });
  const client = createTestQueryClient();
  const props = { epoch: "epoch", id: "conversation", busy: false, onCancel: vi.fn() };
  const view = (revision: number) => <QueryClientProvider client={client}><ConversationTimeline {...props} latest={{ ...initial, progress: { ...initial.progress, revision } } as Parameters<typeof ConversationTimeline>[0]["latest"]} /></QueryClientProvider>;
  const { rerender, unmount } = render(view(1));
  const viewport = screen.getByRole("region", { name: "Conversation messages" });
  let height = 1000;
  Object.defineProperties(viewport, { scrollHeight: { get: () => height }, clientHeight: { get: () => 300 } });
  act(() => resize());
  expect(viewport.scrollTop).toBe(1000);
  viewport.scrollTop = 200;
  fireEvent.scroll(viewport);
  height = 1400;
  act(() => resize());
  rerender(view(2));
  expect(viewport.scrollTop).toBe(200);
  fireEvent.click(screen.getByRole("button", { name: /Latest messages/ }));
  expect(viewport.scrollTop).toBe(1400);
  height = 1800;
  act(() => resize());
  expect(viewport.scrollTop).toBe(1800);
  unmount();
  expect(disconnect).toHaveBeenCalled();
});

it("anchors an existing message when earlier history and a streaming tail grow together", async () => {
  let release: ((value: unknown) => void) | undefined;
  const saved = (position: number) => ({ ...run, id: `saved-${position}`, state: "succeeded", text: `Saved answer ${position}` });
  api.Messages.mockImplementation(async (_epoch, _id, offset: number) => {
    if (offset === 0) return new Promise(resolve => { release = resolve; });
    return { id: "conversation", offset, runs: [saved(offset)], runCount: 7 };
  });
  const client = createTestQueryClient();
  const latest = { ...initial, offset: 6, runCount: 7 };
  const view = (revision: number) => <QueryClientProvider client={client}><ConversationTimeline epoch="epoch" id="conversation" latest={{ ...latest, progress: { ...initial.progress, revision, text: "Synthetic tail ".repeat(revision) } } as Parameters<typeof ConversationTimeline>[0]["latest"]} busy={false} onCancel={vi.fn()} /></QueryClientProvider>;
  const { rerender } = render(view(1));
  await screen.findByText("Saved answer 1");
  await waitFor(() => expect(screen.queryByText("Loading messages…")).toBeNull());
  const viewport = screen.getByRole("region", { name: "Conversation messages" });
  let prepend = 0;
  let tailGrowth = 0;
  Object.defineProperties(viewport, { scrollHeight: { get: () => 2000 + prepend + tailGrowth }, clientHeight: { get: () => 300 } });
  viewport.scrollTop = 100;
  fireEvent.scroll(viewport);
  const article = screen.getByText("Saved answer 1").closest("article")!;
  vi.spyOn(viewport, "getBoundingClientRect").mockImplementation(() => new DOMRect(0, 0, 400, 300));
  vi.spyOn(article, "getBoundingClientRect").mockImplementation(() => new DOMRect(0, 200 + prepend - viewport.scrollTop, 400, 200));
  fireEvent.click(screen.getByRole("button", { name: "Load earlier messages" }));
  await waitFor(() => expect(release).toBeTypeOf("function"));
  // A manual scroll during the pending history read updates the saved offset.
  viewport.scrollTop = 150;
  fireEvent.scroll(viewport);
  tailGrowth = 500;
  rerender(view(2));
  expect(viewport.scrollTop).toBe(150);
  prepend = 300;
  await act(async () => release?.({ id: "conversation", offset: 0, runs: [saved(0)], runCount: 7 }));
  await screen.findByText("Saved answer 0");
  await waitFor(() => expect(viewport.scrollTop).toBe(450));
  expect(article.getBoundingClientRect().top).toBe(50);
});

it.each(["succeeded","refused","failed"])("shows Full %s and its subsequent read with actual inert details",(state)=>{
 const client=createTestQueryClient();
 const full={source:"execute_ledger_change",state,input:'{"amount":"12.34567890"}',output:'{"status":"'+state+'"}',inputTruncated:false,outputTruncated:false};
 const read={...full,source:"get_account_snapshot",state:"succeeded",input:'{"id":"synthetic"}',output:'{"balance":"12.34567890"}'};
 const latest={...initial,progress:undefined,runs:[{...run,state:"succeeded",tools:[full,read]}]};
 render(<QueryClientProvider client={client}><ConversationTimeline epoch="epoch" id="conversation" latest={latest as Parameters<typeof ConversationTimeline>[0]["latest"]} busy={false} onCancel={vi.fn()}/></QueryClientProvider>);
 const label=state==="succeeded"?"Completed":state==="refused"?"Tool refused":"Failed";
 const summary=screen.getByText(`Full ledger change · ${label}`);fireEvent.click(summary.closest("summary")!);
 expect(screen.getByText(full.input)).toBeVisible();expect(screen.getByText(full.output)).toBeVisible();
 expect(screen.getByText(read.output)).toBeInTheDocument();expect(screen.queryByText("Ledger read")).toBeNull();
 expect(api.Inspect).not.toHaveBeenCalled();
});

it.each([
  ["en", "Activity history", "Running", "Completed", "Tool call"],
  ["zh-CN", "账务记录", "执行中", "已完成", "工具调用"],
  ["zh-TW", "帳務紀錄", "執行中", "已完成", "工具呼叫"],
])("localizes live and saved tool names in %s", async (language, label, running, completed, unknown) => {
  await i18n.changeLanguage(language);
  const client = createTestQueryClient();
  const tools = [{ ...initial.progress.tools[0], source: "list_activities" }, { ...initial.progress.tools[0], source: "future_unknown_tool" }];
  const view = (latest: unknown) => <QueryClientProvider client={client}><ConversationTimeline epoch="epoch" id="conversation" latest={latest as Parameters<typeof ConversationTimeline>[0]["latest"]} busy={false} onCancel={vi.fn()} /></QueryClientProvider>;
  const { rerender } = render(view({ ...initial, progress: { ...initial.progress, tools } }));
  expect(screen.getByText(`${label} · ${running}`)).toBeVisible();
  expect(screen.getByText(`${unknown} · ${running}`)).toBeVisible();
  rerender(view({ ...initial, progress: undefined, runs: [{ ...run, state: "succeeded", tools: tools.map(tool => ({ ...tool, state: "succeeded" })) }] }));
  expect(screen.getByText(`${label} · ${completed}`)).toBeVisible();
  expect(screen.getByText(`${unknown} · ${completed}`)).toBeVisible();
  expect(screen.queryByText(/list activities|future unknown tool/)).not.toBeInTheDocument();
});

it("shows legacy receipts without a saved tool trace and keeps inspection failures visible", () => {
  const client = createTestQueryClient();
  render(<QueryClientProvider client={client}><ConversationTimeline epoch="epoch" id="conversation" latest={{ ...initial, progress: undefined, runs: [] }} busy={false} onCancel={vi.fn()}
    operations={[{ id: "old-receipt", toolName: "create_member", state: "succeeded" }]} operationsError /></QueryClientProvider>);
  const recovered = screen.getByText("Create member · Recorded");
  fireEvent.click(recovered.closest("summary")!);
  expect(screen.getByText("Operation result · Recorded")).toBeVisible();
  expect(screen.getByRole("alert")).toHaveTextContent("Could not refresh operation results.");
  expect(screen.queryByText("old-receipt")).not.toBeInTheDocument();
});
