import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import i18n from "@/i18n";
import { MobileAccessSection } from "./MobileAccessSection";

const mobile = vi.hoisted(() => ({ Status: vi.fn(), Configure: vi.fn(), GenerateToken: vi.fn() }));
vi.mock("../../../bindings/github.com/waltwang/nestworth-go/internal/wailsapi/mobile/service", () => mobile);
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

it.each(["en", "zh-CN", "zh-TW"])("explains rejected wildcard addresses at the real input in %s", async (language) => {
  await i18n.changeLanguage(language);
  mobile.Configure.mockRejectedValue(new Error(JSON.stringify({ code: "validation", field: "listen", message: "invalid" })));
  const user = userEvent.setup();
  mount();
  const address = await screen.findByLabelText(i18n.t("mobile.listen"));
  await user.clear(address);
  await user.type(address, "0.0.0.0:9443");
  await user.click(screen.getByRole("button", { name: i18n.t("mobile.saveAccess") }));
  expect(await screen.findByRole("alert")).toHaveTextContent(i18n.t("mobile.listenValidation"));
  expect(address).toHaveAttribute("aria-invalid", "true");
  expect(address).toHaveAccessibleDescription(expect.stringContaining(i18n.t("mobile.listenValidation")));
  expect(address).toHaveValue("0.0.0.0:9443");
  await waitFor(() => expect(address).toBeEnabled());
  await user.clear(address);
  await user.type(address, "192.168.1.20:9443");
  expect(address).not.toHaveAttribute("aria-invalid");
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});
it("associates backend token validation with the token input", async () => {
  mobile.Configure.mockRejectedValue(new Error(JSON.stringify({ code: "validation", field: "token", message: "invalid" })));
  const user = userEvent.setup();
  mount();
  await screen.findByText("Running");
  await user.click(screen.getByRole("button", { name: "Apply connection settings" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(i18n.t("mobile.tokenValidation"));
  expect(screen.getByLabelText("Mobile token")).toHaveAttribute("aria-invalid", "true");
});

it.each(["en", "zh-CN", "zh-TW"].flatMap(language => [
  { field: "listen", value: "" },
  { field: "token", value: "" },
  { field: "token", value: "short" },
  { field: "token", value: "invalid!token" },
].map(test => ({ language, ...test }))))("lets application validation explain $field '$value' inline in $language", async ({ language, field, value }) => {
  await i18n.changeLanguage(language);
  mobile.Configure.mockRejectedValue(new Error(JSON.stringify({ code: "validation", field, message: "invalid" })));
  const user = userEvent.setup();
  mount();
  const input = await screen.findByLabelText(i18n.t(`mobile.${field}`));
  await user.clear(input);
  if (value) await user.type(input, value);
  await user.click(screen.getByRole("button", { name: i18n.t("mobile.saveAccess") }));
  await waitFor(() => expect(mobile.Configure).toHaveBeenCalledTimes(1));
  expect(await screen.findByRole("alert")).toHaveTextContent(i18n.t(`mobile.${field}Validation`));
  expect(input).toHaveAttribute("aria-invalid", "true");
  expect(input).toHaveAccessibleDescription(expect.stringContaining(i18n.t(`mobile.${field}Validation`)));
  expect(input.closest("form")).toHaveAttribute("novalidate");
});

it("retranslates a retained rejection on language changes without resubmitting or dropping the draft", async () => {
  mobile.Configure.mockRejectedValue(new Error(JSON.stringify({ code: "validation", field: "token", message: "invalid" })));
  const user = userEvent.setup();
  mount();
  const input = await screen.findByLabelText("Mobile token");
  await user.clear(input);
  await user.type(input, "SYNTHETIC2");
  await user.click(screen.getByRole("button", { name: "Apply connection settings" }));
  await screen.findByRole("alert");
  await waitFor(() => expect(input).toBeEnabled());
  for (const language of ["zh-CN", "zh-TW", "en"]) {
    await act(async () => { await i18n.changeLanguage(language); });
    expect(screen.getByRole("alert")).toHaveTextContent(i18n.t("mobile.tokenValidation"));
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAccessibleDescription(expect.stringContaining(i18n.t("mobile.tokenValidation")));
    expect(input).toHaveValue("SYNTHETIC2");
    expect(mobile.Configure).toHaveBeenCalledTimes(1);
  }
});
