import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import i18n from "@/i18n";
import { MobileAccessSection } from "./MobileAccessSection";

const mobile = vi.hoisted(() => ({ Status: vi.fn(), Configure: vi.fn(), GenerateToken: vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/mobile/service", () => mobile);
vi.mock("@/lib/wails", () => ({ callService: (run: () => Promise<unknown>) => run() }));
const status = { config: { enabled: true, listen: "192.168.1.20:9443", token: "SYNTHETIC1" }, url: "http://192.168.1.20:9443", running: true };
beforeEach(async () => {
  vi.clearAllMocks();
  await i18n.changeLanguage("en");
  mobile.Status.mockResolvedValue(status);
  mobile.Configure.mockResolvedValue(status);
  mobile.GenerateToken.mockResolvedValue("ABCD234567");
});
function mount() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}><MobileAccessSection /></QueryClientProvider>);
}
it("uses only a LAN address and editable token, applying changes explicitly", async () => {
  const user = userEvent.setup();
  mount();
  await screen.findByText("Running");
  expect(screen.getAllByRole("checkbox")).toHaveLength(1);
  expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  const address = screen.getByLabelText("LAN listen address");
  const token = screen.getByLabelText("Mobile token");
  expect(token).toHaveAttribute("type", "password");
  await user.clear(address);
  await user.type(address, "192.168.1.21:9000");
  await user.clear(token);
  await user.type(token, "MyOwn123");
  expect(mobile.Configure).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Apply connection settings" }));
  expect(mobile.Configure).toHaveBeenCalledWith({ enabled: true, listen: "192.168.1.21:9000", token: "MyOwn123" });
});
it("generates a short draft token without changing the live token or QR until apply", async () => {
  const user = userEvent.setup();
  mount();
  await screen.findByText("Running");
  const qr = screen.getByRole("img", { name: "Open mobile connection" }).outerHTML;
  await user.click(screen.getByRole("button", { name: "Generate token" }));
  expect(await screen.findByDisplayValue("ABCD234567")).toHaveAttribute("type", "text");
  expect(mobile.Configure).not.toHaveBeenCalled();
  expect(screen.getByRole("img", { name: "Open mobile connection" }).outerHTML).toBe(qr);
  await user.click(screen.getByRole("button", { name: "Hide token" }));
  expect(screen.getByLabelText("Mobile token")).toHaveAttribute("type", "password");
  await user.click(screen.getByRole("button", { name: "Apply connection settings" }));
  expect(mobile.Configure).toHaveBeenCalledWith(expect.objectContaining({ token: "ABCD234567" }));
});
it("shows failed apply without losing the draft", async () => {
  const user = userEvent.setup();
  mobile.Configure.mockRejectedValue(new Error("synthetic failure"));
  mount();
  await screen.findByText("Running");
  await user.clear(screen.getByLabelText("Mobile token"));
  await user.type(screen.getByLabelText("Mobile token"), "NewToken1");
  await user.click(screen.getByRole("button", { name: "Apply connection settings" }));
  await screen.findByRole("alert");
  expect(screen.getByLabelText("Mobile token")).toHaveValue("NewToken1");
});
it("can disable access with incomplete address and token", async () => {
  const user = userEvent.setup();
  mount();
  await screen.findByText("Running");
  await user.click(screen.getByLabelText("Enable mobile access"));
  await user.clear(screen.getByLabelText("LAN listen address"));
  await user.clear(screen.getByLabelText("Mobile token"));
  await user.click(screen.getByRole("button", { name: "Apply connection settings" }));
  expect(mobile.Configure).toHaveBeenCalledWith({ enabled: false, listen: "", token: "" });
});
it.each(["en", "zh-CN", "zh-TW"])("renders the simple setup and migration guidance in %s", async (language) => {
  await i18n.changeLanguage(language);
  mobile.Status.mockResolvedValue({ ...status, error: "configuration_reset" });
  mount();
  await screen.findByText(i18n.t("mobile.running"));
  expect(screen.getByRole("alert")).toHaveTextContent(i18n.t("mobile.connectionError.configuration_reset"));
  expect(screen.getByText(i18n.t("mobile.setupHelp"))).not.toHaveTextContent(/HTTPS|TLS/);
  expect(screen.getByLabelText(i18n.t("mobile.listen"))).toBeInTheDocument();
  expect(screen.getByText(i18n.t("mobile.connectionHelp"))).toBeInTheDocument();
});
