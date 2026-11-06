import { BusinessPageRoot } from "@/lib/businessCapture";
import { capturePageContent } from "@/features/assistant/pageContent";
import { beforeEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import nilRuntimeStatus from "../../../../testdata/assistant/nil-runtime-status.json";
import { AICredentialsSection } from "./AICredentialsSection";
const api = vi.hoisted(() => ({ Status: vi.fn(), SaveCredential: vi.fn(), DeleteCredential: vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/airuntime/service", () => ({ ...api, Usage: async () => (await import("@/test/aiUsage")).emptyUsageReport }));
beforeEach(() => {
  vi.clearAllMocks();
  api.Status.mockResolvedValue({ epoch: "host-epoch", credentials: "missing", fullAccess: false, available: false });
  api.SaveCredential.mockImplementation(async () => { api.Status.mockResolvedValue({ epoch: "host-epoch", credentials: "present" }); });
  api.DeleteCredential.mockImplementation(async () => { api.Status.mockResolvedValue({ epoch: "host-epoch", credentials: "missing" }); });
});
function mount() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return { client, ...render(<QueryClientProvider client={client}><BusinessPageRoot view="settings:" accessIdentity="synthetic"><AICredentialsSection /></BusinessPageRoot></QueryClientProvider>) };
}
it("saves, replaces and removes new input without retaining secrets in caches", async () => {
  const user = userEvent.setup(); const { client } = mount();
  await screen.findByText("No AI key saved");
  const input = screen.getByLabelText("OpenAI API key"); expect(input).toHaveAttribute("type", "password");
  await user.type(input, "synthetic-first-key");
  await user.click(screen.getByRole("button", { name: "Save AI key" }));
  await screen.findByText("••••••••");
  expect(api.SaveCredential).toHaveBeenCalledWith("host-epoch", "synthetic-first-key");
  expect(screen.queryByDisplayValue("synthetic-first-key")).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "Replace" }));
  expect(screen.getByLabelText("OpenAI API key")).toHaveValue("");
  await user.type(screen.getByLabelText("OpenAI API key"), "synthetic-next-key");
  await user.click(screen.getByRole("button", { name: "Save AI key" }));
  await waitFor(() => expect(api.SaveCredential).toHaveBeenCalledWith("host-epoch", "synthetic-next-key"));
  await user.click(screen.getByRole("button", { name: "Remove" }));
  await screen.findByText("No AI key saved"); expect(api.DeleteCredential).toHaveBeenCalledWith("host-epoch");
  expect(JSON.stringify(client.getQueryCache().getAll().map((q) => q.state.data))).not.toContain("synthetic-");
  expect(client.getMutationCache().getAll()).toHaveLength(0);
});
it("shows a fixed error and clears new input on rejected saves", async () => {
  api.SaveCredential.mockRejectedValue(new Error("synthetic-key-in-untrusted-error"));
  const user = userEvent.setup(); mount(); await screen.findByText("No AI key saved");
  await user.type(screen.getByLabelText("OpenAI API key"), "synthetic-new-key");
  await user.click(screen.getByRole("button", { name: "Save AI key" }));
  expect(await screen.findByRole("alert")).not.toHaveTextContent("synthetic-");
  expect(screen.getByLabelText("OpenAI API key")).toHaveValue("");
});
it("disables credential actions when the host is unavailable", async () => {
  api.Status.mockResolvedValue({ credentials: "unavailable" }); mount();
  await screen.findByText(/AI configuration unavailable/);
  expect(screen.getByLabelText("OpenAI API key")).toBeDisabled();
  expect(screen.getByRole("button", { name: "Save AI key" })).toBeDisabled();
});


it("disables credential input and submission for the actual nil-runtime wire DTO", async () => {
  // The Go adapter test pins this shared fixture to NewService(nil).Status().
  api.Status.mockResolvedValue(nilRuntimeStatus);
  const { client } = mount();
  await waitFor(() => expect(client.getQueryData(["aiCredentialStatus"])).toEqual(nilRuntimeStatus));
  expect(screen.getByRole("status")).toHaveTextContent("AI configuration unavailable");
  const input = screen.getByLabelText("OpenAI API key");
  expect(input).toBeDisabled();
  expect(screen.getByRole("button", { name: "Save AI key" })).toBeDisabled();
  fireEvent.submit(input.closest("form")!);
  expect(api.SaveCredential).not.toHaveBeenCalled();
  expect(api.DeleteCredential).not.toHaveBeenCalled();
});


it.each([
  ["legacy zero-value status", "", ""],
  ["missing key without a host", "", "missing"],
  ["stored key without a host", "", "present"],
  ["unknown credential state", "host-epoch", "unexpected"],
])("refuses credential mutation for %s", async (_name, epoch, credentials) => {
  const dto = { ...nilRuntimeStatus, epoch, credentials };
  api.Status.mockResolvedValue(dto);
  const { client } = mount();
  await waitFor(() => expect(client.getQueryData(["aiCredentialStatus"])).toEqual(dto));
  expect(screen.getByRole("status")).toHaveTextContent("AI configuration unavailable");
  expect(screen.getByRole("button", { name: "Save AI key" })).toBeDisabled();
  const remove = screen.queryByRole("button", { name: "Remove" });
  if (remove) expect(remove).toBeDisabled();
  fireEvent.submit(screen.getByRole("button", { name: "Save AI key" }).closest("form")!);
  expect(api.SaveCredential).not.toHaveBeenCalled();
  expect(api.DeleteCredential).not.toHaveBeenCalled();
});

it("keeps an enabled credential submit human-only after manual draft entry",async()=>{
 const user=userEvent.setup();mount();await screen.findByText("No AI key saved");await user.type(screen.getByLabelText("OpenAI API key"),"synthetic-security-canary");
 expect(screen.getByRole("button",{name:"Save AI key"})).toBeEnabled();
 const page=capturePageContent("settings:");const save=page.nodes.find(n=>n.name==="Save AI key")!;expect(save.targetRef).toBeUndefined();expect(save.actions).toEqual([]);expect(save.state).toContain("operation_protected=true");expect(JSON.stringify(page)).not.toContain("synthetic-security-canary");expect(api.SaveCredential).not.toHaveBeenCalled();
});
