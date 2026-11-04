import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { createTestQueryClient } from "@/test/queryClient";
import { AssistantPageProvider } from "./PageContext";
import { useDesktopAssistant } from "./DesktopAssistant";

const status = vi.hoisted(() => vi.fn());
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/assistant", () => ({ Service: { Status: status } }));
function Shell() { const assistant = useDesktopAssistant(); return <div>{assistant.trigger}<main hidden={assistant.expanded}>Synthetic business page</main>{assistant.panel}</div>; }
afterEach(() => { vi.unstubAllGlobals(); status.mockReset(); });

it("leaves closed surfaces idle, expands, collapses with focus, and pauses offline", async () => {
  vi.stubGlobal("matchMedia", (query: string) => ({ matches: query === "(min-width: 1280px)", addEventListener: () => {}, removeEventListener: () => {} }));
  status.mockResolvedValue({ version: 1, surface: "desktop", pageGuide: true, provider: "unavailable", credentials: "unavailable", conversations: "memory", modelRuns: false, financialWrites: false, reason: "activation_incomplete" });
  render(<QueryClientProvider client={createTestQueryClient()}><AssistantPageProvider householdId="synthetic" incarnation="generation" surface="desktop" viewKey="overview:" blocked={() => false} onNavigate={vi.fn()}><Shell /></AssistantPageProvider></QueryClientProvider>);
  expect(status).not.toHaveBeenCalled();
  expect(screen.queryByTestId("assistant-conversation")).not.toBeInTheDocument();
  const user = userEvent.setup(); await user.click(screen.getByRole("button", { name: "Open AI assistant" }));
  await waitFor(() => expect(status).toHaveBeenCalledTimes(1));
  expect(screen.getByRole("heading", { name: "AI assistant" })).toHaveFocus();
  await user.click(screen.getByRole("button", { name: "Expand workspace" }));
  expect(screen.getByText("Synthetic business page")).not.toBeVisible();
  act(() => window.dispatchEvent(new Event("offline")));
  // Match the browser's online state for the event-driven transport fence.
  vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
  act(() => window.dispatchEvent(new Event("offline")));
  expect(screen.getByText(/Disconnected\. Actions are paused/)).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Collapse assistant" }));
  expect(screen.getByRole("button", { name: "Open AI assistant" })).toHaveFocus();
  expect(screen.getByText("Synthetic business page")).toBeVisible();
  vi.restoreAllMocks();
});
