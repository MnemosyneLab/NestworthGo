import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import i18n from "@/i18n";
import { AssistantPageProvider, usePublishAssistantPage } from "./PageContext";
import { AssistantConversation } from "./AssistantConversation";
import type { PagePublication } from "./contracts";
import { useLayoutEffect } from "react";
import { useAssistantPage } from "./PageContext";

const status = { version: 1, surface: "desktop", pageGuide: true, provider: "unavailable", credentials: "unavailable", conversations: "memory", modelRuns: false, financialWrites: false, reason: "activation_incomplete" };
function Publisher({ publication, refresh }: { publication: PagePublication; refresh?: () => void }) {
  const enable = useAssistantPage()?.setEnabled;
  useLayoutEffect(() => enable?.(true), [enable]);
  usePublishAssistantPage(publication, refresh); return null;
}
function fixture() : PagePublication { return { viewKey: "overview:", page: "overview", scope: { kind: "household", label: "Synthetic household" }, timezone: "Asia/Shanghai", basis: "base", currency: "CNY", observedAt: Date.now(), state: "available", targets: [], metrics: [
  { label: "Synthetic missing value", value: null, unit: "CNY", basis: "net_worth", status: "unavailable", missingReasons: ["quote"] },
  { label: "Synthetic subtotal", value: "0.004", unit: "CNY", basis: "assets", status: "incomplete", missingReasons: ["fx"] },
] }; }
function App({ publication, disconnected = false, viewKey = "overview:", blocked = false, navigate = vi.fn(), refresh, surface = "desktop" }: { publication: PagePublication; disconnected?: boolean; viewKey?: string; blocked?: boolean; navigate?: () => void; refresh?: () => void; surface?: "desktop" | "mobile" }) {
  return <AssistantPageProvider householdId="synthetic" incarnation="incarnation" surface={surface} viewKey={viewKey} blocked={() => blocked} onNavigate={navigate}>
    <Publisher publication={publication} refresh={refresh} /><AssistantConversation surface={surface} status={{ ...status, surface }} loading={false} disconnected={disconnected} retry={vi.fn()} />
  </AssistantPageProvider>;
}
afterEach(() => vi.useRealTimers());

it.each(["en", "zh-CN", "zh-TW"].flatMap(language => (["desktop", "mobile"] as const).map(surface => ({ language, surface }))))("supports keyboard selection, evidence and closed review in $language on $surface", async ({ language, surface }) => {
  await i18n.changeLanguage(language); const user = userEvent.setup(); render(<App publication={fixture()} surface={surface} />);
  expect(screen.getByLabelText(i18n.t("assistant.composer"))).toBeDisabled();
  await user.click(screen.getByRole("button", { name: i18n.t("assistant.explain") }));
  expect(screen.getByText(i18n.t("assistant.needsSelection"))).toBeVisible();
  const select = screen.getByLabelText(i18n.t("assistant.metric")); select.focus();
  await user.selectOptions(select, screen.getByRole("option", { name: "Synthetic missing value" }));
  await user.tab(); await user.keyboard("{Enter}");
  await waitFor(() => expect(screen.getByText(i18n.t("assistant.localGuide"))).toBeVisible());
  const conversation = screen.getByLabelText(i18n.t("assistant.conversation"));
  expect(within(conversation).getByText(i18n.t("assistant.metricState.unavailable"))).toBeVisible();
  expect(within(conversation).queryByText("0.00")).not.toBeInTheDocument();
  const evidence = within(conversation).getByText(i18n.t("assistant.evidence"));
  expect(evidence).toHaveClass("min-h-11");
  if (surface === "desktop") expect(evidence).toHaveClass("xl:min-h-0");
  else expect(evidence).not.toHaveClass("xl:min-h-0");
  await user.click(evidence);
  expect(screen.getAllByText("Asia/Shanghai")[0]).toBeVisible();
  await user.click(screen.getByRole("button", { name: i18n.t("assistant.review") }));
  expect(await screen.findByText(i18n.t("assistant.reviewUnavailable"))).toBeVisible();
  const close = screen.getByRole("button", { name: i18n.t("common.close") });
  expect(close).toHaveClass("min-h-11");
  if (surface === "mobile") expect(close).not.toHaveClass("xl:min-h-8");
  expect(screen.getByRole("button", { name: i18n.t("assistant.confirmUnavailable") })).toBeDisabled();
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.getByRole("button", { name: i18n.t("assistant.review") })).toHaveFocus());
});

it.each(["en", "zh-CN", "zh-TW"].flatMap(language => (["desktop", "mobile"] as const).map(surface => ({ language, surface }))))("discloses credential storage while gates stay closed in $language on $surface", async ({ language, surface }) => {
  await i18n.changeLanguage(language);
  const user = userEvent.setup();
  render(<App publication={fixture()} surface={surface} />);
  const summary = screen.getByText(i18n.t("assistant.settings"));
  expect(screen.getByText(i18n.t("aiCredentials.backup"))).not.toBeVisible();
  summary.focus();
  expect(summary).toHaveFocus();
  await user.click(summary);
  for (const key of ["assistant.credentialSettings", "aiCredentials.help", "aiCredentials.backup", "assistant.memoryOnly"]) {
    expect(screen.getByText(i18n.t(key))).toBeVisible();
  }
  expect(screen.getByLabelText(i18n.t("assistant.composer"))).toBeDisabled();
  expect(screen.queryByLabelText(i18n.t("aiCredentials.key"))).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: i18n.t("assistant.review") }));
  expect(await screen.findByText(i18n.t("assistant.reviewUnavailable"))).toBeVisible();
  expect(screen.getByRole("button", { name: i18n.t("assistant.confirmUnavailable") })).toBeDisabled();
});

it("stops local work, discards late page results and pauses on disconnect without replay", async () => {
  vi.useFakeTimers(); const publication = fixture(); const view = render(<App publication={publication} />);
  fireEvent.change(screen.getByLabelText("Visible metric"), { target: { value: (screen.getAllByRole("option")[1] as HTMLOptionElement).value } });
  fireEvent.click(screen.getByRole("button", { name: "Explain selected metric" }));
  expect(screen.getByText("Preparing page explanation…")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "Stop explanation" }));
  await act(async () => vi.advanceTimersByTime(0));
  expect(screen.queryByText("Local page guide · no model")).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Explain selected metric" }));
  view.rerender(<App publication={publication} viewKey="settings:" />);
  await act(async () => vi.advanceTimersByTime(0));
  expect(screen.getByText("Page changed. The previous explanation was discarded.")).toBeVisible();
  view.rerender(<App publication={publication} disconnected />);
  expect(screen.getByRole("button", { name: "Explain selected metric" })).toBeDisabled();
  view.rerender(<App publication={publication} />);
  expect(screen.queryByText("Local page guide · no model")).not.toBeInTheDocument();
});

it("expires a snapshot while idle and blocks repeated review confirmation", async () => {
  vi.useFakeTimers(); render(<App publication={fixture()} />);
  await act(async () => vi.advanceTimersByTime(120_000));
  expect(screen.getByRole("button", { name: "Explain selected metric" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "Financial review" }));
  const confirm = screen.getByRole("button", { name: "AI confirmation unavailable" });
  fireEvent.click(confirm); fireEvent.click(confirm);
  expect(confirm).toBeDisabled();
  expect(screen.queryByText(/succeeded|posted successfully/i)).not.toBeInTheDocument();
});

it("refreshes only by an explicit controlled page read and requires a new snapshot", async () => {
  vi.useFakeTimers(); const refresh = vi.fn(); const publication = fixture(); const view = render(<App publication={publication} refresh={refresh} />);
  await act(async () => vi.advanceTimersByTime(120_000));
  expect(refresh).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Refresh page evidence" }));
  expect(refresh).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("button", { name: "Explain selected metric" })).toBeDisabled();
  view.rerender(<App publication={{ ...publication, observedAt: Date.now() }} refresh={refresh} />);
  expect(screen.getByRole("button", { name: "Explain selected metric" })).toBeEnabled();
});
